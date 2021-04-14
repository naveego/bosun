package kube

import (
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environmentvariables"
	"github.com/naveego/bosun/pkg/values"
)

type StackTemplate struct {
	core.ConfigShared `yaml:",inline"`
	NamePattern       string                               `yaml:"namePattern"`
	Variables         []*environmentvariables.Variable     `yaml:"variables,omitempty"`
	Namespaces        NamespaceConfigs                     `yaml:"namespaces"`
	Apps              map[string]values.ValueSetCollection `yaml:"apps"`
	Certs             []ClusterCert                        `yaml:"certs"`
	ValueOverrides    *values.ValueSetCollection           `yaml:"valueOverrides,omitempty"`
}

type StackState struct {
	Name         string
	DeployedApps map[string]StackApp `yaml:"apps"`
}


func (e *StackTemplate) SetFromPath(path string) {
	e.FromPath = path
	for i := range e.Variables {
		e.Variables[i].FromPath = path
	}
}