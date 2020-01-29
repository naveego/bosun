package kube

type RancherClusterConfig struct {
}

func (c RancherClusterConfig) configureKubernetes(ctx ConfigureKubeContextRequest) error {

	ctx.Log.Infof("Bosun cannot configure a rancher cluster, you must do it yourself.")

	return nil
}
