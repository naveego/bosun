package kube

import (
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

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

func (c *Cluster) GetStackConfigs() (map[string]*StackState, error) {
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

		out[stackConfig.Name] = stackConfig
	}

	return out, nil
}

func (c *Cluster) CreateStack(name string) (*Stack, error) {

	namespace := c.DefaultNamespace
	configmapName := makeStackConfigmapName(name)

	_, err := c.Client.CoreV1().ConfigMaps(namespace).Get(configmapName, metav1.GetOptions{})
	if !kerrors.IsNotFound(err) {
		return nil, errors.Errorf("stack with name %q already exists", name)
	}

	return NewStack(c, name)
}

func (c *Cluster) GetStack(name string) (*Stack, error) {

	return NewStack(c, name)
}
