package bosun

import (
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environment"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
)

type DeploymentPlan struct {
	core.ConfigShared         `yaml:",inline"`
	ReleaseVersion            *semver.Version      `yaml:"releaseVersion"`
	// Hash of the release this plan was based on, if any - used to detect that the plan is out of date
	BasedOnHash string `yaml:"basedOnHash"`
	DirectoryPath             string               `yaml:"-"`
	ProviderPriority          []string             `yaml:"providerPriority"`
	SkipDependencyValidation  bool                 `yaml:"skipDependencyValidation"`
	ValueOverrides            values.ValueSet      `yaml:"valueOverrides"`
	DeployApps                map[string]bool      `yaml:"deployApps"`
	EnvironmentDeployProgress map[string][]string  `yaml:"environmentDeployProgress"`
	Apps                      []*AppDeploymentPlan `yaml:"apps"`
	BundleInfo *BundleInfo `yaml:"bundleInfo,omitempty"`
}

type BundleInfo struct {
	Environments map[string]*environment.Config
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

	if d.DirectoryPath == "" {
		if d.FromPath != "" {
			d.DirectoryPath = filepath.Dir(d.FromPath)
		} else {
			return errors.New("directoryPath and fromPath were both empty")
		}
	}

	_ = os.RemoveAll(d.DirectoryPath)
	if err = os.MkdirAll(d.DirectoryPath, 0700); err != nil {
		return err
	}

	for _, app := range d.Apps {
		savePath, saveErr := app.Manifest.Save(d.DirectoryPath)
		if saveErr != nil {
			return errors.Wrapf(err, "saving portable manifest for app %q from providers %+v", app.Name, d.ProviderPriority)
		}
		app.ManifestPath, _ = filepath.Rel(d.DirectoryPath, savePath)
	}

	return d.SavePlanFileOnly()
}

func (d DeploymentPlan) SavePlanFileOnly() error {

	planPath := d.FromPath
	if planPath == "" {
		if d.DirectoryPath == "" {
			return errors.New("fromPath and directoryPath were both empty")
		}
		planPath = filepath.Join(d.DirectoryPath, "plan.yaml")
	}

	err := yaml.SaveYaml(planPath, d)

	return err
}

type AppDeploymentPlan struct {
	Name           string          `yaml:"name"`
	Tag            string          `yaml:"tag"`
	ValueOverrides values.ValueSet `yaml:"valueOverrides"`
	ManifestPath   string          `yaml:"manifestPath"`
	Manifest       *AppManifest    `yaml:"-"`
}
