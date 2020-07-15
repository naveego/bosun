package kube

import (
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
	config2 "sigs.k8s.io/controller-runtime/pkg/client/config"
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

func GetKubeClientWithContext(context string) (*kubernetes.Clientset, error) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		// not running in kubernetes...
		config, err = config2.GetConfigWithContext(context)

		if err != nil {
			return nil, errors.Wrapf(err, "could not get kube config with context %q from in cluster strategy or from ~/.kube/config", context)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)

	return clientset, nil
}
