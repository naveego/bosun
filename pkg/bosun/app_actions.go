package bosun

import (
	"crypto/tls"
	"github.com/naveego/bosun/pkg"
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
	Name        string          `yaml:"name"`
	Description string          `yaml:"description,omitempty"`
	When        ActionSchedule  `yaml:"when"`
	Where       string          `yaml:"where"`
	MaxAttempts int             `yaml:"maxAttempts,omitempty"`
	Timeout     time.Duration   `yaml:"timeout"`
	Interval    time.Duration   `yaml:"interval"`
	Vault       *AppVaultAction `yaml:"vault,omitempty"`
	Exec        *DynamicValue   `yaml:"exec,omitempty"`
	Test        *AppTestAction  `yaml:"test,omitempty"`
}

type AppVaultAction struct {
	File    string           `yaml:"file,omitempty"`
	Layout  *pkg.VaultLayout `yaml:"layout,omitempty"`
	Literal string           `yaml:"literal,omitempty"`
}

type AppTestAction struct {
	Exec *DynamicValue `yaml:"exec,omitempty"`
	HTTP string        `yaml:"http,omitempty""`
	TCP  string        `yaml:"tcp,omitempty""`
}

// MakeSelfContained removes imports all file dependencies into literals,
// then deletes those dependencies.
func (a *AppAction) MakeSelfContained(ctx BosunContext) error {
	if a.Vault != nil {
		if a.Vault.File != "" {
			path := ctx.ResolvePath(a.Vault.File)
			layoutBytes, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}
			a.Vault.File = ""
			a.Vault.Literal = string(layoutBytes)
		}
	}

	return nil
}

func (a *AppAction) Execute(ctx BosunContext) error {
	ctx = ctx.WithLog(ctx.Log.WithField("action", a.Name))
	log := ctx.Log

	if a.Where != "" && !strings.Contains(a.Where, ctx.Env.Name) {
		log.Debugf("Skipping because 'where' is %q.", a.Where)
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

		attemptCtx := ctx.WithTimeout(timeout)

		err := a.execute(attemptCtx)

		if err == nil {
			// succeeded
			return nil
		}

		attempts--

		ctx.Log.WithError(err).WithField("attempts_remaining", attempts).Error("Test failed.")

		if attempts == 0 {
			return err
		}

		ctx.Log.WithField("wait", interval).Info("Waiting before trying again.")
		select {
		case <-ctx.Ctx().Done():
			return nil
		case <-time.After(interval):
		}
	}

	return nil
}

func (a *AppAction) execute(ctx BosunContext) error {
	log := ctx.Log
	if a.Vault != nil {
		log.Debug("Applying vault layout...")
		err := a.executeVault(ctx)
		if err != nil {
			return err
		}
	}

	if a.Exec != nil {
		log.Debug("Executing command or script...")
		_, err := a.Exec.Execute(ctx)
		if err != nil {
			return err
		}
	}

	if a.Test != nil {
		log.Debug("Executing test...")
		err := a.executeTest(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *AppAction) executeVault(ctx BosunContext) error {
	vaultClient, err := ctx.GetVaultClient()
	if err != nil {
		return err
	}

	vaultAction := a.Vault
	var vaultLayout *pkg.VaultLayout
	var layoutBytes []byte
	if vaultAction.File != "" {
		path := ctx.ResolvePath(vaultAction.File)
		layoutBytes, err = ioutil.ReadFile(path)
		if err != nil {
			return err
		}
	} else if vaultAction.Literal != "" {
		layoutBytes = []byte(vaultAction.Literal)
	} else {
		layoutBytes, _ = yaml.Marshal(vaultAction.Layout)
	}

	templateArgs := ctx.GetTemplateArgs()

	vaultLayout, err = pkg.LoadVaultLayoutFromBytes(a.Name, layoutBytes, templateArgs, vaultClient)
	if err != nil {
		return err
	}

	y, _ := yaml.Marshal(vaultLayout)
	ctx.Log.Debugf("Vault layout from %s:\n%s\n", a.Name, string(y))

	if ctx.IsDryRun() {
		return nil
	}

	err = vaultLayout.Apply(vaultClient)
	if err != nil {
		return err
	}

	return nil
}

func (a *AppAction) executeTest(ctx BosunContext) error {
	t := a.Test

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
		resp.Body.Close()
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
