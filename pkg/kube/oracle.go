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
	KubeConfigDefinition ConfigDefinition
	KubeCommandContext   CommandContext
}

func (oc OracleClusterConfig) configureKubernetes(ctx CommandContext) error {

	if contextIsDefined(ctx.Name) && !ctx.Force {
		ctx.Log.Infof("Kubernetes context %q already exists (use --force to configure anyway).", ctx.Name)
		return nil
	}

	kubeConfigPath := os.ExpandEnv("$HOME/.kube/config")
	if ctx.KubeConfigPath != "" {
		kubeConfigPath = ctx.KubeConfigPath
	}

	err := pkg.NewShellExe("oci", "ce", "cluster", "create-kubeconfig",
		"--token-version", "2.0.0",
		"--cluster-id", oc.OCID,
		"--file", kubeConfigPath,
		"--region", oc.Region,
	).RunE()

	if err != nil {
		return err
	}

	opaqueName := oc.OCID[len(oc.OCID)-11:]

	err = pkg.NewShellExe("kubectl", "config",
		"rename-context",
		"context-"+opaqueName,
		ctx.Name,
	).RunE()

	return err
}
