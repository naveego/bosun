package actions

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/go-getter/helper/url"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/mongo"
	"github.com/naveego/bosun/pkg/templating"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	vault "github.com/hashicorp/vault/api"
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
	Bosun              *BosunAction       `yaml:"bosun,omitempty" json:"bosun,omitempty"`
	Test               *TestAction        `yaml:"test,omitempty" json:"test,omitempty"`
	Mongo              *MongoAction       `yaml:"mongo,omitempty" json:"mongo,omitempty"`
	MongoAssert        *MongoAssertAction `yaml:"mongoAssert,omitempty" json:"mongoAssert,omitempty"`
	HTTP               *HTTPAction        `yaml:"http,omitempty" json:"http,omitempty"`
	ExcludeFromRelease bool               `yaml:"excludeFromRelease,omitempty" json:"excludeFromRelease,omitempty"`
	FromPath           string             `yaml:"-" json:"-"`
}

type Action interface {
	Execute(ctx ActionContext) error
}

type SelfContainer interface {
	MakeSelfContained(ctx ActionContext) error
}

// MakeSelfContained removes imports all file dependencies into literals,
// then deletes those dependencies.
func (a *AppAction) MakeSelfContained(ctx ActionContext) error {
	ctx = ctx.WithPwd(a.FromPath).(ActionContext)
	ctx = ctx.WithLogField("action", a.Name).(ActionContext)

	for _, action := range a.GetActions() {
		if sc, ok := action.(SelfContainer); ok {
			err := sc.MakeSelfContained(ctx)
			if err != nil {
				return errors.Errorf("error making %q action self contained: %s", a.Name, err)
			}
		}
	}

	return nil
}

func (a *AppAction) Execute(ctx ActionContext) error {
	log := ctx.Log()
	env := ctx.GetStringValue(core.KeyEnv)

	if a.Where != "" && !strings.Contains(a.Where, env) {
		log.Debugf("Skipping because 'where' is %q but current environment is %q.", a.Where, env)
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

	var err error

	ctx = ctx.WithLogField("action", a.Name).(ActionContext)
	if a.FromPath != "" {
		// if the action has its own FromPath, we'll use it, but usually
		// actions are executed in a context which has already set the
		// Dir to the parent script or app
		ctx = ctx.WithPwd(a.FromPath).(ActionContext)
	}

	for i := 0; i < attempts; i++ {
		if i > 0 {
			seconds := int(interval.Seconds())
			ctx.Log().WithError(err).WithField("attempts_remaining", attempts-i).Errorf("Action failed, waiting %s...", interval)
			if seconds > 0 {
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

		ctx.Log().WithField("description", a.Description).Infof("Executing action...")

		attemptCtx := ctx.WithTimeout(timeout).(ActionContext)

		err = a.execute(attemptCtx)

		if err == nil {
			ctx.Log().Info("Action completed.")
			// succeeded
			return nil
		}
	}

	return errors.Wrapf(err, "action failed after %d attempts (with a timeout of %s and an interval of %s); final error",
		attempts, timeout, interval)

}

func (a *AppAction) execute(ctx ActionContext) error {
	actions := a.GetActions()
	if len(actions) == 0 {
		return errors.New("no actions defined")
	}

	for _, action := range a.GetActions() {
		ctx = ctx.WithLogField("action_type", fmt.Sprintf("%T", action)).(ActionContext)
		ctx.Log().Debugf("Executing %T action...", action)
		err := action.Execute(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

var actionInterfaceType = reflect.TypeOf((*Action)(nil))

func (a *AppAction) GetActions() []Action {

	if a == nil {
		return nil
	}

	var actions []Action
	v := reflect.ValueOf(*a)
	t := v.Type()
	n := t.NumField()
	for i := 0; i < n; i++ {
		fv := v.Field(i)
		if fv.Kind() == reflect.Ptr || fv.Kind() == reflect.Slice || fv.Kind() == reflect.Map {
			if fv.IsNil() {
				continue
			}
			action, ok := fv.Interface().(Action)
			if ok && action != nil {
				actions = append(actions, action)
			}
		}
	}

	return actions
}

type VaultAction struct {
	CacheKey string           `yaml:"cacheKey" json:"cacheKey"`
	File     string           `yaml:"file,omitempty" json:"file,omitempty"`
	Layout   *pkg.VaultLayout `yaml:"layout,omitempty" json:"layout,omitempty"`
	Literal  string           `yaml:"literal,omitempty" json:"literal,omitempty"`
}

func (a *VaultAction) Execute(ctx ActionContext) error {
	var vaultClient *vault.Client
	err := ctx.Provide(&vaultClient)
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

	templateArgs := ctx.TemplateValues()

	vaultLayout, err = pkg.LoadVaultLayoutFromBytes("action", layoutBytes, templateArgs, vaultClient)
	if err != nil {
		return err
	}

	y, _ := yaml.Marshal(vaultLayout)
	ctx.Log().Debugf("Vault layout from %s:\n%s\n", a.Layout, string(y))

	if ctx.GetParameters().DryRun {
		return nil
	}

	err = vaultLayout.Apply(a.CacheKey, ctx.GetParameters().Force, vaultClient)
	if err != nil {
		return err
	}

	return nil
}

func (a *VaultAction) MakeSelfContained(ctx ActionContext) error {
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

func (a *ScriptAction) Execute(ctx ActionContext) error {

	script := *a
	cmd := command.Command{
		Script: string(script),
	}

	_, err := cmd.Execute(ctx)

	return err
}

type BosunAction []string

func (a BosunAction) Execute(ctx ActionContext) error {

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	var stepArgs []string
	stepArgs = append(stepArgs, a...)
	if ctx.GetParameters().Verbose {
		stepArgs = append(stepArgs, "--verbose")
	}
	if ctx.GetParameters().DryRun {
		stepArgs = append(stepArgs, "--dry-run")
	}

	stepArgs = append(stepArgs, "--cluster", ctx.GetStringValue(core.KeyCluster))

	log := ctx.WithLogField("args", stepArgs).Log()
	log.WithField("args", stepArgs).Info("Executing step")

	err = pkg.NewShellExe(exe, stepArgs...).WithDir(ctx.Pwd()).RunE()
	if err != nil {
		log.WithError(err).WithField("args", stepArgs).Error("Step failed.")
		return err
	}

	return nil
}

type TestAction struct {
	Exec *command.Command `yaml:"exec,omitempty" json:"exec,omitempty"`
	HTTP string           `yaml:"http,omitempty" json:"http,omitempty"`
	TCP  string           `yaml:"tcp,omitempty" json:"tcp,omitempty"`
}

func (t *TestAction) Execute(ctx ActionContext) error {

	if ctx.GetParameters().DryRun {
		ctx.Log().Info("Skipping test because this is a dry run.")
		return nil
	}
	if t.Exec != nil {
		_, err := t.Exec.Execute(ctx)
		return err
	}

	if t.HTTP != "" {
		target, err := templating.RenderTemplate(t.HTTP, ctx.TemplateValues())
		c := http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}

		ctx.Log().WithField("url", target).Infof("Making HTTP GET request...")

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
		target, err := templating.RenderTemplate(t.HTTP, ctx.TemplateValues())
		d := new(net.Dialer)
		conn, err := d.DialContext(ctx.Ctx(), "tcp", target)
		if conn != nil {
			conn.Close()
		}
		return err
	}

	return errors.New("test must have exec, http, or tcp element")
}

type MongoAction struct {
	ConnectionName string           `yaml:"connectionName,omitempty"`
	Connection     mongo.Connection `yaml:"connection" json:"connection"`
	DatabaseFile   string           `yaml:"databaseFile" json:"databaseFile"`
	RebuildDB      bool             `yaml:"rebuildDb"`
	Script         string           `yaml:"script,omitempty"`
}

func (a *MongoAction) Execute(ctx ActionContext) error {

	if a.Script != "" {

		script, err := templating.RenderTemplate(a.Script, ctx.TemplateValues())
		if err != nil {
			return errors.Wrap(err, "RenderEnvironmentSettingScript script")
		}

		cmd := mongo.ScriptCommand{
			Conn:   a.Connection,
			Script: script,
			Log:    ctx.Log(),
		}

		err = cmd.Execute()

		return err
	}

	var dataFile []byte

	databaseFilePath := ctx.ResolvePath(a.DatabaseFile)

	dataFile, err := ioutil.ReadFile(databaseFilePath)
	if err != nil {
		return errors.Errorf("could not read file directly: %s", err)
	}

	ctx.Log().Debugf("parsing file '%s'", databaseFilePath)

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
		Log:       ctx.Log(),
	}

	if a.ConnectionName != "" {
		var ok bool
		cmd.Conn, ok = ctx.GetValue(a.ConnectionName).(mongo.Connection)
		if !ok {
			return errors.Errorf("action had connectionName %q, but no connection with that name was found in the context", a.ConnectionName)
		}
	}

	if db.Name != "" {
		cmd.Conn.DBName = db.Name
	}

	j, _ := json.MarshalIndent(cmd, "", "  ")
	ctx.Log().Debugf("Executing mongo import command: \n%s", string(j))

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

func (a *MongoAssertAction) Execute(ctx ActionContext) error {

	if a.ConnectionName != "" {
		var ok bool
		a.Connection, ok = ctx.GetValue(a.ConnectionName).(mongo.Connection)
		if !ok {
			return errors.Errorf("action had connectionName %q, but no connection with that name was found in the context", a.ConnectionName)
		}
	}

	pc, err := a.Connection.Prepare(ctx.Log())
	if err != nil {
		return errors.Wrap(err, "prepare connection")
	}
	defer pc.CleanUp()

	client := pc.Client

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

type HTTPAction struct {
	URL     string                 `yaml:"url" json:"url"`
	Method  string                 `yaml:"method" json:"method"`
	Headers map[string]string      `yaml:"headers" json:"headers"`
	Body    map[string]interface{} `yaml:"body,omitempty" json:"body"`
	Raw     string                 `yaml:"raw,omitempty" json:"raw,omitempty"`
	OKCodes []int                  `yaml:"okCodes,omitempty,flow" json:"okCodes,omitempty"` // codes which should be treated as OK (passing). Defaults to [200, 201, 202, 204].
}

func (a *HTTPAction) Execute(ctx ActionContext) error {

	var req *http.Request
	var err error
	if a.Raw == "" {
		var bodyBytes []byte
		var err error
		if a.Body != nil {
			bodyBytes, err = json.Marshal(a.Body)
			if err != nil {
				return errors.Wrap(err, "marshal body")
			}
		}

		bodyBuffer := bytes.NewBuffer(bodyBytes)
		a.Method = strings.ToUpper(a.Method)

		ctx.Log().Debugf("Making %s request to %s...", a.Method, a.URL)

		req, err = http.NewRequest(a.Method, a.URL, bodyBuffer)
		if err != nil {
			return errors.Wrap(err, "create req")
		}
		for k, v := range a.Headers {
			req.Header.Add(k, v)
		}
	} else {
		if !strings.Contains(a.Raw, "\n\n") {
			a.Raw = a.Raw + "\n\n"
		}

		r := bufio.NewReader(strings.NewReader(a.Raw))

		req, err = http.ReadRequest(r)
		if err != nil {
			return errors.Wrapf(err, "create req from raw input:\n%s", a.Raw)
		}
		req.URL, err = url.Parse(req.RequestURI)
		if err != nil {
			return errors.Wrapf(err, "parse url %q", req.RequestURI)
		}
		req.RequestURI = ""
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "made request")
	}

	ctx.Log().Debugf("Request returned %d - %s.", resp.StatusCode, resp.Status)

	if len(a.OKCodes) == 0 {
		a.OKCodes = []int{http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent}
	}

	defer resp.Body.Close()
	isOK := false
	for _, okCode := range a.OKCodes {
		if resp.StatusCode == okCode {
			isOK = true
			break
		}
	}

	if !isOK {
		respBody, _ := ioutil.ReadAll(resp.Body)
		err = errors.Errorf("Response had non-success status code %d (OKCodes: %v): %s", resp.StatusCode, a.OKCodes, string(respBody))
		return err
	}

	return nil
}
