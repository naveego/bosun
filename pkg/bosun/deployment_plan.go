package bosun

import (
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/values"
)

type DeploymentPlan struct {
	core.ConfigShared `yaml:",inline"`
	Provider          string               `yaml:"provider"`
	ValueOverrides    values.ValueSet      `yaml:"valueOverrides"`
	Apps              []*AppDeploymentPlan `yaml:"apps"`
}

type AppDeploymentPlan struct {
	Name           string          `yaml:"name"`
	ValueOverrides values.ValueSet `yaml:"valueOverrides"`
	ManifestPath   string          `yaml:"manifestPath"`
}
