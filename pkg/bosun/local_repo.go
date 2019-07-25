package bosun

import (
	"github.com/naveego/bosun/pkg/git"
	"github.com/pkg/errors"
	"strings"
)

type LocalRepo struct {
	Name   string         `yaml:"-" json:""`
	Path   string         `yaml:"path,omitempty" json:"path,omitempty"`
	branch git.BranchName `yaml:"-" json:"-"`
}

func (r *LocalRepo) mustBeCloned() {
	if r == nil {
		panic("not cloned; you should have checked Repo.CheckCloned() before interacting with the local repo")
	}
}

func (r *LocalRepo) Git() (git.GitWrapper, error) {
	g, err := git.NewGitWrapper(r.Path)
	return g, err
}

func (r *LocalRepo) IsDirty() bool {
	r.mustBeCloned()
	g, _ := git.NewGitWrapper(r.Path)
	return g.IsDirty()
}

func (r *LocalRepo) Commit(message string, filesToAdd ...string) error {
	r.mustBeCloned()

	g, err := git.NewGitWrapper(r.Path)
	if err != nil {
		return err
	}

	addArgs := append([]string{"add"}, filesToAdd...)
	_, err = g.Exec(addArgs...)
	if err != nil {
		return err
	}

	_, err = g.Exec("commit", "-m", message)

	if err != nil {
		return err
	}

	return nil
}

func (r *LocalRepo) Push() error {
	r.mustBeCloned()

	g, err := git.NewGitWrapper(r.Path)
	if err != nil {
		return err
	}

	_, err = g.Exec("push")
	return err
}

// SwitchToNewBranch pulls the current branch, then creates a new branch based on it and checks it out.
func (r *LocalRepo) SwitchToNewBranch(ctx BosunContext, parent, child string) error {
	ctx.Log.Infof("Creating branch %s...", child)
	g := r.git()
	_, err := g.Exec("checkout", parent)
	if err != nil {
		return errors.Wrapf(err, "check out parent branch %q", parent)
	}

	_, err = g.Exec("pull")
	if err != nil {
		return errors.Wrapf(err, "pulling parent branch %q", parent)
	}

	_, err = g.Exec("branch", child)
	if err != nil {
		return err
	}
	_, err = g.Exec("checkout", child)
	if err != nil {
		return err
	}

	return nil
}

func (r *LocalRepo) SwitchToBranchAndPull(ctx BosunContext, name string) error {
	ctx.Log.Info("Checking out release branch...")
	g := r.git()
	_, err := g.Exec("checkout", name)
	if err != nil {
		return err
	}
	_, err = g.Exec("pull")
	if err != nil {
		return err
	}

	_, err = g.Exec("push", "-u", "origin", name)
	if err != nil {
		return err
	}

	return nil
}

func (r *LocalRepo) GetCurrentCommit() string {
	r.mustBeCloned()
	return r.git().Commit()
}

func (r *LocalRepo) git() git.GitWrapper {
	g, err := git.NewGitWrapper(r.Path)
	if err != nil {
		panic(err)
	}
	return g
}

func (r *LocalRepo) GetCurrentBranch() string {
	return r.git().Branch()
}

func (r *LocalRepo) DoesBranchExist(ctx BosunContext, name string) (bool, error) {
	if r == nil {
		return false, errors.New("not cloned")
	}

	g, err := git.NewGitWrapper(r.Path)
	if err != nil {
		return false, err
	}

	_, err = g.Exec("fetch")
	if err != nil {
		return false, err
	}

	branches, err := g.Exec("branch", "-a")
	if err != nil {
		return false, err
	}

	return strings.Contains(branches, name), nil
}
