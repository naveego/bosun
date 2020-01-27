package bosun

import (
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/values"
)

type TemplateBosunValues struct {
	AppName         string              `yaml:"appName"`
	AppVersion      string              `yaml:"appVersion"`
	Cluster         string              `yaml:"cluster"`
	ClusterRole     string              `yaml:"clusterRole"`
	ClusterRoles    []string            `yaml:"clusterRoles"`
	ClusterProvider string              `yaml:"clusterProvider"`
	ClustersRoles   map[string][]string `yaml:"clusters"`
	Namespace       string              `yaml:"namespace"`
	NamespaceRole   string              `yaml:"namespaceRole"`
	NamespaceRoles  []string            `yaml:"namespaceRoles"`
	Environment     string              `yaml:"environment"`
	EnvironmentRole string              `yaml:"environmentRole"`
	ReleaseVersion  string              `yaml:"releaseVersion"`
}

func (t TemplateBosunValues) ToValues() values.Values {
	return values.Values{

		core.KeyCluster:         t.Cluster,
		core.KeyClusterRole:     t.ClusterRole,
		core.KeyClusterRoles:    t.ClusterRoles,
		core.KeyClustersRoles:   t.ClustersRoles,
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
