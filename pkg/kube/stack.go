package kube

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	vault "github.com/hashicorp/vault/api"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/brns"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/util/multierr"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	stackConfigMapKey = "data"
)

const (
	StackLabel = "bosun.aunalytics.com/stack"
)

type Stack struct {
	StackTemplate
	Brn     brns.StackBrn
	Cluster *Cluster

	state *StackState
}

func NewStack(cluster *Cluster, name string) (*Stack, error) {

	if name == "" || name == DefaultStackName {

		defaultStackTemplate := cluster.StackTemplate
		defaultStackTemplate.Name = DefaultStackName
		return &Stack{
			Brn:           brns.NewStack(cluster.Environment, cluster.Name, DefaultStackName),
			StackTemplate: defaultStackTemplate,
			Cluster:       cluster,
		}, nil
	}

	var stackTemplate *StackTemplate
	patterns := map[string]string{}

	var shortName string

	for _, candidate := range cluster.StackTemplates {
		patterns[candidate.Name] = candidate.NamePattern

		re, err := regexp.Compile(candidate.NamePattern)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid name pattern in template %q", candidate.Name)
		}

		m := re.FindStringSubmatch(name)
		switch len(m) {
		case 0:
			continue
		case 1:
			shortName = m[0]
			stackTemplate = candidate
		default:
			shortName = m[1]
			stackTemplate = candidate
		}
		if stackTemplate != nil {
			break
		}
	}

	if stackTemplate == nil {
		return nil, errors.Errorf("no stack template name pattern matched requested name %q; name patterns: %+v", name, patterns)
	}

	y, _ := yaml.MarshalString(stackTemplate)

	parameters := map[string]string{
		"Name":      name,
		"ShortName": shortName,
	}

	var renderedStackTemplate *StackTemplate

	rendered, err := pkg.NewTemplateBuilder(name + "-stack-template").WithTemplate(y).BuildAndExecute(parameters)
	if err != nil {
		return nil, err
	}

	err = yaml.UnmarshalString(rendered, &renderedStackTemplate)

	if err != nil {
		return nil, err
	}

	renderedStackTemplate.Name = name
	renderedStackTemplate.SetFromPath(stackTemplate.FromPath)

	return &Stack{
		StackTemplate: *renderedStackTemplate,
		Brn:           brns.NewStack(cluster.Brn.EnvironmentName, cluster.Brn.ClusterName, name),
		Cluster:       cluster,
	}, nil
}

func (c *Stack) GetState() (*StackState, error) {

	if c.state == nil {

		namespace := c.Cluster.DefaultNamespace
		if namespace == "" {
			namespace = "default"
		}

		configmapName := makeStackConfigmapName(c.Name)

		configmap, err := c.Cluster.Client.CoreV1().ConfigMaps(namespace).Get(configmapName, metav1.GetOptions{})
		if kerrors.IsNotFound(err) {
			c.state = &StackState{
				Name:         c.Name,
				DeployedApps: map[string]StackApp{},
			}
		} else if err != nil {
			return nil, err
		}

		err = yaml.Unmarshal([]byte(configmap.Data[stackConfigMapKey]), &c.state)
		if err != nil {
			return nil, err
		}

		if c.state == nil {
			c.state = &StackState{
				Name:         c.Name,
				DeployedApps: map[string]StackApp{},
			}
		}
	}

	return c.state, nil
}

func (c *Stack) UpdateApp(updates ...StackApp) error {

	state, err := c.GetState()

	if err != nil {
		return err
	}

	if state.DeployedApps == nil {
		state.DeployedApps = map[string]StackApp{}
	}

	for _, app := range updates {
		state.DeployedApps[app.Name] = app
	}

	return c.Save()
}

func (c *Stack) Save() error {

	client := c.Cluster.Client
	namespace := c.Cluster.GetDefaultNamespace()
	configmapName := makeStackConfigmapName(c.Name)

	var err error
	var configmap *v1.ConfigMap

	for {

		configmap, err = client.CoreV1().ConfigMaps(namespace).Get(configmapName, metav1.GetOptions{})
		if kerrors.IsNotFound(err) {
			defaultData, _ := yaml.MarshalString(c.state)

			configmap, err = client.CoreV1().ConfigMaps(namespace).Create(&v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: configmapName,
					Labels: map[string]string{
						StackLabel: c.Name,
					},
				},
				Data: map[string]string{
					stackConfigMapKey: defaultData,
				},
				BinaryData: nil,
			})

			return err
		} else if err != nil {
			return err
		}

		configmap.Data[stackConfigMapKey], _ = yaml.MarshalString(c.state)

		_, err = client.CoreV1().ConfigMaps(namespace).Update(configmap)

		if kerrors.IsConflict(err) {
			pkg.Log.Warn("Conflict while updating deployed apps, will try again.")
			<-time.After(1 * time.Second)
		} else if err != nil {
			return err
		} else {
			break
		}
	}

	return err
}

func (k Stack) ConfigureNamespaces() error {

	for role, ns := range k.StackTemplate.Namespaces {
		log := k.Cluster.ctx.Log().WithField("namespace", ns.Name).WithField("namespace-role", role)
		namespace := &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns.Name,
				Labels: map[string]string{
					LabelNamespaceRole: string(role),
				},
			},
		}
		_, err := k.Cluster.Client.CoreV1().Namespaces().Create(namespace)
		if kerrors.IsAlreadyExists(err) {
			_, err = k.Cluster.Client.CoreV1().Namespaces().Update(namespace)
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

func (k Stack) ConfigureCerts() error {

	k.Cluster.ctx.Log().Infof("Configuring certs (%d certs to configure)", len(k.Certs))

	errs := multierr.New()

	for _, certConfig := range k.Certs {

		log := k.Cluster.ctx.Log().WithField("cert", certConfig.SecretName)

		log.Infof("Creating cert with common name %q and alt names %v", certConfig.CommonName, certConfig.AltNames)

		token, err := certConfig.VaultToken.Resolve(k.Cluster.ctx)
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

			kubectl := k.Cluster.Kubectl

			_, _ = kubectl.Exec("delete", "secret", certConfig.SecretName, "-n", namespaceName)

			_, err = kubectl.Exec("create", "secret", "tls", certConfig.SecretName, "--cert", certPath, "--key", keyPath, "-n", namespaceName)
			if err != nil {
				errs.Collect(err)
			}
		}
	}

	k.Cluster.ctx.Log().Infof("Done configuring certs.")

	return errs.ToError()
}

func (k Stack) ConfigurePullSecrets() error {

	for _, namespaceName := range k.StackTemplate.Namespaces.UniqueNames() {

		for _, pullSecret := range k.Cluster.PullSecrets {

			log := k.Cluster.ctx.Log().WithField("pull-secret", pullSecret.Name).WithField("namespace", namespaceName)

			var password string
			var username string
			var err error

			client := k.Cluster.Client

			if pullSecret.FromDockerConfig {
				var dockerConfig map[string]interface{}
				dockerConfigPath, ok := os.LookupEnv("DOCKER_CONFIG")
				if !ok {
					dockerConfigPath = os.ExpandEnv("$HOME/.docker/config.json")
				}
				data, dockerFileErr := ioutil.ReadFile(dockerConfigPath)
				if dockerFileErr != nil {
					return errors.Errorf("error reading docker kubeconfig from %q: %s", dockerConfigPath, dockerFileErr)
				}

				dockerFileErr = json.Unmarshal(data, &dockerConfig)
				if dockerFileErr != nil {
					return errors.Errorf("error docker kubeconfig from %q, file was invalid: %s", dockerConfigPath, dockerFileErr)
				}

				auths, ok := dockerConfig["auths"].(map[string]interface{})

				entry, ok := auths[pullSecret.Domain].(map[string]interface{})
				if !ok {
					return errors.Errorf("no %q entry in docker kubeconfig, you should docker login first", pullSecret.Domain)
				}
				authBase64, _ := entry["auth"].(string)
				auth, dockerFileErr := base64.StdEncoding.DecodeString(authBase64)
				if dockerFileErr != nil {
					return errors.Errorf("invalid %q entry in docker kubeconfig, you should docker login first: %s", pullSecret.Domain, dockerFileErr)
				}
				segs := strings.Split(string(auth), ":")
				username, password = segs[0], segs[1]
			} else {

				username = pullSecret.Username
				password, err = pullSecret.Password.Resolve(k.Cluster.ctx)
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
				if k.Cluster.ctx.GetParameters().Force {
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
				log.Infof("Created pull secret.")
			}
		}

	}

	return nil
}

func (e *Stack) GetValueSetCollection() values.ValueSetCollection {
	if e.StackTemplate.ValueOverrides == nil {
		return values.NewValueSetCollection()
	}
	return *e.StackTemplate.ValueOverrides
}

func (k *Stack) GetNamespace(role core.NamespaceRole) (NamespaceConfig, error) {
	if ns, ok := k.StackTemplate.Namespaces[role]; ok {
		return ns, nil
	}
	return NamespaceConfig{}, errors.Errorf("kubernetes cluster kubeconfig %q does not have a namespace for the role %q", k.StackTemplate.Namespaces, role)
}

// GetAppValueSetCollectionProvider returns a ValuesSetCollectionProvider that will provide any values set collection
// defined in this cluster for a specific app. If none is defined, an instance that does nothing will be returned.

func (c *Stack) GetAppValueSetCollectionProvider(appName string) values.ValueSetCollectionProvider {

	if appValueOverride, ok := c.StackTemplate.Apps[appName]; ok {
		return appValueSetCollectionProvider{
			valueSetCollection: appValueOverride,
		}
	}

	return appValueSetCollectionProvider{
		valueSetCollection: values.NewValueSetCollection(),
	}
}

func (s Stack) IsAppDisabled(name string) bool {
	app, ok := s.StackTemplate.Apps[name]

	return !(ok && !app.Disabled)
}

func (c *Stack) Destroy() error {

	log := c.Cluster.ctx.Log()

	client, err := dynamic.NewForConfig(c.Cluster.kubeconfig)
	if err != nil {
		return err
	}

	log.Warn("Deleting tenants first to avoid operator issues...")

	tenantRes := schema.GroupVersionResource{Group: "naveego.com", Version: "v1", Resource: "tenants"}

	tenantNamespace := c.StackTemplate.Namespaces["tenants"].Name
	err = client.Resource(tenantRes).Namespace(tenantNamespace).DeleteCollection(&metav1.DeleteOptions{}, metav1.ListOptions{})

	if err != nil {
		return err
	}

	log.Warn("Deleted tenants")

	deleted := map[string]bool{}

	for _, ns := range c.StackTemplate.Namespaces {

		if ns.Shared {
			log.Infof("Skipping deletion of namespace %q because it is marked as shared.", ns.Name)
			continue
		}

		if deleted[ns.Name] {
			continue
		}

		log.Warnf("Deleting namespace %q...", ns.Name)

		err = c.Cluster.Client.CoreV1().Namespaces().Delete(ns.Name, &metav1.DeleteOptions{})
		if err != nil {
			return errors.Wrapf(err, "deleting namespace %q", ns.Name)
		}

		deleted[ns.Name] = true
	}

	log.Warnf("Deleting stack history...")
	err = c.Cluster.Client.CoreV1().ConfigMaps(c.Cluster.GetDefaultNamespace()).Delete(makeStackConfigmapName(c.Name), &metav1.DeleteOptions{})

	if err != nil {
		return err
	}

	log.Warnf("Done deleting stack.")

	return nil

}
