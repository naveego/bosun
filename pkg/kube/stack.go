package kube

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	vault "github.com/hashicorp/vault/api"
	"github.com/naveego/bosun/pkg/brns"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/util/multierr"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"os"
	"path/filepath"
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

	state           *StackState
	// Set to true if we have reason to think that this stack has been defined and deployed
	BelievedToExist bool
}

func NewStackFromState(cluster *Cluster, state *StackState) (*Stack, error) {

	if cluster == nil {
		panic("cluster must not be nil")
	}

	if state == nil {
		panic("state must not be nil")
	}

	stack := &Stack{
		Brn:           brns.NewStack(cluster.Environment, cluster.Name, state.Name),
		Cluster:       cluster,
	}

	var hit bool

	if state.TemplateName == "" || state.TemplateName == DefaultStackName {
		stack.StackTemplate = cluster.StackTemplate
		stack.StackTemplate.Name = DefaultStackName
		hit = true
	} else {
		for _, stackTemplate := range cluster.StackTemplates {
			if stackTemplate.Name == state.TemplateName {
				renderedTemplate, err := stackTemplate.Render(state.Name)
				if err != nil {
					return nil, err
				}
				stack.StackTemplate = *renderedTemplate
				hit = true
				break
			}
		}
	}

	if !hit {
		return nil, errors.Errorf("no stack template matched name %q", state.TemplateName)
	}

	stack.state = state

	return stack, nil

}

func NewStack(cluster *Cluster, name string, templateName string, template StackTemplate) (*Stack, error) {

	if name == "" || name == DefaultStackName {
		defaultStackTemplate := cluster.StackTemplate
		defaultStackTemplate.Name = DefaultStackName

		return &Stack{
			Brn:           brns.NewStack(cluster.Environment, cluster.Name, DefaultStackName),
			StackTemplate: defaultStackTemplate,
			Cluster:       cluster,
			state: &StackState{
				Name:          name,
				TemplateName:  DefaultStackName,
				StoryID:       "",
				Uninitialized: true,
				DeployedApps:  nil,
			},
		}, nil
	}

	renderedStackTemplate, err := template.Render(name)
	if err != nil {
		return nil, err
	}

	return &Stack{
		StackTemplate: *renderedStackTemplate,
		Brn:           brns.NewStack(cluster.Brn.EnvironmentName, cluster.Brn.ClusterName, name),
		Cluster:       cluster,
		state: &StackState{
			Name:          name,
			TemplateName:  templateName,
			StoryID:       "",
			Uninitialized: true,
			DeployedApps:  nil,
		},
	}, nil
}

func (c *Stack) GetState(refresh bool) (*StackState, error) {
	if c.state == nil || refresh {

		var err error
		c.state, err = c.Cluster.GetStackState(c.Name)
		if err != nil {
			if c.Name == DefaultStackName {
				// default stack in a cluster which has not been initialized yet
				c.state = &StackState{}
				err = c.Save()
				return c.state, err
			}
			return nil, err
		}
	}

	return c.state, nil
}

func (c *Stack) UpdateApp(updates ...StackApp) error {

	state, err := c.GetState(false)

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

	c.state.Uninitialized = false
	c.state.Name = c.Name

	return c.Cluster.SaveStackState(c.state)
}

func (k *Stack) Initialize() error {
	err := k.ConfigureNamespaces()
	if err != nil {
		return errors.Wrap(err, "could not configure namespaces")
	}

	err = k.ConfigurePullSecrets()
	if err != nil {
		return errors.Wrap(err, "could not configure pull secrets")
	}

	err = k.ConfigureCerts()
	if err != nil {
		return errors.Wrap(err, "could not configure certs")
	}

	err = k.Save()
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

		_, renewErr := vaultClient.Auth().Token().RenewSelf(int((768 * time.Hour).Seconds()))
		if renewErr != nil {
			log.Warnf("Couldn't renew token: %s", renewErr)
		}

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
			// log.Infof("Deploying cert to namespace %q", namespaceName)

			kubectl := k.Cluster.Kubectl

			log.Infof("Uploading cert to namespace %s", namespaceName)

			_, _ = kubectl.Exec("delete", "secret", certConfig.SecretName, "-n", namespaceName)

			_, err = kubectl.Exec("create", "secret", "tls", certConfig.SecretName, "--cert", certPath, "--key", keyPath, "-n", namespaceName)
			if err != nil {
				log.WithError(err).Warn("Could not create secret")
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
					var dataString string
					dataString, dockerFileErr = command.NewShellExe("bash", "-c", "sudo cat " + dockerConfigPath).RunOut()
					if dockerFileErr != nil {
						return errors.Errorf("error reading docker kubeconfig from %q: %s", dockerConfigPath, dockerFileErr)
					}
					data = []byte(dataString)
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
	if len(s.StackTemplate.Apps) == 0 {
		// if the stack doesn't have an explicit list of apps then all environment apps are enabled
		return false
	}
	app, ok := s.StackTemplate.Apps[name]

	return !(ok && !app.Disabled)
}

func (c *Stack) Destroy() error {

	errs := multierr.New()

	log := c.Cluster.ctx.Log()

	client, err := dynamic.NewForConfig(c.Cluster.kubeconfig)
	if err != nil {
		return err
	}

	log.Info("Deleting tenants first to avoid operator issues...")

	tenantRes := schema.GroupVersionResource{Group: "naveego.com", Version: "v1", Resource: "tenants"}

	tenantNamespace := c.StackTemplate.Namespaces["tenants"].Name
	err = client.Resource(tenantRes).Namespace(tenantNamespace).DeleteCollection(&metav1.DeleteOptions{}, metav1.ListOptions{})

	if err != nil {
		errs.Collect(err)
	} else {
		log.Info("Deleted tenants")
	}

	deleted := map[string]bool{}

	for _, ns := range c.StackTemplate.Namespaces {

		if ns.Shared {
			log.Infof("Skipping deletion of namespace %q because it is marked as shared.", ns.Name)
			continue
		}

		sharedWith := ""
		otherTemplates := append(c.Cluster.StackTemplates, &c.Cluster.StackTemplate)
		for _, otherTemplate := range otherTemplates {
			if otherTemplate.NamePattern == c.NamePattern {
				continue
			}
			for _, otherNs := range otherTemplate.Namespaces {
				if otherNs.Name == ns.Name {
					sharedWith = otherTemplate.Name
					break
				}
			}
		}

		if sharedWith != "" {
			log.Infof("Skipping deletion of namespace %q because it is also used by stack template %q.", ns.Name, sharedWith)
			continue
		}

		if deleted[ns.Name] {
			continue
		}

		log.Warnf("Deleting namespace %q...", ns.Name)

		err = c.Cluster.Client.CoreV1().Namespaces().Delete(ns.Name, &metav1.DeleteOptions{})

		if err != nil && !kerrors.IsNotFound(err) {
			errs.Collect(errors.Wrapf(err, "deleting namespace %q", ns.Name))
		}

		deleted[ns.Name] = true
	}

	log.Info("Deleting stack history...")
	err = c.Cluster.Client.CoreV1().ConfigMaps(c.Cluster.GetDefaultNamespace()).Delete(makeStackConfigmapName(c.Name), &metav1.DeleteOptions{})

	if err != nil {
		errs.Collect(err)
	}

	err = errs.ToError()
	if err != nil {
		return err
	}

	log.Info("Done deleting stack.")

	return nil
}

func (c *Stack) SetStoryID(id string) {
	c.state.StoryID = id
}

func (c *Stack) IsInitialized() bool {
	return c.state != nil && c.state.Uninitialized != true
}

func (c *Stack) GetStoryID() string {
	return c.state.StoryID
}

func (s StackState) Headers() []string {
	return []string{
		"Name",
		"Version",
		"Release",
		"Repo",
		"Branch",
		"Provider",
		"Commit",
		"DeployedAt",
		"DevopsBranch",
		"StoryKey",
		"Details",
	}
}

func (s StackState) Rows() [][]string {
	var out [][]string
	for _, appName := range util.SortedKeys(s.DeployedApps) {

		app := s.DeployedApps[appName]

		var deployedAt string
		if !app.DeployedAt.IsZero() {
			deployedAt = app.DeployedAt.Format(time.RFC3339)
		}

		out = append(out, []string{
			app.Name,
			app.Version,
			app.Release,
			app.Repo,
			app.Branch,
			app.Provider,
			app.Commit,
			deployedAt,
			app.DevopsBranch,
			app.StoryKey,
			app.Details,
		})
	}
	return out
}

type StackApp struct {
	Name         string    `yaml:"name"`
	Version      string    `yaml:"version"`
	Release      string    `yaml:"release"`
	Provider     string    `yaml:"provider"`
	Repo         string    `yaml:"repo"`
	Branch       string    `yaml:"branch"`
	Commit       string    `yaml:"commit"`
	DeployedAt   time.Time `yaml:"deployedAt"`
	DevopsBranch string    `yaml:"devopsBranch"`
	StoryKey     string    `yaml:"storyKey"`
	Details      string    `yaml:"details"`
}

const (
	DefaultStackName = "default"
)

func makeStackConfigmapName(name string) string {
	return "bosun-stack-" + git.Slug(name)
}
