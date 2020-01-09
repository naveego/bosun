package bosun

import (
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
)

type DeploymentPlan struct {
	core.ConfigShared  `yaml:",inline"`
	DirectoryPath      string               `yaml:"-"`
	Provider           string               `yaml:"provider"`
	IgnoreDependencies bool                 `yaml:"ignoreDependencies,omitempty"`
	ValueOverrides     values.ValueSet      `yaml:"valueOverrides"`
	Apps               []*AppDeploymentPlan `yaml:"apps"`
}

func LoadDeploymentPlanFromFile(path string) (*DeploymentPlan, error) {
	var out DeploymentPlan
	err := yaml.LoadYaml(path, &out)
	if err != nil {
		return &out, err
	}

	out.SetFromPath(path)

	for _, appPlan := range out.Apps {
		manifestPath := out.ResolveRelative(appPlan.ManifestPath)

		appPlan.Manifest, err = LoadAppManifestFromPathAndName(manifestPath, appPlan.Name)
		if err != nil {
			return nil, err
		}
	}

	return &out, nil
}

func (d DeploymentPlan) Save() error {
	var err error

	_ = os.RemoveAll(d.DirectoryPath)
	if err = os.MkdirAll(d.DirectoryPath, 0700); err != nil {
		return err
	}

	for _, app := range d.Apps {
		err = app.Manifest.Save(d.DirectoryPath)
		if err != nil {
			return errors.Wrapf(err, "saving portable manifest for app %q from provider %q", app.Name, d.Provider)
		}
	}

	planPath := filepath.Join(d.DirectoryPath, "plan.yaml")
	err = yaml.SaveYaml(planPath, d)

	return err
}

type AppDeploymentPlan struct {
	Name           string          `yaml:"name"`
	ValueOverrides values.ValueSet `yaml:"valueOverrides"`
	ManifestPath   string          `yaml:"manifestPath"`
	Manifest       *AppManifest    `yaml:"-"`
}
