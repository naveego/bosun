package kube


// ExternalClusterConfig represents a cluster we do not control.  When using this we will not create things like
// storage classes etc.
type ExternalClusterConfig struct {
}

func (c ExternalClusterConfig) configureKubernetes(ctx ConfigureKubeContextRequest) error {

	ctx.Log.Infof("Bosun cannot configure an external cluster, you must do it yourself.")

	return nil
}
