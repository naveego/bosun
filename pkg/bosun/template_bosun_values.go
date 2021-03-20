package bosun

import (
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/values"
)

type TemplateBosunValues struct {
	AppName         string   `yaml:"appName"`
	AppVersion      string   `yaml:"appVersion"`
	Cluster         string   `yaml:"cluster"`
	Stack           string   `yaml:"stack"`
	ClusterRoles    []string `yaml:"clusterRoles"`
	ClusterProvider string   `yaml:"clusterProvider"`
	Namespace       string   `yaml:"namespace"`
	NamespaceRole   string   `yaml:"namespaceRole"`
	NamespaceRoles  []string `yaml:"namespaceRoles"`
	Environment     string   `yaml:"environment"`
	EnvironmentRole string   `yaml:"environmentRole"`
	ReleaseVersion  string   `yaml:"releaseVersion"`
	DeployedAt      string   `yaml:"deployedAt"`
}

func (t TemplateBosunValues) ToValues() values.Values {
	return values.Values{

		core.KeyCluster:         t.Cluster,
		core.KeyStack:           t.Stack,
		core.KeyClusterRoles:    t.ClusterRoles,
		core.KeyClusterProvider: t.ClusterProvider,
		core.KeyNamespace:       t.Namespace,
		core.KeyNamespaceRole:   t.NamespaceRole,
		core.KeyNamespaceRoles:  t.NamespaceRoles,
		core.KeyEnvironment:     t.Environment,
		core.KeyEnvironmentRole: t.EnvironmentRole,
		core.KeyAppName:         t.AppName,
		core.KeyAppVersion:      t.AppVersion,
		core.KeyReleaseVersion:  t.ReleaseVersion,
	}
}
