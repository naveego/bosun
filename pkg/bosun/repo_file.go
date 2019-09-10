package bosun

import (
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/git"
)

type RepoFile struct {
	File       `yaml:",inline"`
	Name       string            `yaml:"name"`
	APIVersion string            `yaml:"apiVersion,omitempty"`
	Branching  git.BranchSpec    `yaml:"branching,omitempty"`
	AppFolders map[string]string `yaml:"appFolders"`
}

func (f *RepoFile) MarshalYAML() (interface{}, error) {
	if f == nil {
		return nil, nil
	}
	type proxy RepoFile
	p := proxy(*f)

	return &p, nil
}

func (f *RepoFile) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type proxy RepoFile
	var p proxy
	if f != nil {
		p = proxy(*f)
	}

	err := unmarshal(&p)

	if err == nil {
		*f = RepoFile(p)
	}

	if f.Branching.Master == "" {
		f.Branching.Master = "master"
	}
	if f.Branching.Develop == "" {
		// default behavior is trunk based development
		f.Branching.Develop = "master"
	}
	if f.Branching.Release == "" {
		// migrate BranchForRelease to p.Branching.Release pattern.
		f.Branching.Release = "release/{{.Version}}"
	}
	if f.Branching.Feature == "" {
		f.Branching.Feature = "issue/{{.Number}}/{{.Slug}}"
	}

	return err
}

type AppRequest struct {
	Filter     filter.Filter
	Name       string
	Branch     string
	BranchType git.BranchType
}

type AppStore interface {
	GetApp(req AppRequest) (*App, error)
	GetApps(req AppRequest) ([]*App, error)
	SaveApp(app *App) error
}
