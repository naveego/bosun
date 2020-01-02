package kube

import (
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"strings"
)

type CommandContext struct {
	KubeConfigPath string
	Force          bool
	Name           string
	Log            *logrus.Entry
}

type ConfigDefinitions []*ConfigDefinition

func (k ConfigDefinitions) GetKubeConfigDefinitionByName(name string) (*ConfigDefinition, error) {
	for _, c := range k {
		if c.Name == name {
			return c, nil
		}
	}
	return nil, errors.Errorf("no definition with name %q", name)
}

func (k ConfigDefinitions) GetKubeConfigDefinitionsByAttributes(env string, role string) ([]*ConfigDefinition, error) {
	var out []*ConfigDefinition
	for _, c := range k {
		matched := env == ""
		if !matched {
			for _, e := range c.Environments {
				if e == env {
					matched = true
					break
				}
			}
		}
		if !matched {
			continue
		}
		matched = role == ""
		if !matched {
			for _, r := range c.Roles {
				if r == role {
					matched = true
					break
				}
			}
		}
		if !matched {
			continue
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, errors.Errorf("no definition had environment %q and role %q", env, role)
	}
	return out, nil
}

type KubernetesConfigurer interface {
	ConfigureKubernetes(ctx CommandContext) error
}

type ConfigDefinition struct {
	Name         string               `yaml:"name"`
	Roles        []string             `yaml:"roles"`
	Environments []string             `yaml:"environments"`
	Oracle       *OracleClusterConfig `yaml:"oracle,omitempty"`
	Minikube     *MinikubeConfig      `yaml:"minikube,omitempty"`
	Amazon       *AmazonClusterConfig `yaml:"amazon,omitempty"`
}

func (k ConfigDefinition) ConfigureKubernetes(ctx CommandContext) error {
	ctx.Name = k.Name

	if k.Oracle != nil {
		ctx.Log.Infof("Configuring Oracle cluster %q...", k.Name)

		err := k.Oracle.ConfigureKubernetes(ctx)
		return err
	}

	if k.Minikube != nil {
		ctx.Log.Infof("Configuring minikube cluster %q...", k.Name)

		err := k.Minikube.ConfigureKubernetes(ctx)
		return err
	}

	if k.Amazon != nil {
		ctx.Log.Infof("Configuring Amazon cluster %q...", k.Name)

		err := k.Amazon.ConfigureKubernetes(ctx)
		return err
	}

	return errors.Errorf("no recognized kube vendor found on %q", k.Name)
}

func contextIsDefined(name string) bool {
	out, err := pkg.NewCommand("kubectl",
		"config",
		"get-contexts",
		name,
	).RunOut()
	if err != nil {
		return false
	}
	if strings.Contains(out, "error:") {
		return false
	}
	return true
}
