package actions

import (
	"fmt"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/vault"
	"github.com/naveego/bosun/pkg/yaml"
	"io/ioutil"
)

type VaultAction struct {
	CacheKey string           `yaml:"cacheKey,omitempty" json:"cacheKey"`
	File     string           `yaml:"file,omitempty" json:"file,omitempty"`
	Layout   *vault.VaultLayout `yaml:"layout,omitempty" json:"layout,omitempty"`
	Literal  string           `yaml:"literal,omitempty" json:"literal,omitempty"`
}

func (a *VaultAction) Execute(ctx ActionContext) error {
	var vaultClient *vaultapi.Client
	err := ctx.Provide(&vaultClient)
	if err != nil {
		return err
	}

	var vaultLayout *vault.VaultLayout
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

	vaultLayout, err = vault.LoadVaultLayoutFromBytes("action", layoutBytes, templateArgs, vaultClient)
	if err != nil {
		return err
	}

	y, _ := yaml.Marshal(vaultLayout)
	ctx.Log().Debugf("Vault layout from %s:\n%s\n", a.Layout, string(y))

	if ctx.GetParameters().DryRun {
		return nil
	}

	if a.CacheKey == "" {
		for _,k := range util.SortedKeys(ctx.Log().Data) {
			a.CacheKey += fmt.Sprintf("%s:%s/", k, ctx.Log().Data[k])
		}
	}

	err = vaultLayout.Apply(a.CacheKey, ctx.GetParameters().Force, vaultClient, ctx.Log())

	return err
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
