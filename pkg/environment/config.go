package environment

import (
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environmentvariables"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/script"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/yaml"
)

type Config struct {
	core.ConfigShared `yaml:",inline"`
	Role              core.EnvironmentRole   `yaml:"role" json:"role"`
	ClusterRoles      []string               `yaml:"clusterRoles,omitempty" json:"clusterRoles,omitempty"`
	DefaultCluster    string                 `yaml:"defaultCluster,omitempty" json:"defaultCluster"`
	Clusters          kube.ConfigDefinitions `yaml:"clusters,omitempty"`
	PullSecrets       []kube.PullSecret      `yaml:"pullSecrets,omitempty"`
	VaultNamespace    string                 `yaml:"vaultNamespace,omitempty" json:"vaultNamespace,omitempty"`
	// If true, commands which would cause modifications to be deployed will
	// trigger a confirmation prompt.
	Protected bool                             `yaml:"protected" json:"protected"`
	IsLocal   bool                             `yaml:"isLocal" json:"isLocal"`
	Commands  []*Command                       `yaml:"commands,omitempty" json:"commands,omitempty"`
	Variables []*environmentvariables.Variable `yaml:"variables,omitempty" json:"variables,omitempty"`
	Scripts   []*script.Script                 `yaml:"scripts,omitempty" json:"scripts,omitempty"`
	// Contains app value overrides which should be applied when deploying
	// apps to this environment.
	AppValues         *values.ValueSet                     `yaml:"appValues,omitempty" json:"appValues"`
	ValueSetNames     []string                             `yaml:"valueSets,omitempty" json:"valueSets,omitempty"`
	ValueOverrides    *values.ValueSetCollection           `yaml:"valueOverrides,omitempty"`
	AppValueOverrides map[string]values.ValueSetCollection `yaml:"appValueOverrides,omitempty"`
	// Apps which should not be deployed to this environment.
	AppBlacklist         []string          `yaml:"appBlacklist,omitempty"`
	SecretGroupFilePaths map[string]string `yaml:"secretFiles"`
}

func (e *Config) MarshalYAML() (interface{}, error) {
	if e == nil {
		return nil, nil
	}
	type proxy Config
	p := proxy(*e)

	return &p, nil
}

func (e *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type proxy Config
	var p proxy
	if e != nil {
		p = proxy(*e)
	}

	err := unmarshal(&p)

	if err == nil {
		*e = Config(p)
	}

	return err
}

func LoadConfig(path string) (*Config, error) {
	var config *Config
	err := yaml.LoadYaml(path, &config)
	if err != nil {
		return nil, err
	}
	config.SetFromPath(path)
	return config, nil
}

type Command struct {
	FromPath string                `yaml:"fromPath,omitempty" json:"fromPath,omitempty"`
	Name     string                `yaml:"name" json:"name"`
	Exec     *command.CommandValue `yaml:"exec,omitempty" json:"exec,omitempty"`
}

func (e *Config) SetFromPath(path string) {
	e.FromPath = path
	for i := range e.Scripts {
		e.Scripts[i].SetFromPath(path)
	}
	for i := range e.Variables {
		e.Variables[i].FromPath = path
	}
	for i := range e.Commands {
		e.Variables[i].FromPath = path
	}
	for i := range e.Clusters {
		e.Clusters[i].FromPath = path
	}
}
func (e *Config) Merge(other *Config) {

	e.Commands = append(e.Commands, other.Commands...)
	e.Variables = append(e.Variables, other.Variables...)

	for _, v := range other.Scripts {
		e.Scripts = append(e.Scripts, v)
	}
}

func firstNonemptyString(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}
