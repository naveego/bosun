package bosun

import (
	"github.com/naveego/bosun/pkg"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"strings"
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
	File   string           `yaml:"file,omitempty"`
	Layout *pkg.VaultLayout `yaml:"layout,omitempty"`
}

type AppTestAction struct {
	Exec *DynamicValue `yaml:"exec"`
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
	values := ctx.Values
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
	} else {
		layoutBytes, _ = yaml.Marshal(vaultAction.Layout)
	}

	env := ctx.Env

	values.AddPath("cluster", env.Cluster)
	values.AddPath("domain", env.Domain)

	templateArgs := pkg.TemplateValues{
		Cluster: env.Cluster,
		Domain:  env.Domain,
		Values:  values,
	}

	vaultLayout, err = pkg.LoadVaultLayoutFromBytes(a.Name, layoutBytes, templateArgs, vaultClient)
	if err != nil {
		return err
	}

	y, _ := yaml.Marshal(vaultLayout)
	ctx.Log.Debugf("Vault layout from %s:\n%s", a.Name, string(y))

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
	_, err := t.Exec.Execute(ctx)

	return err
}
