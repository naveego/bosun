package bosun

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/mongo"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

type ActionSchedule string

const (
	ActionBeforeDeploy = "BeforeDeploy"
	ActionAfterDeploy  = "AfterDeploy"
	ActionManual       = "Manual"
)

type AppAction struct {
	Name               string             `yaml:"name" json:"name"`
	Description        string             `yaml:"description,omitempty" json:"description,omitempty"`
	When               ActionSchedule     `yaml:"when,omitempty" json:"when,omitempty"`
	Where              string             `yaml:"where,omitempty" json:"where,omitempty"`
	MaxAttempts        int                `yaml:"maxAttempts,omitempty" json:"maxAttempts,omitempty"`
	Timeout            time.Duration      `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Interval           time.Duration      `yaml:"interval,omitempty" json:"interval,omitempty"`
	Vault              *VaultAction       `yaml:"vault,omitempty" json:"vault,omitempty"`
	Script             *ScriptAction      `yaml:"script,omitempty" json:"script,omitempty"`
	Test               *TestAction        `yaml:"test,omitempty" json:"test,omitempty"`
	Mongo              *MongoAction       `yaml:"mongo,omitempty" json:"mongo,omitempty"`
	MongoAssert        *MongoAssertAction `yaml:"mongoAssert,omitempty" json:"mongoAssert,omitempty"`
	ExcludeFromRelease bool               `yaml:"excludeFromRelease,omitempty" json:"excludeFromRelease,omitempty"`
	FromPath           string             `yaml:"-" json:"-"`
}

type Action interface {
	Execute(ctx BosunContext) error
}

type SelfContainer interface {
	MakeSelfContained(ctx BosunContext) error
}

// MakeSelfContained removes imports all file dependencies into literals,
// then deletes those dependencies.
func (a *AppAction) MakeSelfContained(ctx BosunContext) error {
	ctx = ctx.WithLog(ctx.Log.WithField("action", a.Name)).WithDir(a.FromPath)

	for _, action := range a.getActions() {
		if sc, ok := action.(SelfContainer); ok {
			err := sc.MakeSelfContained(ctx)
			if err != nil {
				return errors.Errorf("error making %q action self contained: %s", a.Name, err)
			}
		}
	}

	return nil
}

func (a *AppAction) Execute(ctx BosunContext) error {
	log := ctx.Log

	if a.Where != "" && !strings.Contains(a.Where, ctx.Env.Name) {
		log.Debugf("Skipping because 'where' is %q but current environment is %q.", a.Where, ctx.Env.Name)
		return nil
	}

	attempts := a.MaxAttempts
	if attempts == 0 {
		attempts = 1
	}
	timeout := a.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	interval := a.Interval
	if interval == 0 {
		interval = 5 * time.Second
	}

	for {
		ctx = ctx.WithLog(ctx.Log.WithField("action", a.Name))
		if a.FromPath != "" {
			// if the action has its own FromPath, we'll use it, but usually
			// actions are executed in a context which has already set the
			// Dir to the parent script or app
			ctx = ctx.WithDir(a.FromPath)
		}

		ctx.Log.WithField("description", a.Description).Infof("Executing action...")

		attemptCtx := ctx.WithTimeout(timeout)

		err := a.execute(attemptCtx)

		if err == nil {
			ctx.Log.Info("Action completed.")
			// succeeded
			return nil
		}

		attempts--

		ctx.Log.WithError(err).WithField("attempts_remaining", attempts).Error("Action failed.")

		if attempts == 0 {
			return err
		}
		seconds := int(interval.Seconds())
		fmt.Printf("\rWaiting: %d", seconds)
		for ; seconds >= 0; seconds = seconds - 1 {
			select {
			case <-ctx.Ctx().Done():
				fmt.Printf("\r")
				return nil
			case <-time.After(time.Second):
				fmt.Printf("\rWaiting: %d  ", seconds-1)
			}
		}
		fmt.Printf("\r                     \r")
	}
}

func (a *AppAction) execute(ctx BosunContext) error {
	for _, action := range a.getActions() {
		ctx.Log.Debugf("Executing %T action...", action)
		err := action.Execute(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *AppAction) getActions() []Action {

	var actions []Action
	if a.Vault != nil {
		actions = append(actions, a.Vault)
	}

	if a.Script != nil {
		actions = append(actions, a.Script)
	}

	if a.Test != nil {
		actions = append(actions, a.Test)
	}

	if a.Mongo != nil {
		actions = append(actions, a.Mongo)
	}

	if a.MongoAssert != nil {
		actions = append(actions, a.MongoAssert)
	}

	return actions
}

type VaultAction struct {
	File    string           `yaml:"file,omitempty" json:"file,omitempty"`
	Layout  *pkg.VaultLayout `yaml:"layout,omitempty" json:"layout,omitempty"`
	Literal string           `yaml:"literal,omitempty" json:"literal,omitempty"`
}

func (a *VaultAction) Execute(ctx BosunContext) error {
	vaultClient, err := ctx.GetVaultClient()
	if err != nil {
		return err
	}

	var vaultLayout *pkg.VaultLayout
	var layoutBytes []byte
	if a.File != "" {
		path := ctx.ResolvePath(a.File)
		layoutBytes, err = ioutil.ReadFile(path)
		if err != nil {
			return err
		}
	} else if a.Literal != "" {
		layoutBytes = []byte(a.Literal)
	} else {
		layoutBytes, _ = yaml.Marshal(a.Layout)
	}

	templateArgs := ctx.GetTemplateArgs()

	vaultLayout, err = pkg.LoadVaultLayoutFromBytes("action", layoutBytes, templateArgs, vaultClient)
	if err != nil {
		return err
	}

	y, _ := yaml.Marshal(vaultLayout)
	ctx.Log.Debugf("Vault layout from %s:\n%s\n", a.Layout, string(y))

	if ctx.IsDryRun() {
		return nil
	}

	err = vaultLayout.Apply(vaultClient)
	if err != nil {
		return err
	}

	return nil
}

func (a *VaultAction) MakeSelfContained(ctx BosunContext) error {
	if a.File != "" {

		path := ctx.ResolvePath(a.File)
		layoutBytes, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		a.File = ""
		a.Literal = string(layoutBytes)
	}

	return nil
}

type ScriptAction string

func (a *ScriptAction) Execute(ctx BosunContext) error {

	script := *a
	cmd := Command{
		Script: string(script),
	}

	_, err := cmd.Execute(ctx)

	return err
}

type TestAction struct {
	Exec *Command `yaml:"exec,omitempty" json:"exec,omitempty"`
	HTTP string   `yaml:"http,omitempty" json:"http,omitempty"`
	TCP  string   `yaml:"tcp,omitempty" json:"tcp,omitempty"`
}

func (t *TestAction) Execute(ctx BosunContext) error {

	if ctx.GetParams().DryRun {
		ctx.Log.Info("Skipping test because this is a dry run.")
		return nil
	}
	if t.Exec != nil {
		_, err := t.Exec.Execute(ctx)
		return err
	}

	if t.HTTP != "" {
		target, err := renderTemplate(ctx, t.HTTP)
		c := http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}

		ctx.Log.WithField("url", target).Infof("Making HTTP GET request...")

		resp, err := c.Get(target)
		if err != nil {
			return err
		}
		body, _ := ioutil.ReadAll(resp.Body)
		err = resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode >= 400 {
			return errors.Errorf("got non-success code %d - %s: %s", resp.StatusCode, resp.Status, string(body))
		}

		return nil
	}

	if t.TCP != "" {
		target, err := renderTemplate(ctx, t.HTTP)
		d := new(net.Dialer)
		conn, err := d.DialContext(ctx.Ctx(), "tcp", target)
		if conn != nil {
			conn.Close()
		}
		return err
	}

	return errors.New("test must have exec, http, or tcp element")
}

func renderTemplate(ctx BosunContext, tmpl string) (string, error) {

	t, err := template.New("").Parse(tmpl)
	if err != nil {
		return "", err
	}

	templateArgs := ctx.GetTemplateArgs()
	w := new(strings.Builder)
	err = t.Execute(w, templateArgs)

	return w.String(), err

}

type MongoAction struct {
	ConnectionName string           `yaml:"connectionName,omitempty"`
	Connection     mongo.Connection `yaml:"connection" json:"connection"`
	DatabaseFile   string           `yaml:"databaseFile" json:"databaseFile"`
	RebuildDB      bool             `yaml:"rebuildDb"`
}

func (a *MongoAction) Execute(ctx BosunContext) error {
	var dataFile []byte

	databaseFilePath := ctx.ResolvePath(a.DatabaseFile)

	dataFile, err := ioutil.ReadFile(databaseFilePath)
	if err != nil {
		return errors.Errorf("could not read file directly: %s", err)
	}

	ctx.Log.Debugf("parsing file '%s'", databaseFilePath)

	db := mongo.Database{}
	err = yaml.Unmarshal(dataFile, &db)
	if err != nil {
		return errors.Errorf("could not read file as yaml '%s': %v", databaseFilePath, err)
	}

	dataDir := filepath.Dir(databaseFilePath)

	cmd := mongo.MongoImportCommand{
		Conn:      a.Connection,
		DB:        db,
		DataDir:   dataDir,
		RebuildDB: a.RebuildDB,
		Log:       ctx.Log,
	}

	if a.ConnectionName != "" {
		var ok bool
		cmd.Conn, ok = ctx.GetKeyedValue(a.ConnectionName).(mongo.Connection)
		if !ok {
			return errors.Errorf("action had connectionName %q, but no connection with that name was found in the context", a.ConnectionName)
		}
	}

	j, _ := json.MarshalIndent(cmd, "", "  ")
	ctx.Log.Debugf("Executing mongo import command: \n%s", string(j))

	err = cmd.Execute()

	return err
}

type MongoAssertAction struct {
	ConnectionName      string                 `yaml:"connectionName,omitempty"`
	Connection          mongo.Connection       `yaml:"connection" json:"connection"`
	Database            string                 `yaml:"database,omitempty" json:"database,omitempty"`
	Collection          string                 `yaml:"collection" json:"collection"`
	Query               map[string]interface{} `yaml:"query" json:"query"`
	ExpectedResultCount int64                  `yaml:"expectedResultCount" json:"expectedResultCount"`
}

func (a *MongoAssertAction) Execute(ctx BosunContext) error {

	if a.ConnectionName != "" {
		var ok bool
		a.Connection, ok = ctx.GetKeyedValue(a.ConnectionName).(mongo.Connection)
		if !ok {
			return errors.Errorf("action had connectionName %q, but no connection with that name was found in the context", a.ConnectionName)
		}
	}

	pc, err := a.Connection.Prepare(ctx.Log)
	if err != nil {
		return errors.Wrap(err, "prepare connection")
	}
	defer pc.CleanUp()

	client, err := pc.GetClient()
	if err != nil {
		return errors.Wrap(err, "get client")
	}

	databaseName := a.Database
	if databaseName == "" {
		databaseName = a.Connection.DBName
	}

	db := client.Database(databaseName)

	collectionName := a.Collection
	collection := db.Collection(collectionName)

	res, err := collection.CountDocuments(ctx.Ctx(), a.Query)

	if res != a.ExpectedResultCount {
		return errors.Errorf("expected %d results, but found %d", a.ExpectedResultCount, res)
	}

	return nil
}
