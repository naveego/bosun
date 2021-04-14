package kube

import (
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/naveego/bosun/pkg/brns"
	"github.com/naveego/bosun/pkg/command"
	"github.com/pkg/errors"
	"os"
)

type ClusterConfigs []*ClusterConfig

func (k ClusterConfigs) Headers() []string {
	return []string{
		"Name",
		"KubeConfig",
		"EnvironmentBrn",
	}
}

func (k ClusterConfigs) Rows() [][]string {
	var out [][]string
	for _, c := range k {
		row := []string{
			c.Name,
			c.KubeconfigPath,
			c.Environment,
		}
		out = append(out, row)
	}
	return out
}

func (k ClusterConfigs) GetClusterConfig(name string) (*ClusterConfig, error) {

	for _, config := range k {
		if config.Name == name ||
			util.StringSliceContains(config.Aliases, name) {
			return config, nil
		}
	}

	return nil, errors.Errorf("no cluster with name or alias %q", name)
}

func (k ClusterConfigs) GetCluster(name string, ctx command.ExecutionContext) (*Cluster, error) {

	for _, config := range k {
		if config.Name == name ||
			util.StringSliceContains(config.Aliases, name) {
			return NewCluster(*config, ctx)
		}
	}

	return nil, errors.Errorf("no cluster with name or alias %q", name)
}

func (k ClusterConfigs) GetClusterConfigByBrn(brn brns.StackBrn) (*ClusterConfig, error) {

	if brn.EnvironmentName != "" && brn.ClusterName == "" && brn.StackName == "" {
		var clustersWithEnvironment []*ClusterConfig
		for _, c := range k {
			if c.Environment == brn.EnvironmentName {
				clustersWithEnvironment = append(clustersWithEnvironment, c)
			}
		}
		switch len(clustersWithEnvironment) {
		case 0:
			return nil, errors.Errorf("no cluster matched environment %s", brn)
		case 1:
			return clustersWithEnvironment[0], nil
		default:
			var candidateBrns []string
			for _, c := range clustersWithEnvironment {
				candidateBrns = append(candidateBrns, c.Brn.String())
				if c.IsDefaultCluster {
					return c, nil
				}
			}
			return nil, errors.Errorf("%d clusters matched hint %s, but none had isDefaultCluster=true; matches: %v", len(clustersWithEnvironment), brn, candidateBrns)
		}
	}

	var clusterConfig *ClusterConfig
	for _, c := range k {
		if c.Name == brn.ClusterName {
			clusterConfig = c
			break
		}
	}

	if clusterConfig == nil {
		return nil, errors.Errorf("no cluster matched cluster name %q from cluster path %q", brn.ClusterName, brn)
	}

	return clusterConfig, nil
}
/*

func (k *Cluster) HandleConfigureRequest(req ConfigureRequest) error {

	if configuredClusters[k.Name] {
		req.Log.Debugf("Already configured kubernetes cluster %q.", k.Name)
		return nil
	}

	switch req.Action.(type) {
	case ConfigureContextAction:
		return k.configureKubernetes(req)
	case ConfigureNamespacesAction:
		return k.configureNamespaces(req)
	case ConfigurePullSecretsAction:
		return k.configurePullSecrets(req)
	case ConfigureCertsAction:
		return k.configureCerts(req)
	}

	configuredClusters[k.Name] = true

	return nil

}


 */


func (k ClusterConfig) configureKubernetes(req ConfigureRequest) error {
	kubectl := Kubectl{
		Cluster:    k.Name,
		Kubeconfig: k.KubeconfigPath,
	}

	if kubectl.contextIsDefined(req.Brn.ClusterName) && !req.Force {
		req.Log.Warnf("Kubernetes context %q already exists (use --force to configure anyway).", req.Brn.ClusterBrn)
		return nil
	}

	if req.KubeConfigPath == "" {
		req.KubeConfigPath = k.GetKubeconfigPath()
	}

	if req.KubeConfigPath == "" {
		req.KubeConfigPath = os.ExpandEnv("$HOME/.kube/config")
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

	return nil
}
