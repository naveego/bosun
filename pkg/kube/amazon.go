package kube

import (
	"github.com/naveego/bosun/pkg/command"
)

type AmazonClusterConfig struct {
	Region string `yaml:"region"`
}

func (c AmazonClusterConfig) configureKubernetes(ctx ConfigureRequest) error {

	if c.Region == "" {
		c.Region = "us-east-1"
	}

	err := command.NewShellExe("aws", "eks", "--region", c.Region, "update-kubeconfig", "--kubeconfig", ctx.KubeConfigPath, "--name", ctx.Brn.ClusterName, "--alias", ctx.Brn.ClusterName).RunE()
	if err != nil {
		return err
	}

	return err
}
