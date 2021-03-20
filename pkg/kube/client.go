package kube

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
	// creates the in-cluster config
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
			return nil, errors.Wrapf(err, "could not get kube config from in cluster strategy or from ~/.kube/config")
		}
	}

	clientset, err := kubernetes.NewForConfig(config)

	return clientset, nil
}

func GetKubeClientWithContext(configPath string, context string) (*kubernetes.Clientset, error) {

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
			return nil, errors.Wrapf(err, "could not get kube config from in cluster strategy or from %s", configPath)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrapf(err, "get api using config from %q and context %q", configPath, context)
	}

	return clientset, nil
}
