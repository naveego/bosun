package bosun

import (
	"github.com/naveego/bosun/pkg"
	"gopkg.in/yaml.v2"
	"strings"
)

type ActionSchedule string
const (
	ActionBeforeDeploy = "BeforeDeploy"
	ActionAfterDeploy = "AfterDeploy"
	ActionManual = "Manual"
)

type AppAction struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description,omitempty"`
	When        ActionSchedule `yaml:"when"`
	Where      string 			`yaml:"where"`
	VaultFile   string         `yaml:"vaultFile,omitempty"`
	Exec        *DynamicValue  `yaml:"exec,omitempty"`
}

func (a *AppAction) Execute(ctx BosunContext, values Values) error {
	log := ctx.Log.WithField("action", a.Name)

	if a.Where != "" && !strings.Contains(a.Where, ctx.Env.Name) {
		log.Debugf("Skipping because 'where' is %q.", a.Where)
		return nil
	}


	if a.VaultFile != "" {
		log.Debug("Applying vault layout...")
		a.VaultFile = resolvePath(ctx.Dir + "/placeholder", a.VaultFile)
		err := a.executeVault(ctx, values)
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

	return nil
}

func (a *AppAction) executeVault(ctx BosunContext, values Values) error {
	vaultClient, err := ctx.GetVaultClient()
	if err != nil {
		return err
	}

	env := ctx.Env

	values.AddPath("cluster", env.Cluster)
	values.AddPath("domain", env.Domain)

	templateArgs := pkg.TemplateValues{
		Cluster:env.Cluster,
		Domain:env.Domain,
		Values:values,
	}

	vaultLayout, err := pkg.LoadVaultLayoutFromFiles([]string{a.VaultFile}, templateArgs, vaultClient)
	if err != nil {
		return err
	}

	y, _ := yaml.Marshal(vaultLayout)
	ctx.Log.Debugf("Vault layout from %s:\n%s", a.VaultFile, string(y))

	if ctx.IsDryRun() {
		return nil
	}

	err = vaultLayout.Apply(vaultClient)
	if err != nil {
		return err
	}

	return nil
}
