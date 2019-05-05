package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/filter"
	"path/filepath"
)

type RepoConfig struct {
	Name            string            `yaml:"name" json:"name"`
	FilteringLabels map[string]string `yaml:"labels" json:"labels"`
}

type Repo struct {
	RepoConfig
	LocalRepo *LocalRepo
	Apps      map[string]*AppRepoConfig
}

func (r Repo) GetLabels() filter.Labels {
	out := filter.Labels{
		"name": filter.LabelString(r.Name),
	}
	if r.LocalRepo != nil {
		out["path"] = filter.LabelString(r.LocalRepo.Path)
	}
	for k, v := range r.RepoConfig.FilteringLabels {
		out[k] = filter.LabelString(v)
	}
	return out
}

func (r Repo) IsRepoCloned() bool {
	return r.LocalRepo != nil
}

func (r *Repo) CloneRepo(ctx BosunContext, toDir string) error {
	if r.IsRepoCloned() {
		return nil
	}

	dir, _ := filepath.Abs(filepath.Join(toDir, r.Name))

	err := pkg.NewCommand("git", "clone",
		"--depth", "1",
		"--no-single-branch",
		fmt.Sprintf("git@github.com:%s.git", r.Name),
		dir).
		RunE()

	if err != nil {
		return err
	}

	r.LocalRepo = &LocalRepo{
		Name: r.Name,
		Path: dir,
	}

	ctx.Bosun.AddLocalRepo(r.LocalRepo)

	return nil
}
