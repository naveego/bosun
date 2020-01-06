package kube

import (
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/core"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

type CommandContext struct {
	KubeConfigPath string
	Force          bool
	Name           string
	Log            *logrus.Entry
}

type ConfigDefinitions []*ConfigDefinition

func (k ConfigDefinitions) GetKubeConfigDefinitionByName(name string) (*ConfigDefinition, error) {
	for _, c := range k {
		if c.Name == name {
			return c, nil
		}
	}
	return nil, errors.Errorf("no definition with name %q", name)
}

type ConfigureKubeContextRequest struct {
	Name           string
	Environment    string
	Role           core.ClusterRole
	KubeConfigPath string
	Force          bool
	Log            *logrus.Entry
}

func (k ConfigDefinitions) HandleConfigureKubeContextRequest(req ConfigureKubeContextRequest) error {
	if req.Log == nil {
		req.Log = logrus.NewEntry(logrus.StandardLogger())
	}

	var konfigs ConfigDefinitions

	if req.Name != "" {
		konfig, err := k.GetKubeConfigDefinitionByName(req.Name)
		if err != nil {
			return err
		}
		konfigs = append(konfigs, konfig)
	} else {
		found, err := k.GetKubeConfigDefinitionsByAttributes(req.Environment, req.Role)
		if err != nil {
			return err
		}
		konfigs = append(konfigs, found...)
	}

	if len(konfigs) == 0 {
		return errors.Errorf("could not find any kube configs")
	}

	for _, c := range konfigs {
		ktx := CommandContext{
			Name:           c.Name,
			Force:          req.Force,
			KubeConfigPath: req.KubeConfigPath,
			Log:            req.Log,
		}

		err := c.configureKubernetes(ktx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (k ConfigDefinitions) GetKubeConfigDefinitionsByAttributes(env string, role core.ClusterRole) ([]*ConfigDefinition, error) {
	var out []*ConfigDefinition
	for _, c := range k {
		matched := env == ""
		if !matched {
			for _, e := range c.Environments {
				if e == env {
					matched = true
					break
				}
			}
		}
		if !matched {
			continue
		}
		matched = role == ""
		if !matched {
			for _, r := range c.Roles {
				if r == role {
					matched = true
					break
				}
			}
		}
		if !matched {
			continue
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, errors.Errorf("no definition had environment %q and role %q", env, role)
	}
	return out, nil
}

type ConfigDefinition struct {
	Name         string                                 `yaml:"name"`
	Roles        []core.ClusterRole                     `yaml:"roles,flow"`
	Environments []string                               `yaml:"environments,flow"`
	Oracle       *OracleClusterConfig                   `yaml:"oracle,omitempty"`
	Minikube     *MinikubeConfig                        `yaml:"minikube,omitempty"`
	Amazon       *AmazonClusterConfig                   `yaml:"amazon,omitempty"`
	Namespaces   map[core.NamespaceRole]NamespaceConfig `yaml:"namespaces"`
}

type NamespaceConfig struct {
	Name string `yaml:"name"`
}

func (k ConfigDefinition) configureKubernetes(ctx CommandContext) error {
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
