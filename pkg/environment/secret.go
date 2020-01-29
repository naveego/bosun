package environment

import (
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
)

type SecretsConfig struct {
	core.ConfigShared
	Metadata []SecretMetadata `yaml:"metadata"`
	Key command.CommandValue `yaml:"key"`
}

type SecretMetadata struct {
	Name string `yaml:"name"`
}

type Secrets struct {
	SecretsConfig

	Values map[string]string
}


func LoadSecrets(ctx command.ExecutionContext,  config SecretsConfig) (*Secrets, error) {

}

func (s *Secrets) Save(ctx command.ExecutionContext) error {

}
