package kube

import (
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/kube/kubeclient"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"os"
)

type Cluster struct {
	ctx command.ExecutionContext

	ClusterConfig
	Kubectl    Kubectl
	Client     *kubernetes.Clientset
	kubeconfig *rest.Config
}

func NewCluster(config ClusterConfig, ctx command.ExecutionContext, allowIncomplete bool) (*Cluster, error) {

	kubectl := Kubectl{
		Cluster:    config.Name,
		Kubeconfig: config.GetKubeconfigPath(),
	}

	if kubectl.Kubeconfig == "" {
		kubectl.Kubeconfig = os.ExpandEnv("$HOME/.kube/config")
	}

	c := &Cluster{
		ctx:           ctx,
		ClusterConfig: config,
		Kubectl:       Kubectl{},
	}

	var err error
	c.kubeconfig, err = kubeclient.GetKubeConfigWithContext(config.KubeconfigPath, config.Name)
	if err != nil && !allowIncomplete{
		return nil, errors.Wrapf(err, "could not create kubernetes client for cluster %q, you may need to run `bosun cluster configure`", config.Name)
	}

	if c.kubeconfig != nil {

		c.Client, err = kubernetes.NewForConfig(c.kubeconfig)
		if err != nil && !allowIncomplete {
			return nil, err
		}
	}

	return c, nil

}

func (c *Cluster) ConfigureKubectl() error {

	kubectl := c.Kubectl
	config := c.ClusterConfig
	ctx := c.ctx

	if kubectl.contextIsDefined(config.Name) && !ctx.GetParameters().Force {
		ctx.Log().Debugf("Kubernetes context %q already exists (use --force to configure anyway).", config.Name)
	} else {

		k := config
		req := ConfigureRequest{
			KubeConfigPath:   config.KubeconfigPath,
			Force:            ctx.GetParameters().Force,
			Log:              ctx.Log(),
			ExecutionContext: ctx,
		}

		if k.Oracle != nil {
			req.Log.Infof("Configuring Oracle cluster %q...", k.Name)

			if err := k.Oracle.configureKubernetes(req); err != nil {
				return err
			}
		} else if k.Minikube != nil {
			req.Log.Infof("Configuring minikube cluster %q...", k.Name)

			if err := k.Minikube.configureKubernetes(req); err != nil {
				return err
			}
		} else if k.Microk8s != nil {
			req.Log.Infof("Configuring microk8s cluster %q...", k.Name)

			if err := k.Microk8s.configureKubernetes(req); err != nil {
				return err
			}
		} else if k.Amazon != nil {
			req.Log.Infof("Configuring Amazon cluster %q...", k.Name)

			if err := k.Amazon.configureKubernetes(req); err != nil {
				return err
			}
		} else if k.Rancher != nil {
			req.Log.Infof("Configuring Rancher cluster %q...", k.Name)

			if err := k.Rancher.configureKubernetes(req); err != nil {
				return err
			}
		} else if k.ExternalCluster != nil {
			req.Log.Infof("Configuring external cluster %q...", k.Name)

			if err := k.ExternalCluster.configureKubernetes(req); err != nil {
				return err
			}
		} else {
			return errors.Errorf("no recognized kube vendor found on %q", k.Name)
		}
	}

	return nil
}

func (c *Cluster) Activate() error {
	_, err := c.Kubectl.Exec("config", "use-context", c.Name)

	return err
}

func (c *Cluster) GetDefaultNamespace() string {
	namespace := c.DefaultNamespace
	if namespace == "" {
		namespace = "default"
	}
	return namespace
}

var _ values.ValueSetCollectionProvider = &Cluster{}
