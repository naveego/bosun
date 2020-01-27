package core

const (
	KeyCluster         = "cluster"
	KeyClusterRole     = "clusterRole"
	KeyClusterRoles    = "clusterRoles"
	KeyClustersRoles    = "clustersRoles"
	KeyClusterProvider = "clusterProvider"
	KeyNamespace       = "namespace"
	KeyNamespaceRole   = "namespaceRole"
	KeyNamespaceRoles  = "namespaceRoles"
	KeyEnvironment     = "environment"
	KeyEnvironmentRole = "environmentRole"
	KeyAppName         = "appName"
	KeyAppVersion         = "appVersion"
	KeyReleaseVersion  = "releaseVersion"
)

type StringKeyValuer interface {
	GetStringValue(key string, defaultValue ...string) string
	WithStringValue(key string, value string) StringKeyValuer
}

type InterfaceKeyValuer interface {
	GetValue(key string, defaultValue ...interface{}) interface{}
	WithValue(key string, value interface{}) StringKeyValuer
}
