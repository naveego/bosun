package kube

import (
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

type Stack struct {
	Apps map[string]StackApp `yaml:"apps"`
}

func (s Stack) Headers() []string {
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

func (s Stack) Rows() [][]string {
	var out [][]string
	for _, appName := range util.SortedKeys(s.Apps) {

		app := s.Apps[appName]

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
	deployedAppsConfigMapName = "bosun-deployed-apps"
	deployedAppsConfigMapKey  = "data"
)

func (c *ClusterConfig) GetStackState() (*Stack, error) {

	client, err := GetKubeClientWithContext(c.KubeconfigPath, c.Name)
	if err != nil {
		return nil, err
	}

	namespace := c.Namespaces["default"].Name

	configmap, err := client.CoreV1().ConfigMaps(namespace).Get(deployedAppsConfigMapName, metav1.GetOptions{})
	if kerrors.IsNotFound(err) {
		return &Stack{Apps: map[string]StackApp{}}, nil
	}
	if err != nil {
		return nil, err
	}

	data := configmap.Data[deployedAppsConfigMapKey]

	var deployedApps *Stack

	err = yaml.UnmarshalString(data, &deployedApps)

	return deployedApps, err
}

func (c *ClusterConfig) UpdateStackApp(updates ...StackApp) error {

	err := c.UpdateStack(func(stack *Stack) error {
		for _, app := range updates {

			stack.Apps[app.Name] = app
		}

		return nil
	})

	return err
}

func (c *ClusterConfig) UpdateStack(mutator func(apps *Stack) error) error {

	client, err := GetKubeClientWithContext(c.KubeconfigPath, c.Name)
	if err != nil {
		return err
	}

	namespace := c.Namespaces["default"].Name

	var configmap *v1.ConfigMap

	for {

		configmap, err = client.CoreV1().ConfigMaps(namespace).Get(deployedAppsConfigMapName, metav1.GetOptions{})
		if kerrors.IsNotFound(err) {
			emptyData, _ := yaml.MarshalString(Stack{})

			configmap, err = client.CoreV1().ConfigMaps(namespace).Create(&v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: deployedAppsConfigMapName},
				Data: map[string]string{
					deployedAppsConfigMapKey: emptyData,
				},
				BinaryData: nil,
			})

			return err
		} else if err != nil {
			return err
		}

		var stack *Stack
		_ = yaml.UnmarshalString(configmap.Data[deployedAppsConfigMapKey], &stack)

		err = mutator(stack)
		if err != nil {
			return errors.Wrap(err, "error applying mutator")
		}

		configmap.Data[deployedAppsConfigMapKey], _ = yaml.MarshalString(stack)

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
