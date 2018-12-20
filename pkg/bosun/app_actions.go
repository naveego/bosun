package bosun

import (
	"github.com/naveego/bosun/pkg"
	"gopkg.in/yaml.v2"
)

type ActionSchedule string
const (
	ActionBeforeDeploy = "BeforeDeploy"
	ActionAfterDeploy = "AfterDeploy"
	ActionManual = "Manual"
)

type AppAction struct {
	Name string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Schedule ActionSchedule `yaml:"schedule"`
	VaultFile string `yaml:"vaultFile,omitempty"`
}

func (a *AppAction) Execute(ctx BosunContext, values Values) error {

	if a.VaultFile != "" {
		a.VaultFile = resolvePath(ctx.Dir, a.VaultFile)
		err := a.executeVault(ctx, values)
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

	if ctx.IsDryRun() {
		y, _ := yaml.Marshal(vaultLayout)
		ctx.Log.Debugf("Vault layout from %s (skipped for dry run):\n %s", a.VaultFile, string(y))
		return nil
	}

	err = vaultLayout.Apply(vaultClient)
	if err != nil {
		return err
	}

	return nil
}
