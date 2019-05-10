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

func (r *LocalRepo) Branch(ctx BosunContext, parentBranch string, name string) error {
	if r == nil {
		return errors.New("not cloned")
	}

	g, err := git.NewGitWrapper(r.Path)
	if err != nil {
		return err
	}

	_, err = g.Exec("fetch")
	if err != nil {
		return err
	}

	branches, err := g.Exec("branch", "-a")
	if err != nil {
		return err
	}

	if strings.Contains(branches, name) {
		ctx.Log.Info("Checking out release branch...")
		_, err = g.Exec("checkout", name)
		if err != nil {
			return err
		}
		_, err = g.Exec("pull")
		if err != nil {
			return err
		}
	} else {
		ctx.Log.Infof("Creating branch %s...", name)

		_, err = g.Exec("checkout", parentBranch)
		if err != nil {
			return errors.Wrapf(err, "check out parent branch %q", parentBranch)
		}

		_, err = g.Exec("pull")
		if err != nil {
			return errors.Wrapf(err, "pulling parent branch %q", parentBranch)
		}

		_, err = g.Exec("branch", name)
		if err != nil {
			return err
		}
		_, err = g.Exec("checkout", name)
		if err != nil {
			return err
		}

		_, err = g.Exec("push", "-u", "origin", name)
		if err != nil {
			return err
		}
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
