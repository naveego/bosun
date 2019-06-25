package kube

import (
	"github.com/go-errors/errors"
	"github.com/naveego/bosun/pkg/util"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"path/filepath"
)

func GetKubeClient() (*kubernetes.Clientset, error) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		// not running in kubernetes...
		home := util.HomeDir()
		configPath := filepath.Join(home, ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", configPath)

		if err != nil {
			return nil, errors.Errorf("could not get kube config from in cluster strategy or from ~/.kube/config")
		}
	}

	clientset, err := kubernetes.NewForConfig(config)

	return clientset, nil
}
