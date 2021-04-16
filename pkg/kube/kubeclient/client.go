package kubeclient

import (
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
)

func GetKubeClient() (*kubernetes.Clientset, error) {
	// creates the in-cluster kubeconfig
	config, err := rest.InClusterConfig()
	if err != nil {
		// not running in kubernetes...
		home := util.HomeDir()

		configPath := os.Getenv("KUBECONFIG")
		if configPath == "" {
			configPath = filepath.Join(home, ".kube", "config")
		}
		config, err = clientcmd.BuildConfigFromFlags("", configPath)

		if err != nil {
			return nil, errors.Wrapf(err, "could not get kube kubeconfig from in cluster strategy or from ~/.kube/config")
		}
	}

	clientset, err := kubernetes.NewForConfig(config)

	return clientset, nil
}

func GetKubeClientWithContext(configPath string, context string) (*kubernetes.Clientset, error) {

	config, err := GetKubeConfigWithContext(configPath, context)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrapf(err, "get api using kubeconfig from %q and context %q", configPath, context)
	}

	return clientset, nil
}

func GetKubeConfigWithContext(configPath string, context string) (*rest.Config, error) {

	config, err := rest.InClusterConfig()
	if err != nil {
		// not running in kubernetes...
		home := util.HomeDir()

		if configPath == "" {
			configPath = os.Getenv("KUBECONFIG")
		}
		if configPath == "" {
			configPath = filepath.Join(home, ".kube", "config")
		}
		config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: configPath},
			&clientcmd.ConfigOverrides{CurrentContext: context}).ClientConfig()

		if err != nil {
			return nil, errors.Wrapf(err, "could not get kube kubeconfig from in cluster strategy or from %s", configPath)
		}
	}

	return config, nil
}
