package vcs

import (
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/util"
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

	_, err = g.Exec("commit", "-m", message, "--no-verify")

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

	if r.HasUpstream() {
		_, err = g.Exec("push")
	} else {
		_, err = g.Exec("push", "-u", "origin", string(r.GetCurrentBranch()))
	}
	return err
}

func (r *LocalRepo) CheckOut(name git.BranchName) error {
	_, err := r.git().Exec("checkout", string(name))
	return err
}

// SwitchToNewBranch pulls the current branch, then creates a new branch based on it and checks it out.
func (r *LocalRepo) SwitchToNewBranch(ctx util.Logger, parent, child string) error {
	ctx.Log().Infof("Creating branch %s...", child)
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
	_, err = g.Exec("push", "-u", "origin", child)
	if err != nil {
		return err
	}

	return nil
}

func (r *LocalRepo) HasUpstream() bool {
	_, err := r.git().Exec("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	return err == nil
}

func (r *LocalRepo) SwitchToBranchAndPull(logger util.Logger, name string) error {
	logger.Log().WithField("repo", r.Name).Infof("Checking out branch %q.", name)
	g := r.git()
	_, err := g.Exec("checkout", name)
	if err != nil {
		return err
	}

	// check if upstream exists
	if r.HasUpstream() {
		logger.Log().WithField("repo", r.Name).Infof("Pulling branch %q.", name)
		_, err = g.Exec("pull")
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *LocalRepo) GetCurrentCommit() string {
	r.mustBeCloned()
	return r.git().GetCurrentCommit()
}

func (r *LocalRepo) git() git.GitWrapper {
	g, err := git.NewGitWrapper(r.Path)
	if err != nil {
		panic(err)
	}
	return g
}

func (r *LocalRepo) GetCurrentBranch() git.BranchName {
	return git.BranchName(r.git().Branch())
}

func (r *LocalRepo) DoesBranchExist(ctx util.Logger, name string) (bool, error) {
	if r == nil {
		return false, errors.New("not cloned")
	}

	g, err := git.NewGitWrapper(r.Path)
	if err != nil {
		return false, err
	}

	_, err = g.Exec("fetch", "-p")
	if err != nil {
		return false, err
	}

	branches, err := g.Exec("branch", "-a")
	if err != nil {
		return false, err
	}

	return strings.Contains(branches, name), nil
}

func (r *LocalRepo) GetMostRecentTagRef(pattern string) (string, error) {
	lines, err := r.git().ExecLines("tag", "-l", "--sort=-authordate", pattern)
	if err != nil {
		return "", err
	}
	if len(lines) == 0 {
		return "", nil
	}
	return lines[0], nil
}

func (r *LocalRepo) GetUpstreamStatus() string {
	g := r.git()
	_ = g.Fetch()
	upstream := "@{u}"
	local, _ := g.Exec("rev-parse", "@")
	remote, _ := g.Exec("rev-parse", upstream)
	base, _ := g.Exec("merge-base", "@", upstream)
	if local == remote {
		return "up-to-date"
	}
	if local == base {
		return "behind"
	}
	if remote == "base" {
		return "ahead"
	}
	return "diverged"
}
