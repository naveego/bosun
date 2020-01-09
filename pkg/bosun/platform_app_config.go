package bosun

import (
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/issues"
)

// PlatformAppConfig is the config unit for referencing
// an app from a platform and specifying the deployment
// settings for the app.
type PlatformAppConfig struct {
	Name    string         `yaml:"name"`
	RepoRef issues.RepoRef `yaml:"repo"`
	// The cluster roles this app should be deployed to.
	ClusterRoles core.ClusterRoles `yaml:"clusterRoles,flow"`
	// The namespace roles this app should be deployed to.
	NamespaceRoles core.NamespaceRoles `yaml:"namespaceRoles,flow"`
	Dependencies   []string            `yaml:"dependencies,omitempty"`
}

type PlatformAppConfigs []*PlatformAppConfig

func (p PlatformAppConfigs) Names() []string {
	var out []string
	for _, a := range p {
		out = append(out, a.Name)
	}
	return out
}
