package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/pkg/errors"
	"path/filepath"
)

type RepoConfig struct {
	ConfigShared    `yaml:",inline"`
	Branching       git.BranchSpec
	FilteringLabels map[string]string `yaml:"labels" json:"labels"`
}

type Repo struct {
	RepoConfig
	LocalRepo *LocalRepo
	Apps      map[string]*AppConfig
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

func (r *Repo) CheckCloned() error {
	if r == nil {
		return errors.New("repo is unknown")
	}
	if r.LocalRepo == nil {
		return errors.Errorf("repo %q is not cloned", r.Name)
	}
	return nil
}

func (r *Repo) Clone(ctx BosunContext, toDir string) error {
	if err := r.CheckCloned(); err != nil {
		return err
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

func (r Repo) GetLocalBranchName() git.BranchName {
	if r.LocalRepo == nil {
		return ""
	}

	if r.LocalRepo.branch == "" {
		g, _ := git.NewGitWrapper(r.LocalRepo.Path)
		r.LocalRepo.branch = git.BranchName(g.Branch())
	}
	return r.LocalRepo.branch
}

func (r *Repo) Pull(ctx BosunContext, rebase bool) error {
	if err := r.CheckCloned(); err != nil {
		return err
	}

	g, _ := git.NewGitWrapper(r.LocalRepo.Path)
	if rebase {
		return g.PullRebase()
	} else {
		return g.Pull()
	}
}

func (r *Repo) Fetch() error {
	if err := r.CheckCloned(); err != nil {
		return err
	}

	g, _ := git.NewGitWrapper(r.LocalRepo.Path)
	err := g.Fetch()

	return err
}

func (r *Repo) Merge(fromBranch, toBranch string) error {
	if err := r.CheckCloned(); err != nil {
		return err
	}

	g, _ := git.NewGitWrapper(r.LocalRepo.Path)

	err := g.Fetch()
	if err != nil {
		return err
	}

	_, err = g.Exec("checkout", fromBranch)
	if err != nil {
		return err
	}

	err = g.Pull()

	return err
}

func (r *Repo) GetRef() (issues.RepoRef, error) {
	if r == nil {
		return issues.RepoRef{}, errors.New("repo not set")
	}
	return issues.ParseRepoRef(r.Name)
}
