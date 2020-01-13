package core

const (
	KeyEnv             = "env"
	KeyCluster         = "cluster"
	KeyClusterRole     = "clusterRole"
	KeyClusterRoles    = "clusterRoles"
	KeyClusterProvider = "clusterProvider"
	KeyNamespace       = "namespace"
	KeyNamespaceRole   = "namespaceRole"
	KeyNamespaceRoles  = "namespaceRoles"
	KeyEnvironment     = "environment"
	KeyEnvironmentRole = "environmentRole"
	KeyAppName         = "appName"
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
