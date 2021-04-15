package kube

import (
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/brns"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"os"
	"strings"
)

type ClusterConfig struct {
	StackTemplate    `yaml:",inline"`
	KubeconfigPath   string                 `yaml:"kubeconfigPath,omitempty"`
	Provider         string                 `yaml:"-"`
	EnvironmentAlias string                 `yaml:"environmentAlias,omitempty"`
	Environment      string                 `yaml:"environment,omitempty"`
	DefaultNamespace string                 `yaml:"defaultNamespace,omitempty"`
	Roles            core.ClusterRoles      `yaml:"roles,flow"`
	Protected        bool                   `yaml:"protected"`
	Oracle           *OracleClusterConfig   `yaml:"oracle,omitempty"`
	Minikube         *MinikubeConfig        `yaml:"minikube,omitempty"`
	Microk8s         *Microk8sConfig        `yaml:"microk8s,omitempty"`
	Amazon           *AmazonClusterConfig   `yaml:"amazon,omitempty"`
	Rancher          *RancherClusterConfig  `yaml:"rancher,omitempty"`
	ExternalCluster  *ExternalClusterConfig `yaml:"externalCluster,omitempty"`
	StackTemplates   []*StackTemplate       `yaml:"stackTemplates,omitempty"`
	IsDefaultCluster bool                   `yaml:"isDefaultCluster"`
	Aliases          []string               `yaml:"aliases,omitempty"`
	// Set by the environment during load
	PullSecrets []PullSecret  `yaml:"-"`
	Brn         brns.StackBrn `yaml:"-"`
}

type ClusterCert struct {
	SecretName string                `yaml:"secretName"`
	VaultUrl   string                `yaml:"vaultUrl"`
	VaultToken *command.CommandValue `yaml:"vaultToken"`
	VaultPath  string                `yaml:"vaultPath"`
	CommonName string                `yaml:"commonName"`
	AltNames   []string              `yaml:"altNames"`
}

func (c ClusterConfig) GetKubeconfigPath() string {
	if c.KubeconfigPath == "" {
		return os.ExpandEnv("$HOME/.kube/config")
	}
	return c.KubeconfigPath
}

type PullSecret struct {
	Name             string               `yaml:"name"`
	Domain           string               `yaml:"domain"`
	FromDockerConfig bool                 `yaml:"fromDockerConfig,omitempty"`
	Username         string               `yaml:"username,omitempty"`
	Password         command.CommandValue `yaml:"password,omitempty"`
}

func (c *ClusterConfig) SetFromPath(fp string) {
	c.FromPath = fp
	c.StackTemplate.SetFromPath(fp)
	for i := range c.StackTemplates {
		st := c.StackTemplates[i]
		st.SetFromPath(fp)
	}
}

func (f *ClusterConfig) MarshalYAML() (interface{}, error) {
	if f == nil {
		return nil, nil
	}
	type proxy ClusterConfig
	p := proxy(*f)

	return &p, nil
}

func (f *ClusterConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type proxy ClusterConfig
	var p proxy
	if f != nil {
		p = proxy(*f)
	}

	err := unmarshal(&p)

	if err == nil {
		*f = ClusterConfig(p)
		if f.Oracle != nil {
			f.Provider = "oracle"
		}
		if f.Minikube != nil {
			f.Provider = "minikube"
		}
		if f.Amazon != nil {
			f.Provider = "amazon"
		}
		if f.Rancher != nil {
			f.Provider = "rancher"
		}
		if f.Microk8s != nil {
			f.Provider = "microk8s"
		}
	}

	f.Brn = brns.NewStack(f.Environment, f.Name, DefaultStackName)

	return err
}

const DefaultRole core.ClusterRole = "default"

type ConfigureContextAction struct{}
type ConfigureCertsAction struct{}
type ConfigureNamespacesAction struct{}
type ConfigurePullSecretsAction struct{}

type ConfigureRequest struct {
	Action           interface{}
	Brn              brns.StackBrn
	KubeConfigPath   string
	Force            bool
	Log              *logrus.Entry
	ExecutionContext command.ExecutionContext
}

// cache of clusters configured this run, no need to configure theme again
var configuredClusters = map[string]bool{}

//
// func (k ClusterConfigs) HandleConfigureRequest(req ConfigureRequest) error {
// 	if req.Log == nil {
// 		req.Log = logrus.NewEntry(logrus.StandardLogger())
// 	}
//
// 	var err error
// 	var konfigs []*ClusterConfig
//
// 	if !req.Brn.IsEmpty() {
// 		konfig, kubeConfigErr := k.GetClusterConfigByBrn(req.Brn)
// 		if kubeConfigErr != nil {
// 			return kubeConfigErr
// 		}
// 		konfigs = []*ClusterConfig{konfig}
// 	}
//
// 	if len(konfigs) == 0 {
// 		return errors.Errorf("could not find any kube configs using brn %s", req.Brn)
// 	}
//
// 	for _, konfig := range konfigs {
//
// 		var cluster *Cluster
// 		cluster, err = NewCluster(*konfig, req.ExecutionContext)
// 		if err != nil {
// 			return err
// 		}
//
// 		err = cluster.HandleConfigureRequest(req)
// 		if err != nil {
// 			return err
// 		}
// 	}
//
// 	return nil
// }

func (e *ClusterConfig) GetValueSetCollection() values.ValueSetCollection {
	if e.StackTemplate.ValueOverrides == nil {
		return values.NewValueSetCollection()
	}
	return *e.StackTemplate.ValueOverrides
}

type NamespaceConfigs map[core.NamespaceRole]NamespaceConfig

func (n NamespaceConfigs) UniqueNames() []string {
	var out []string
	uniq := map[string]bool{}
	for _, ns := range n {
		if uniq[ns.Name] {
			continue
		}
		uniq[ns.Name] = true
		out = append(out, ns.Name)
	}
	return out
}

func (n NamespaceConfigs) ToStringMap() map[string]NamespaceConfig {
	out := map[string]NamespaceConfig{}
	for k, v := range n {
		out[string(k)] = v
	}
	return out
}

type NamespaceConfig struct {
	Name   string `yaml:"name"`
	Shared bool `yaml:"shared,omitempty"`
}

type appValueSetCollectionProvider struct {
	valueSetCollection values.ValueSetCollection
}

func (a appValueSetCollectionProvider) GetValueSetCollection() values.ValueSetCollection {
	return a.valueSetCollection
}

func (k Kubectl) contextIsDefined(name string) bool {
	out, err := k.Exec(
		"kubeconfig",
		"get-contexts",
		name,
	)
	if err != nil {
		return false
	}
	if strings.Contains(out, "error:") {
		return false
	}
	return true
}

type Kubectl struct {
	Namespace  string
	Cluster    string
	Kubeconfig string
}

func (k Kubectl) Exec(args ...string) (string, error) {
	if k.Namespace != "" {
		args = append(args, "--namespace", k.Namespace)
	}

	if k.Cluster != "" {
		args = append(args, "--cluster", k.Cluster)
	}

	if k.Kubeconfig != "" {
		args = append(args, "--kubeconfig", k.Kubeconfig)
	}

	out, err := pkg.NewShellExe("kubectl", args...).RunOut()
	if err != nil {
		return "", errors.Wrapf(err, "kubectl:%v", k)
	}
	return out, nil
}
