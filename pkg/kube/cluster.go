package kube

import (
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/kube/kubeclient"
	"github.com/naveego/bosun/pkg/tmpcache"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"os"
	"regexp"
	"time"
)

type Cluster struct {
	ctx command.ExecutionContext

	ClusterConfig
	Kubectl    Kubectl
	Client     *kubernetes.Clientset
	kubeconfig *rest.Config
}

var _ values.ValueSetCollectionProvider = &Cluster{}

func NewCluster(config ClusterConfig, ctx command.ExecutionContext, allowIncomplete bool) (*Cluster, error) {

	kubectl := Kubectl{
		Cluster:    config.Name,
		Kubeconfig: config.GetKubeconfigPath(),
	}

	if kubectl.Kubeconfig == "" {
		kubectl.Kubeconfig = os.ExpandEnv("$HOME/.kube/config")
	}

	c := &Cluster{
		ctx:           ctx,
		ClusterConfig: config,
		Kubectl:       Kubectl{},
	}

	var err error
	c.kubeconfig, err = kubeclient.GetKubeConfigWithContext(config.KubeconfigPath, config.Name)
	if err != nil && !allowIncomplete {
		return nil, errors.Wrapf(err, "could not create kubernetes client for cluster %q, you may need to run `bosun cluster configure`", config.Name)
	}

	if c.kubeconfig != nil {

		c.Client, err = kubernetes.NewForConfig(c.kubeconfig)
		if err != nil && !allowIncomplete {
			return nil, err
		}
	}

	return c, nil

}

func (c *Cluster) ConfigureKubectl() error {

	kubectl := c.Kubectl
	config := c.ClusterConfig
	ctx := c.ctx

	if kubectl.contextIsDefined(config.Name) && !ctx.GetParameters().Force {
		ctx.Log().Debugf("Kubernetes context %q already exists (use --force to configure anyway).", config.Name)
	} else {

		k := config
		req := ConfigureRequest{
			KubeConfigPath:   config.KubeconfigPath,
			Force:            ctx.GetParameters().Force,
			Log:              ctx.Log(),
			ExecutionContext: ctx,
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
	}

	return nil
}

func (c *Cluster) Activate() error {
	_, err := c.Kubectl.Exec("config", "use-context", c.Name)

	return err
}

func (c *Cluster) GetDefaultNamespace() string {
	namespace := c.DefaultNamespace
	if namespace == "" {
		for k, n := range c.Namespaces {
			if k == core.NamespaceRoleDefault {
				namespace = n.Name
			}
		}

	}
	if namespace == "" {
		namespace = "default"
	}
	return namespace
}

func (c *Cluster) GetStackStateFast(name string) (*StackState, error) {

	var stackState StackState
	if tmpcache.Get(makeStackConfigmapName(name), &stackState) {
		return &stackState, nil
	}

	return c.GetStackState(name)
}

func (c *Cluster) GetStackState(name string) (*StackState, error) {
	namespace := c.DefaultNamespace
	if namespace == "" {
		namespace = "default"
	}

	configmapName := makeStackConfigmapName(name)

	configmap, err := c.Client.CoreV1().ConfigMaps(namespace).Get(configmapName, metav1.GetOptions{})

	var state *StackState
	if kerrors.IsNotFound(err) {
		if name == DefaultStackName {
			c.ctx.Log().Info("Initializing default stack in cluster...")
			var stack *Stack
			stack, err = NewStack(c, name, "", c.StackTemplate)
			if err != nil {
				return nil, err
			}
			stack.state = &StackState{
				Name: name,
			}
			err = stack.Save()
			if err != nil {
				return nil, errors.WithStack(err)
			}
			state, _ = stack.GetState(false)
		} else {
			return nil, errors.Errorf("Did not find stack %q in cluster %q by looking for configmap %q in namespace %q", name, c.Name, configmapName, namespace)
		}
	} else if err != nil {
		return nil, errors.Wrap(err, "checking for existence of stack")
	}

	if state == nil {
		var unmarshalledState StackState

		err = yaml.Unmarshal([]byte(configmap.Data[stackConfigMapKey]), &unmarshalledState)
		if err != nil {
			return nil, err
		}
		state = &unmarshalledState
	}

	tmpcache.Set(makeStackConfigmapName(name), state)

	return state, nil
}

func (c *Cluster) SaveStackState(stackState *StackState) error {
	var err error
	configmapName := makeStackConfigmapName(stackState.Name)

	tmpcache.Set(configmapName, stackState)

	client := c.Client
	namespace := c.GetDefaultNamespace()
	var configmap *v1.ConfigMap
	for {

		configmap, err = client.CoreV1().ConfigMaps(namespace).Get(configmapName, metav1.GetOptions{})
		if kerrors.IsNotFound(err) {
			defaultData, _ := yaml.MarshalString(stackState)

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

			return errors.WithStack(err)
		} else if err != nil {
			return errors.WithStack(err)
		}

		configmap.Data[stackConfigMapKey], _ = yaml.MarshalString(stackState)

		_, err = client.CoreV1().ConfigMaps(namespace).Update(configmap)

		if kerrors.IsConflict(err) {
			core.Log.Warn("Conflict while updating deployed apps, will try again.")
			<-time.After(1 * time.Second)
		} else if err != nil {
			return errors.WithStack(err)
		} else {
			break
		}
	}

	return nil
}

type StackStateMap map[string]*StackState

func (s StackStateMap) Headers() []string {
	return []string{"Name", "Template", "Story"}
}

func (s StackStateMap) Rows() [][]string {
	var out [][]string
	for _, n := range util.SortedKeys(s) {
		stack := s[n]
		out = append(out, []string{n, stack.TemplateName, stack.StoryID})
	}
	return out
}

func (c *Cluster) GetStackStates() (StackStateMap, error) {
	namespace := c.DefaultNamespace

	configmaps, err := c.Client.CoreV1().ConfigMaps(namespace).List(metav1.ListOptions{
		LabelSelector: StackLabel,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	out := map[string]*StackState{}

	for _, configmap := range configmaps.Items {

		data := configmap.Data[stackConfigMapKey]

		var stackConfig *StackState

		err = yaml.UnmarshalString(data, &stackConfig)

		if err != nil {
			return nil, errors.WithStack(err)
		}

		if stackConfig == nil {
			c.ctx.Log().Warnf("Invalid stack configmap found with name %q", configmap.Name)
			continue
		}

		out[stackConfig.Name] = stackConfig
	}

	return out, nil
}

func (c *Cluster) CreateStack(name string, templateName string) (*Stack, error) {

	namespace := c.GetDefaultNamespace()
	configmapName := makeStackConfigmapName(name)

	_, err := c.Client.CoreV1().ConfigMaps(namespace).Get(configmapName, metav1.GetOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return nil, errors.WithStack(err)
	}
	if !kerrors.IsNotFound(err) {
		return nil, errors.Errorf("stack with name %q already exists", name)
	}

	template, err := c.GetStackTemplate(templateName)
	if err != nil {
		return nil, err
	}

	stack, err := NewStack(c, name, templateName, *template)

	if err != nil {
		return nil, err
	}

	err = stack.Save()
	return stack, err
}

func (c *Cluster) GetStack(name string) (*Stack, error) {
	stackState, err := c.GetStackStateFast(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return NewStackFromState(c, stackState)
}

func (c *Cluster) GetStackTemplateForStackName(name string) (*StackTemplate, error) {

	if name == "" || name == DefaultStackName {
		return &c.StackTemplate, nil
	}

	patterns := map[string]string{}

	var stackTemplate *StackTemplate

	for _, candidate := range c.StackTemplates {

		if candidate.Name == name {
			stackTemplate = candidate
			break
		}

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
			stackTemplate = candidate
		default:
			stackTemplate = candidate
		}
		if stackTemplate != nil {
			break
		}
	}

	if stackTemplate == nil {
		return nil, errors.Errorf("no stack template name pattern matched requested name %q; you are currently using cluster %q in environment %q; available name patterns: %+v", name, c.Name, c.Environment, patterns)
	}

	return stackTemplate, nil
}

func (c *Cluster) GetStackTemplate(templateName string) (*StackTemplate, error) {

	if templateName == "" || templateName == DefaultStackName {
		return &c.StackTemplate, nil
	}

	for _, candidate := range c.StackTemplates {

		if candidate.Name == templateName {
			return candidate, nil
		}

	}

	return nil, errors.Errorf("no stack template found with name %q; available templates: %+v", templateName, util.SortedKeys(c.StackTemplates))
}

func (c *Cluster) GetStackTemplates() []*StackTemplate {
	out := []*StackTemplate{&c.StackTemplate}
	for _, t := range c.StackTemplates {
		out = append(out, t)
	}
	return out
}
