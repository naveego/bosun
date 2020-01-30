package environment

import (
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/pkg/errors"
	"io/ioutil"
)

type SecretGroupConfig struct {
	core.ConfigShared
	Secrets []*SecretConfig       `yaml:"metadata"`
	Passphrase     command.CommandValue `yaml:"passphrase"`
	Nonce string `yaml:"nonce"`
}

type SecretConfig struct {
	Name string `yaml:"name"`
}

type SecretGroup struct {
	SecretGroupConfig

	passphrase string
	key []byte
	updated bool
	values map[string]string
}

type Secret struct {
	SecretConfig
	Value string `yaml:"-"`
}


func (s *SecretGroupConfig) LoadSecrets(ctx command.ExecutionContext) (*SecretGroup, error) {

	filePath := s.ResolveRelative(s.Name + ".secrets")
	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	return &SecretGroup{
		SecretGroupConfig: *s,
		values: map[string]string{},
	}, nil
}

func (s *SecretGroup) GetSecretValue(name string)(string, error) {
	if value, ok := s.values[name]; ok {
		return value, nil
	}
	return "", errors.Errorf("group %q did not contain secret with name %q", s.Name, name)
}

func (s *SecretGroup) Save(ctx command.ExecutionContext) error {

	if !s.updated {
		return nil
	}

	return nil
}
