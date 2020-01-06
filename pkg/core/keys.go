package core

const (
	KeyEnv     = "env"
	KeyCluster = "cluster"
)

type StringKeyValuer interface {
	GetStringValue(key string, defaultValue ...string) string
	WithStringValue(key string, value string) StringKeyValuer
}

type InterfaceKeyValuer interface {
	GetValue(key string, defaultValue ...interface{}) interface{}
	WithValue(key string, value interface{}) StringKeyValuer
}
