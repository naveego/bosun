package kube

import (
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environmentvariables"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

const DefaultRole core.ClusterRole = "default"

type CommandContext struct {
	KubeConfigPath string
	Force          bool
	Name           string
	Log            *logrus.Entry
}

type ConfigDefinitions []*ClusterConfig

func (k ConfigDefinitions) GetKubeConfigDefinitionByName(name string) (*ClusterConfig, error) {
	for _, c := range k {
		if c.Name == name {
			return c, nil
		}
	}
	return nil, errors.Errorf("no cluster definition with name %q", name)
}

type ConfigureKubeContextRequest struct {
	Name           string
	Role           core.ClusterRole
	KubeConfigPath string
	Force          bool
	Log            *logrus.Entry
}

// cache of clusters configured this run, no need to configure theme again
var configuredClusters = map[string]bool{}

func (k ConfigDefinitions) HandleConfigureKubeContextRequest(req ConfigureKubeContextRequest) error {
	if req.Log == nil {
		req.Log = logrus.NewEntry(logrus.StandardLogger())
	}

	if req.Role == "" {
		req.Role = DefaultRole
	}

	var err error
	var konfig *ClusterConfig

	if req.Name != "" {
		konfig, err = k.GetKubeConfigDefinitionByName(req.Name)
		if err != nil {
			return err
		}
	} else {
		konfig, err = k.GetKubeConfigDefinitionByRole(req.Role)
		if err != nil {
			return err
		}
	}

	if konfig == nil {
		return errors.Errorf("could not find any kube configs")
	}

	if configuredClusters[konfig.Name] {
		req.Log.Debugf("Already configured kubernetes cluster %q.", konfig.Name)
		return nil
	}

	ktx := CommandContext{
		Name:           konfig.Name,
		Force:          req.Force,
		KubeConfigPath: req.KubeConfigPath,
		Log:            req.Log,
	}

	err = konfig.configureKubernetes(ktx)
	if err != nil {
		return err
	}

	configuredClusters[konfig.Name] = true

	return nil
}

func (k ConfigDefinitions) GetKubeConfigDefinitionByRole(role core.ClusterRole) (*ClusterConfig, error) {

	if role == "" {
		role = DefaultRole
	}
	for _, c := range k {
		for _, r := range c.Roles {
			if r == role {
				return c, nil
			}
		}
	}

	return nil, errors.Errorf("no cluster definition had role %q", role)
}

type ClusterConfig struct {
	core.ConfigShared `yaml:",inline"`
	Provider          string                           `yaml:"-"`
	Roles             core.ClusterRoles                `yaml:"roles,flow"`
	Variables         []*environmentvariables.Variable `yaml:"variables,omitempty"`
	ValueOverrides    *values.ValueSetCollection       `yaml:"valueOverrides,omitempty"`
	Oracle            *OracleClusterConfig             `yaml:"oracle,omitempty"`
	Minikube          *MinikubeConfig                  `yaml:"minikube,omitempty"`
	Amazon            *AmazonClusterConfig             `yaml:"amazon,omitempty"`
	Namespaces        NamespaceConfigs                 `yaml:"namespaces"`
}

func (c *ClusterConfig) SetFromPath(fp string) {
	c.FromPath = fp
	for i, v := range c.Variables {
		v.FromPath = fp
		c.Variables[i] = v
	}
	if c.ValueOverrides != nil {
		c.ValueOverrides.SetFromPath(fp)
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
	}

	return err
}

func (e *ClusterConfig) GetValueSetCollection() values.ValueSetCollection {
	if e.ValueOverrides == nil {
		return values.NewValueSetCollection()
	}
	return *e.ValueOverrides
}

type NamespaceConfigs map[core.NamespaceRole]NamespaceConfig

func (n NamespaceConfigs) ToStringMap() map[string]NamespaceConfig {
	out := map[string]NamespaceConfig{}
	for k, v := range n {
		out[string(k)] = v
	}
	return out
}

type NamespaceConfig struct {
	Name string `yaml:"name"`
}

func (k ClusterConfig) GetNamespace(role core.NamespaceRole) (NamespaceConfig, error) {
	if ns, ok := k.Namespaces[role]; ok {
		return ns, nil
	}
	return NamespaceConfig{}, errors.Errorf("kubernetes cluster config %q does not have a namespace for the role %q", k.Namespaces, role)
}

func (k ClusterConfig) configureKubernetes(ctx CommandContext) error {
	ctx.Name = k.Name

	if contextIsDefined(ctx.Name) && !ctx.Force {
		ctx.Log.Debugf("Kubernetes context %q already exists (use --force to configure anyway).", ctx.Name)
		return nil
	}

	if k.Oracle != nil {
		ctx.Log.Infof("Configuring Oracle cluster %q...", k.Name)

		if err := k.Oracle.configureKubernetes(ctx); err != nil {
			return err
		}
	} else if k.Minikube != nil {
		ctx.Log.Infof("Configuring minikube cluster %q...", k.Name)

		if err := k.Minikube.configureKubernetes(ctx); err != nil {
			return err
		}
	} else if k.Amazon != nil {
		ctx.Log.Infof("Configuring Amazon cluster %q...", k.Name)

		if err := k.Amazon.configureKubernetes(ctx); err != nil {
			return err
		}
	} else {
		return errors.Errorf("no recognized kube vendor found on %q", k.Name)
	}

	client, err := GetKubeClientWithContext(k.Name)
	if err != nil {
		return err
	}

	for role, ns := range k.Namespaces {
		ctx.Log.Infof("Creating namespace %q with role %q.", ns.Name, role)
		namespace := &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns.Name,
				Labels: map[string]string{
					LabelNamespaceRole: string(role),
				},
			},
		}
		_, err = client.CoreV1().Namespaces().Create(namespace)
		if kerrors.IsAlreadyExists(err) {
			ctx.Log.Infof("Namespace already exists, updating...")
			_, err = client.CoreV1().Namespaces().Update(namespace)
		}
		if err != nil {
			return errors.Wrapf(err, "create or update namespace %q with role %q", ns.Name, role)
		}
	}

	return nil
}

func contextIsDefined(name string) bool {
	out, err := pkg.NewShellExe("kubectl",
		"config",
		"get-contexts",
		name,
	).RunOut()
	if err != nil {
		return false
	}
	if strings.Contains(out, "error:") {
		return false
	}
	return true
}
