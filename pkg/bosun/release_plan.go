package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
)

type ReleasePlan struct {
	FromPath        string              `yaml:"-"`
	Apps            map[string]*AppPlan `yaml:"apps"`
	ReleaseMetadata *ReleaseMetadata    `yaml:"releaseManifest"`
}

func (ReleasePlan) Headers() []string {
	return []string{"Name", "Provider", "Deploy"}
}

func (r ReleasePlan) Rows() [][]string {
	var out [][]string
	for _, name := range util.SortedKeys(r.Apps) {
		appPlan := r.Apps[name]

		out = append(out, []string{
			appPlan.Name,
			appPlan.String(),
			fmt.Sprint(appPlan.Deploy),
		})
	}
	return out
}

func (r ReleasePlan) GetAppPlan(name string) (*AppPlan, error) {
	if a, ok := r.Apps[name]; ok {
		return a, nil
	}
	return nil, errors.Errorf("no plan for app %q", name)
}

func NewReleasePlan(releaseMetadata *ReleaseMetadata) *ReleasePlan {
	return &ReleasePlan{
		ReleaseMetadata: releaseMetadata,
		Apps:            map[string]*AppPlan{},
	}
}

type AppPlan struct {
	Name           string                     `yaml:"name"`
	Deploy         bool                       `yaml:"deploy"`
	ChosenProvider string                     `yaml:"chosenProvider"`
	BumpOverride   string                     `yaml:"bumpOverride,omitempty"`
	Providers      map[string]AppProviderPlan `yaml:"providers"`
}

type AppProviderPlan struct {
	Version   string   `yaml:"version"`
	Bump      string   `yaml:"bump,omitempty"`
	Changelog []string `yaml:"changelog,omitempty"`
}

func (a *AppPlan) IsProviderChosen() bool {
	return a.ChosenProvider != ""
}

func (a AppPlan) String() string {
	if a.ChosenProvider != "" {
		providerPlan, ok := a.Providers[a.ChosenProvider]
		if !ok {
			return fmt.Sprintf("Invalid provider %q", a.ChosenProvider)
		}
		return fmt.Sprintf("%s: %s", a.ChosenProvider, providerPlan.Version)
	}

	return "no chosen provider"
}
