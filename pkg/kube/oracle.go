package kube

import (
	"github.com/naveego/bosun/pkg"
	"os"
)

type OracleClusterConfig struct {
	OCID   string `yaml:"ocid"`
	Region string `yaml:"region"`
}

type ConfigureOracleClusterCommand struct {
	KubeConfigDefinition KubeConfigDefinition
	KubeCommandContext   KubeCommandContext
}

func (c ConfigureOracleClusterCommand) Execute() error {

	oc := c.KubeConfigDefinition.Oracle

	kubeConfigPath := os.ExpandEnv("$HOME/.kube/config")
	if c.KubeCommandContext.KubeConfigPath != "" {
		kubeConfigPath = c.KubeCommandContext.KubeConfigPath
	}

	err := pkg.NewCommand("oci", "ce", "cluster", "create-kubeconfig",
		"--token-version", "2.0.0",
		"--cluster-id", oc.OCID,
		"--file", kubeConfigPath,
		"--region", oc.Region,
	).RunE()

	if err != nil {
		return err
	}

	opaqueName := oc.OCID[len(oc.OCID)-11:]

	err = pkg.NewCommand("kubectl", "config",
		"rename-context",
		"context-"+opaqueName,
		c.KubeConfigDefinition.Name,
	).RunE()

	return err
}
