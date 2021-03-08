package kube

import (
	"github.com/naveego/bosun/pkg"
)

type AmazonClusterConfig struct {
	Region string `yaml:"region"`
}

func (c AmazonClusterConfig) configureKubernetes(ctx ConfigureRequest) error {

	if c.Region == "" {
		c.Region = "us-east-1"
	}

	err := pkg.NewShellExe("aws", "eks", "--region", c.Region, "update-kubeconfig", "--name", ctx.Name, "--alias", ctx.Name).RunE()
	if err != nil {
		return err
	}

	return err
}
