package kube

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	vault "github.com/hashicorp/vault/api"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/brns"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environmentvariables"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"path/filepath"
	"strings"
)

type ClusterConfig struct {
	core.ConfigShared `yaml:",inline"`
	StackConfig       `yaml:",inline"`
	KubeconfigPath    string                 `yaml:"kubeconfigPath,omitempty"`
	Provider          string                 `yaml:"-"`
	EnvironmentAlias  string                 `yaml:"environmentAlias,omitempty"`
	Environment       string                 `yaml:"environment,omitempty"`
	Roles             core.ClusterRoles      `yaml:"roles,flow"`
	Protected         bool                   `yaml:"protected"`
	Oracle            *OracleClusterConfig   `yaml:"oracle,omitempty"`
	Minikube          *MinikubeConfig        `yaml:"minikube,omitempty"`
	Microk8s          *Microk8sConfig        `yaml:"microk8s,omitempty"`
	Amazon            *AmazonClusterConfig   `yaml:"amazon,omitempty"`
	Rancher           *RancherClusterConfig  `yaml:"rancher,omitempty"`
	ExternalCluster   *ExternalClusterConfig `yaml:"externalCluster,omitempty"`
	StackTemplate     *StackConfig           `yaml:"stackTemplate,omitempty"`
	Brn               brns.Stack             `yaml:"-"`
	IsDefaultCluster  bool                   `yaml:"isDefaultCluster"`
	Aliases           []string               `yaml:"aliases,omitempty"`
}

type StackConfig struct {
	Variables      []*environmentvariables.Variable     `yaml:"variables,omitempty"`
	Namespaces     NamespaceConfigs                     `yaml:"namespaces"`
	Apps           map[string]values.ValueSetCollection `yaml:"apps"`
	Certs          []ClusterCert                        `yaml:"certs"`
	ValueOverrides *values.ValueSetCollection           `yaml:"valueOverrides,omitempty"`
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
	return os.ExpandEnv(c.KubeconfigPath)
}

// IsAppDisabled returns true if the app is disabled for the cluster
// Apps are assumed to be enabled for the cluster unless explicitly disabled
func (c ClusterConfig) IsAppDisabled(appName string) bool {
	v, ok := c.Apps[appName]
	return ok && v.Disabled
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
		if f.Microk8s != nil {
			f.Provider = "microk8s"
		}
	}

	f.Brn = brns.NewStack(f.Environment, f.Name, "")

	return err
}

const DefaultRole core.ClusterRole = "default"

type ClusterConfigs []*ClusterConfig

func (k ClusterConfigs) Headers() []string {
	return []string{
		"Name",
		"KubeConfig",
		"Environment",
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

func (k ClusterConfigs) GetByBrn(brn brns.Stack) (*ClusterConfig, error) {

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

	if brn.StackName != "" {
		return clusterConfig.RenderStack(brn.StackName)
	}

	return clusterConfig, nil
}

func (c *ClusterConfig) RenderStack(subclusterName string) (*ClusterConfig, error) {

	if c.StackTemplate == nil {
		return nil, errors.Errorf("cluster %q has no subclusterTemplate to render %q", c.Name, subclusterName)
	}

	y, _ := yaml.MarshalString(c.StackTemplate)

	parameters := map[string]string{
		"Name": subclusterName,
	}

	rendered, err := pkg.NewTemplateBuilder(c.Name + "-subclusterTemplate").WithTemplate(y).BuildAndExecute(parameters)
	if err != nil {
		return nil, err
	}

	panic("Steve needs to fix this")


	var clusterConfig *ClusterConfig
	err = yaml.UnmarshalString(rendered, &clusterConfig)

	clusterConfig.Brn = brns.NewStack(c.Environment, c.Name, subclusterName)

	clusterConfig.SetFromPath(c.FromPath)

	return clusterConfig, err
}

type ConfigureContextAction struct{}
type ConfigureCertsAction struct{}
type ConfigureNamespacesAction struct{}
type ConfigurePullSecretsAction struct{}

type ConfigureRequest struct {
	Action           interface{}
	Brn              brns.Stack
	KubeConfigPath   string
	Force            bool
	Log              *logrus.Entry
	ExecutionContext command.ExecutionContext
	PullSecrets      []PullSecret
}

// cache of clusters configured this run, no need to configure theme again
var configuredClusters = map[string]bool{}

func (k ClusterConfigs) HandleConfigureRequest(req ConfigureRequest) error {
	if req.Log == nil {
		req.Log = logrus.NewEntry(logrus.StandardLogger())
	}

	var err error
	var konfigs []*ClusterConfig

	if !req.Brn.IsEmpty() {
		konfig, kubeConfigErr := k.GetByBrn(req.Brn)
		if kubeConfigErr != nil {
			return kubeConfigErr
		}
		konfigs = []*ClusterConfig{konfig}
	}

	if len(konfigs) == 0 {
		return errors.Errorf("could not find any kube configs using brn %s", req.Brn)
	}

	for _, konfig := range konfigs {

		err = konfig.HandleConfigureRequest(req)
		if err != nil {
			return err
		}
	}

	return nil
}

func (k ClusterConfig) HandleConfigureRequest(req ConfigureRequest) error {

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

func (k ClusterConfigs) GetKubeConfigDefinitionsByRole(role core.ClusterRole) ([]*ClusterConfig, error) {

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

func (k ClusterConfig) configureKubernetes(req ConfigureRequest) error {
	kubectl := Kubectl{
		Cluster:    k.Name,
		Kubeconfig: k.KubeconfigPath,
	}

	if kubectl.contextIsDefined(req.Brn.ClusterName) && !req.Force {
		req.Log.Warnf("Kubernetes context %q already exists (use --force to configure anyway).", req.Brn.Cluster)
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

	err := k.configureNamespaces(req)
	if err != nil {
		return errors.Wrap(err, "could not configure namespaces")
	}
	err = k.configurePullSecrets(req)
	if err != nil {
		return errors.Wrap(err, "could not configure pull secrets")
	}

	return err
}

func (k ClusterConfig) configureNamespaces(req ConfigureRequest) error {

	client, err := GetKubeClientWithContext(req.KubeConfigPath, k.Name)
	if err != nil {
		return err
	}

	for role, ns := range k.Namespaces {
		log := req.Log.WithField("namespace", ns.Name).WithField("namespace-role", role)
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
			_, err = client.CoreV1().Namespaces().Update(namespace)
			if err != nil {
				return errors.Wrapf(err, "update namespace %q with role %q", ns.Name, role)

			}
			log.Infof("Updated namespace.")
		} else if err != nil {
			return errors.Wrapf(err, "create or update namespace %q with role %q", ns.Name, role)
		} else {
			log.Info("Created namespace")
		}
	}

	return nil
}

func (k ClusterConfig) configurePullSecrets(req ConfigureRequest) error {

	for _, ns := range k.Namespaces {

		for _, pullSecret := range req.PullSecrets {

			if req.ExecutionContext == nil {
				req.Log.Warnf("No execution context provided, cannot create pull secret %q in namespace %q", pullSecret.Name, ns.Name)
				continue
			}

			err := createOrUpdatePullSecret(req, k.Name, ns.Name, pullSecret)
			if err != nil {
				return err
			}
		}

	}

	return nil
}

func (k ClusterConfig) configureCerts(req ConfigureRequest) error {

	req.Log.Infof("Configuring certs (%d certs to configure)", len(k.Certs))

	for _, certConfig := range k.Certs {

		log := req.Log.WithField("cert", certConfig.SecretName)

		log.Infof("Creating cert with common name %q and alt names %v", certConfig.CommonName, certConfig.AltNames)

		token, err := certConfig.VaultToken.Resolve(req.ExecutionContext)
		if err != nil {
			return errors.Wrap(err, "could not resolve token for creating cert")
		}
		if token == "" {
			return errors.Errorf("token resolved using command %v was empty", certConfig.VaultToken)
		}

		log.Infof("Acquiring cert from vault using url=%s, path=%s, token=%s...", certConfig.VaultUrl, certConfig.VaultPath, token[0:3])

		vaultClient, err := vault.NewClient(&vault.Config{
			Address: certConfig.VaultUrl,
		})
		if err != nil {
			return err
		}

		vaultClient.SetToken(token)

		data := map[string]interface{}{
			"common_name": certConfig.CommonName,
			"alt_names":   strings.Join(certConfig.AltNames, ","),
			"ttl":         "9504h",
		}

		certData, err := vaultClient.Logical().Write(certConfig.VaultPath, data)
		if err != nil {
			return err
		}

		certificate := certData.Data["certificate"].(string)
		key := certData.Data["private_key"].(string)

		tempDir, _ := os.MkdirTemp(os.TempDir(), k.Name+"-certs-*")
		defer os.RemoveAll(tempDir)

		certPath := filepath.Join(tempDir, "cert.pem")
		keyPath := filepath.Join(tempDir, "cert.key")

		err = ioutil.WriteFile(certPath, []byte(certificate), 0600)
		if err != nil {
			return errors.WithStack(err)
		}

		err = ioutil.WriteFile(keyPath, []byte(key), 0600)
		if err != nil {
			return errors.WithStack(err)
		}

		namespaceNames := map[string]bool{}

		for _, ns := range k.Namespaces {
			namespaceNames[ns.Name] = true
		}

		for namespaceName := range namespaceNames {
			log.Infof("Deploying cert to namespace %q", namespaceName)

			kubectl := Kubectl{Kubeconfig: req.KubeConfigPath, Namespace: namespaceName}

			_, _ = kubectl.Exec("delete", "secret", certConfig.SecretName)

			_, err = kubectl.Exec("create", "secret", "tls", certConfig.SecretName, "--cert", certPath, "--key", keyPath)
			if err != nil {
				return err
			}
		}
	}

	req.Log.Infof("Done configuring certs.")

	return nil
}

// GetAppValueSetCollectionProvider returns a ValuesSetCollectionProvider that will provide any values set collection
// defined in this cluster for a specific app. If none is defined, an instance that does nothing will be returned.

func (c *ClusterConfig) GetAppValueSetCollectionProvider(appName string) values.ValueSetCollectionProvider {

	if appValueOverride, ok := c.Apps[appName]; ok {
		return appValueSetCollectionProvider{
			valueSetCollection: appValueOverride,
		}
	}

	return appValueSetCollectionProvider{
		valueSetCollection: values.NewValueSetCollection(),
	}
}

type appValueSetCollectionProvider struct {
	valueSetCollection values.ValueSetCollection
}

func (a appValueSetCollectionProvider) GetValueSetCollection() values.ValueSetCollection {
	return a.valueSetCollection
}

func createOrUpdatePullSecret(req ConfigureRequest, clusterName, namespaceName string, pullSecret PullSecret) error {

	log := req.Log.WithField("pull-secret", pullSecret.Name).WithField("namespace", namespaceName)

	var password string
	var username string
	var err error

	client, err := GetKubeClientWithContext(req.KubeConfigPath, clusterName)

	if pullSecret.FromDockerConfig {
		var dockerConfig map[string]interface{}
		dockerConfigPath, ok := os.LookupEnv("DOCKER_CONFIG")
		if !ok {
			dockerConfigPath = os.ExpandEnv("$HOME/.docker/config.json")
		}
		data, dockerFileErr := ioutil.ReadFile(dockerConfigPath)
		if dockerFileErr != nil {
			return errors.Errorf("error reading docker config from %q: %s", dockerConfigPath, dockerFileErr)
		}

		dockerFileErr = json.Unmarshal(data, &dockerConfig)
		if dockerFileErr != nil {
			return errors.Errorf("error docker config from %q, file was invalid: %s", dockerConfigPath, dockerFileErr)
		}

		auths, ok := dockerConfig["auths"].(map[string]interface{})

		entry, ok := auths[pullSecret.Domain].(map[string]interface{})
		if !ok {
			return errors.Errorf("no %q entry in docker config, you should docker login first", pullSecret.Domain)
		}
		authBase64, _ := entry["auth"].(string)
		auth, dockerFileErr := base64.StdEncoding.DecodeString(authBase64)
		if dockerFileErr != nil {
			return errors.Errorf("invalid %q entry in docker config, you should docker login first: %s", pullSecret.Domain, dockerFileErr)
		}
		segs := strings.Split(string(auth), ":")
		username, password = segs[0], segs[1]
	} else {

		username = pullSecret.Username
		password, err = pullSecret.Password.Resolve(req.ExecutionContext)
		if err != nil {
			// req.Log.Errorf("Could not resolve password for pull secret %q in namespace %q: %s", pullSecret.Name, ns.Name, err)
			return err
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
	_, err = client.CoreV1().Secrets(namespaceName).Create(secret)
	if kerrors.IsAlreadyExists(err) {
		if req.Force {
			_, err = client.CoreV1().Secrets(namespaceName).Update(secret)
			if err != nil {
				return errors.Wrapf(err, "update pull secret %q in namespace %q", pullSecret.Name, namespaceName)
			}
			log.Info("Updated existing pull secret.")
		} else {
			log.Info("Pull secret already exists, run with --force to force update.")
		}
	} else if err != nil {
		return errors.Wrapf(err, "create pull secret %q in namespace %q", pullSecret.Name, namespaceName)
	} else {
		req.Log.Infof("Created pull secret.")
	}

	return nil
}

func (k Kubectl) contextIsDefined(name string) bool {
	out, err := k.Exec(
		"config",
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
