package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
	"strings"
)

type ReleasePlan struct {
	Apps            map[string]*AppPlan `yaml:"apps"`
	ReleaseMetadata *ReleaseMetadata    `yaml:"releaseManifest"`
}

func (ReleasePlan) Headers() []string {
	return []string{"Name", "Previous Release", "From Branch", "To Branch", "Bump", "Deploy"}
}

func (r ReleasePlan) Rows() [][]string {
	var out [][]string
	for _, name := range util.SortedKeys(r.Apps) {
		appPlan := r.Apps[name]

		previousVersion := appPlan.PreviousReleaseName

		out = append(out, []string{
			appPlan.Name,
			previousVersion,
			appPlan.PreviousReleaseName,
			appPlan.FromBranch,
			appPlan.ToBranch,
			appPlan.Bump,
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
	Name                        string   `yaml:"name"`
	Repo                        string   `yaml:"repo"`
	Deploy                      bool     `yaml:"deploy"`
	ToBranch                    string   `yaml:"toBranch"`
	FromBranch                  string   `yaml:"fromBranch"`
	Bump                        string   `yaml:"bump"`
	Reason                      string   `yaml:"reason,omitempty"`
	PreviousReleaseName         string   `yaml:"previousRelease"`
	PreviousReleaseVersion      string   `yaml:"previousReleaseVersion"`
	CurrentVersionInMaster      string   `yaml:"currentVersionInMaster"`
	CommitsNotInPreviousRelease []string `yaml:"commitsNotInPreviousRelease,omitempty"`
}

func (a *AppPlan) IsBumpUnset() bool {
	return a.Bump == "" || strings.HasPrefix(strings.ToLower(a.Bump), "no")
}

func (a AppPlan) String() string {

	w := new(strings.Builder)
	_, _ = fmt.Fprintf(w, "%s: ", a.Name)
	if a.PreviousReleaseName == "" {
		_, _ = fmt.Fprintf(w, "never released;")
	} else {
		_, _ = fmt.Fprintf(w, "previously released from %s;", a.PreviousReleaseName)
	}

	if a.FromBranch != "" {
		if a.ToBranch != "" {
			_, _ = fmt.Fprintf(w, "branching: %s -> %s;", a.FromBranch, a.ToBranch)
		} else {
			_, _ = fmt.Fprintf(w, "using branch: %s;", a.FromBranch)
		}

	}
	if a.Bump != "" {
		_, _ = fmt.Fprintf(w, "bump: %s;", a.Bump)
	}
	if a.Deploy {
		_, _ = fmt.Fprint(w, " (will be deployed by default) ")
	} else {
		_, _ = fmt.Fprint(w, " (will NOT be deployed by default) ")
	}
	return w.String()
}
