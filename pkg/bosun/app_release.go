package bosun

import (
	"strings"
	"time"
)

type AppReleasesSortedByName []*AppRelease

func (a AppReleasesSortedByName) Len() int {
	return len(a)
}

func (a AppReleasesSortedByName) Less(i, j int) bool {
	return strings.Compare(a[i].Name, a[j].Name) < 0
}

func (a AppReleasesSortedByName) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

type AppReleaseConfig struct {
	Name      string       `yaml:"name"`
	Repo      string       `yaml:"repo"`
	RepoPath  string       `yaml:"repoPath"`
	BosunPath string       `yaml:"bosunPath"`
	Branch    string       `yaml:"branch"`
	Commit    string       `yaml:"commit"`
	Version   string       `yaml:"version"`
	SyncedAt  time.Time    `yaml:"syncedAt"`
	Chart     string       `yaml:"chart"`
	Image     string       `yaml:"image"`
	Actions   []*AppAction `yaml:"actions"`
	// Additional values to be merged in, specific to this release.
	Values       AppValuesByEnvironment `yaml:"values"`
	ParentConfig *ReleaseConfig         `yaml:"-"`
}

func (r *AppReleaseConfig) SetParent(config *ReleaseConfig) {
	r.ParentConfig = config
}

type AppRelease struct {
	*AppReleaseConfig
	AppRepo  *AppRepo `yaml:"-"`
	Excluded bool     `yaml:"-"`
}

func NewAppRelease(ctx BosunContext, config *AppReleaseConfig) (*AppRelease, error) {
	release := &AppRelease{
		AppReleaseConfig: config,
		AppRepo:          ctx.Bosun.GetApps()[config.Name],
	}

	return release, nil
}
