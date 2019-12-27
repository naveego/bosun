package kube

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type KubeCommandContext struct {
	KubeConfigPath string
	Log            *logrus.Entry
}

type KubeConfigDefinition struct {
	Name   string               `yaml:"name"`
	Oracle *OracleClusterConfig `yaml:"oracle,omitempty"`
}

func (k KubeConfigDefinition) Configure(ctx KubeCommandContext) error {
	if k.Oracle != nil {
		ctx.Log.Infof("Configuring Oracle cluster %q...", k.Name)
		command := ConfigureOracleClusterCommand{
			KubeConfigDefinition: k,
			KubeCommandContext:   ctx,
		}

		err := command.Execute()
		return err
	}

	return errors.Errorf("no recognized kube vendor found on %q", k.Name)
}
