package kube

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environmentvariables"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"strings"
)

const DefaultRole core.ClusterRole = "default"

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
	Name             string
	Role             core.ClusterRole
	KubeConfigPath   string
	Force            bool
	Log              *logrus.Entry
	ExecutionContext command.ExecutionContext
	PullSecrets      []PullSecret
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
	var konfigs []*ClusterConfig

	if req.Name != "" {
		konfig, err := k.GetKubeConfigDefinitionByName(req.Name)
		if err != nil {
			return err
		}
		konfigs = []*ClusterConfig{konfig}
	} else {
		konfigs, err = k.GetKubeConfigDefinitionsByRole(req.Role)
		if err != nil {
			return err
		}
	}

	if len(konfigs) == 0 {
		return errors.Errorf("could not find any kube configs")
	}

	for _, konfig := range konfigs {

		if configuredClusters[konfig.Name] {
			req.Log.Debugf("Already configured kubernetes cluster %q.", konfig.Name)
			return nil
		}

		err = konfig.configureKubernetes(req)
		if err != nil {
			return err
		}

		configuredClusters[konfig.Name] = true
	}

	return nil
}

func (k ConfigDefinitions) GetKubeConfigDefinitionsByRole(role core.ClusterRole) ([]*ClusterConfig, error) {

	if role == "" {
		role = DefaultRole
	}
	var out []*ClusterConfig
	for _, c := range k {
		for _, r := range c.Roles {
			if r == role {
				out = append(out, c)
			}
		}
	}
	if len(out) > 0 {
		return out, nil
	}

	return nil, errors.Errorf("no cluster definition had role %q", role)
}

type ClusterConfig struct {
	core.ConfigShared `yaml:",inline"`
	Provider          string                           `yaml:"-"`
	EnvironmentAlias  string                           `yaml:"environmentAlias,omitempty"`
	Roles             core.ClusterRoles                `yaml:"roles,flow"`
	Variables         []*environmentvariables.Variable `yaml:"variables,omitempty"`
	ValueOverrides    *values.ValueSetCollection       `yaml:"valueOverrides,omitempty"`
	Oracle            *OracleClusterConfig             `yaml:"oracle,omitempty"`
	Minikube          *MinikubeConfig                  `yaml:"minikube,omitempty"`
	Amazon            *AmazonClusterConfig             `yaml:"amazon,omitempty"`
	Rancher           *RancherClusterConfig            `yaml:"rancher,omitempty"`
	Namespaces        NamespaceConfigs                 `yaml:"namespaces"`
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
		if f.Rancher != nil {
			f.Provider = "rancher"
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

func (k ClusterConfig) configureKubernetes(req ConfigureKubeContextRequest) error {
	req.Name = k.Name

	if contextIsDefined(req.Name) && !req.Force {
		req.Log.Debugf("Kubernetes context %q already exists (use --force to configure anyway).", req.Name)
		return nil
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
	} else {
		return errors.Errorf("no recognized kube vendor found on %q", k.Name)
	}

	client, err := GetKubeClientWithContext(k.Name)
	if err != nil {
		return err
	}

	for role, ns := range k.Namespaces {
		req.Log.Infof("Creating namespace %q with role %q.", ns.Name, role)
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
			req.Log.Infof("Namespace already exists, updating...")
			_, err = client.CoreV1().Namespaces().Update(namespace)
		}
		if err != nil {
			return errors.Wrapf(err, "create or update namespace %q with role %q", ns.Name, role)
		}

		for _, pullSecret := range req.PullSecrets {

			if req.ExecutionContext == nil {
				req.Log.Warnf("No execution context provided, cannot create pull secret %q in namespace %q", pullSecret.Name, ns.Name)
				continue
			}

			req.Log.Infof("Creating or updating pull secret %q in namespace %q...", pullSecret.Name, ns.Name)

			var password string
			var username string

			if pullSecret.FromDockerConfig {
				var dockerConfig map[string]interface{}
				dockerConfigPath, ok := os.LookupEnv("DOCKER_CONFIG")
				if !ok {
					dockerConfigPath = os.ExpandEnv("$HOME/.docker/config.json")
				}
				data, err := ioutil.ReadFile(dockerConfigPath)
				if err != nil {
					return errors.Errorf("error reading docker config from %q: %s", dockerConfigPath, err)
				}

				err = json.Unmarshal(data, &dockerConfig)
				if err != nil {
					return errors.Errorf("error docker config from %q, file was invalid: %s", dockerConfigPath, err)
				}

				auths, ok := dockerConfig["auths"].(map[string]interface{})

				entry, ok := auths[pullSecret.Domain].(map[string]interface{})
				if !ok {
					return errors.Errorf("no %q entry in docker config, you should docker login first", pullSecret.Domain)
				}
				authBase64, _ := entry["auth"].(string)
				auth, err := base64.StdEncoding.DecodeString(authBase64)
				if err != nil {
					return errors.Errorf("invalid %q entry in docker config, you should docker login first: %s", pullSecret.Domain, err)
				}
				segs := strings.Split(string(auth), ":")
				username, password = segs[0], segs[1]
			} else {

				username = pullSecret.Username
				password, err = pullSecret.Password.Resolve(req.ExecutionContext)
				if err != nil {
					req.Log.Errorf("Could not resolve password for pull secret %q in namespace %q: %s", pullSecret.Name, ns.Name, err)
					continue
				}
			}

			auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", username, password)))

			dockerConfig := map[string]interface{}{
				"auths": map[string]interface{}{
					pullSecret.Domain: map[string]interface{}{
						"username": username,
						"password": password,
						"email":    username,
						"auth":     auth,
					},
				},
			}

			dockerConfigJSON, err := json.Marshal(dockerConfig)

			if err != nil {
				return errors.Wrap(err, "marshall dockerconfigjson")
			}

			secret := &v1.Secret{
				Type: v1.SecretTypeDockerConfigJson,
				ObjectMeta: metav1.ObjectMeta{
					Name: pullSecret.Name,
				},
				StringData: map[string]string{
					".dockerconfigjson": string(dockerConfigJSON),
				},
			}
			_, err = client.CoreV1().Secrets(namespace.Name).Create(secret)
			if kerrors.IsAlreadyExists(err) {
				req.Log.Infof("Pull secret already exists, updating...")
				_, err = client.CoreV1().Secrets(namespace.Name).Update(secret)
			}
			if err != nil {
				return errors.Wrapf(err, "create or update pull secret %q in namespace %q", pullSecret.Name, namespace.Name)
			}

			req.Log.Infof("Done creating or updating pull secret %q in namespace %q.", pullSecret.Name, ns.Name)

		}

		req.Log.Infof("Done creating or updating namespace %q.", ns.Name)
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
