package bosun

import (
	"crypto/tls"
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/mongo"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net"
	"net/http"
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
	Name               string         `yaml:"name"`
	Description        string         `yaml:"description,omitempty"`
	When               ActionSchedule `yaml:"when,omitempty"`
	Where              string         `yaml:"where,omitempty"`
	MaxAttempts        int            `yaml:"maxAttempts,omitempty"`
	Timeout            time.Duration  `yaml:"timeout,omitempty"`
	Interval           time.Duration  `yaml:"interval,omitempty"`
	Vault              *VaultAction   `yaml:"vault,omitempty"`
	Script             *ScriptAction  `yaml:"script,omitempty"`
	Test               *TestAction    `yaml:"test,omitempty"`
	ExcludeFromRelease bool           `yaml:"excludeFromRelease,omitempty"`
	FromPath           string         `yaml:"-"`
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
		ctx = ctx.WithLog(ctx.Log.WithField("action", a.Name)).WithDir(a.FromPath)

		ctx.Log.Infof("Executing action...")

		attemptCtx := ctx.WithTimeout(timeout)

		err := a.execute(attemptCtx)

		if err == nil {
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

	return actions
}

type VaultAction struct {
	File    string           `yaml:"file,omitempty"`
	Layout  *pkg.VaultLayout `yaml:"layout,omitempty"`
	Literal string           `yaml:"literal,omitempty"`
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
	Exec *Command `yaml:"exec,omitempty"`
	HTTP string   `yaml:"http,omitempty"`
	TCP  string   `yaml:"tcp,omitempty"`
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
	Connection   mongo.Connection `yaml:"connection"`
	DatabaseFile string           `yaml:"databaseFile"`
}
