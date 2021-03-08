package kube

import (
	"github.com/naveego/bosun/pkg"
	"os"
)

type OracleClusterConfig struct {
	OCID   string `yaml:"ocid"`
	Region string `yaml:"region"`
}

func (oc OracleClusterConfig) configureKubernetes(ctx ConfigureRequest) error {

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
		"delete-context",
		ctx.Name,
	).RunE()

	err = pkg.NewShellExe("kubectl", "config",
		"rename-context",
		"context-"+opaqueName,
		ctx.Name,
	).RunE()

	return err
}
