package bosun

import (
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environment"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/values"
)

// PlatformAppConfig is the config unit for referencing
// an app from a platform and specifying the deployment
// settings for the app.
type PlatformAppConfig struct {
	Name          string                `yaml:"name"`
	RepoRef       issues.RepoRef        `yaml:"repo"`
	Dependencies  []string              `yaml:"dependsOn,omitempty"`
	TargetFilters filter.MatchMapConfig `yaml:"targetFilters"`
	// The cluster roles this app should be deployed to.
	ClusterRoles core.ClusterRoles `yaml:"clusterRoles,flow"`
	// The namespace roles this app should be deployed to.
	NamespaceRoles core.NamespaceRoles        `yaml:"namespaceRoles,flow"`
	ValueOverrides *values.ValueSetCollection `yaml:"valueOverrides,omitempty"`
}


func (e *PlatformAppConfig) GetValueSetCollection() values.ValueSetCollection {
	if e.ValueOverrides == nil {
		return values.NewValueSetCollection()
	}
	return *e.ValueOverrides
}

type PlatformAppConfigs []*PlatformAppConfig

func (p PlatformAppConfigs) Names() []string {
	var out []string
	for _, a := range p {
		out = append(out, a.Name)
	}
	return out
}

func (p PlatformAppConfigs) FilterByEnvironment(env *environment.Environment) PlatformAppConfigs {
	stack := env.Stack()

	var out PlatformAppConfigs
	for _, app := range p {

		_, allowedByEnvironment := env.Apps[app.Name]
		if !allowedByEnvironment {
			core.Log.Debugf("Skipping app %q because it's disabled or not included in environment %q", app.Name, env.Name)
			continue
		}

		clusterApp, ok := stack.StackTemplate.Apps[app.Name]

		disabledForCluster := ok && clusterApp.Disabled

		if disabledForCluster {
			core.Log.Debugf("Skipping app %q because it's disabled for stack %q", app.Name, stack.Name)
		}

		out = append(out, app)
	}

	return out
}