package kube

import (
	"github.com/naveego/bosun/pkg"
)

type OracleClusterConfig struct {
	OCID   string `yaml:"ocid"`
	Region string `yaml:"region"`
}

func (oc OracleClusterConfig) configureKubernetes(ctx ConfigureRequest) error {

	kubectl := Kubectl{Kubeconfig: ctx.KubeConfigPath}

	err := pkg.NewShellExe("oci", "ce", "cluster", "create-kubeconfig",
		"--token-version", "2.0.0",
		"--cluster-id", oc.OCID,
		"--file", kubectl.Kubeconfig,
		"--region", oc.Region,
	).RunE()

	if err != nil {
		return err
	}

	opaqueName := oc.OCID[len(oc.OCID)-11:]

	_, err = kubectl.Exec( "config",
		"delete-context",
		ctx.Brn.ClusterName,
	)

	if err != nil {
		ctx.Log.Warnf("Delete of previous context instance %q failed: %s", ctx.Brn.Cluster, err.Error())
	}

	_, err = kubectl.Exec("config",
		"rename-context",
		"context-"+opaqueName,
		ctx.Brn.ClusterName,
	)

	return err
}
