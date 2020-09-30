package git

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"github.com/rs/xid"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"strings"
)

type Worktree struct {
	GitWrapper
	WorktreeConfig
	log  *logrus.Entry
	fake bool
}

func (w Worktree) ResolvePath(path string) string {
	if filepath.IsAbs(path) {
		return strings.Replace(path, w.OriginalDir, w.dir, 1)
	}

	return filepath.Join(w.dir, path)
}

func (w Worktree) Dispose() {
	if w.fake {
		return
	}

	_, _ = w.Exec("worktree", "remove", w.dir)
	_, _ = w.Exec("branch", "-D", w.WorktreeBranch)
}

func NewWorktree(g GitWrapper, branch BranchName) (Worktree, error) {
	var err error
	repoDirName := filepath.Base(g.dir)
	currentBranch := g.Branch()
	if currentBranch == branch.String() {
		return Worktree{
			GitWrapper: g,
			WorktreeConfig: WorktreeConfig{
				OriginalDir:    g.dir,
				OriginalBranch: branch.String(),
				WorktreeBranch: branch.String(),
				WorktreeDir:    g.dir,
			},
			fake: true,
			log: pkg.Log.WithField("repo", repoDirName).WithField("branch", branch).WithField("_", "worktree-fake"),
		}, nil
	}

	branchSlug := Slug(branch.String())

	worktreeDir := filepath.Join(getWorktreePath(), fmt.Sprintf("%s-worktree-%s", repoDirName, branchSlug))
	worktree := Worktree{
		GitWrapper: GitWrapper{
			dir: worktreeDir,
		},
		WorktreeConfig: WorktreeConfig{
		OriginalDir: g.dir,
		OriginalBranch: branch.String(),
		WorktreeDir: worktreeDir,
		WorktreeBranch:     fmt.Sprintf("worktree-%s-%s", branchSlug, xid.New()),
		},
		log:        pkg.Log.WithField("repo", repoDirName).WithField("branch", branch).WithField("_", "worktree"),
	}

	worktree.log.Infof("Creating worktree at %s", worktree.dir)

	err = os.MkdirAll(worktree.dir, 0700)
	if err != nil {
		return worktree, err
	}

	err = g.Fetch()
	if err != nil {
		return worktree, err
	}

	_, err = g.Exec("branch", "--track", worktree.WorktreeBranch, fmt.Sprintf("origin/%s", branch))

	if err != nil {
		return worktree, errors.Wrapf(err, "checking out worktree for %s", branch)
	}

	_, err = g.Exec("worktree", "add", worktree.dir, worktree.WorktreeBranch)
	if err != nil {
		return worktree, errors.Wrapf(err, "create work tree for checking out branch %q", branch)
	}

	return worktree, nil
}

func getWorktreePath() string {
	p := filepath.Join(os.TempDir(), "bosun-worktrees")
	_ = os.MkdirAll(p, 0700)
	return p
}

type WorktreeConfig struct {
	WorktreeDir    string
	WorktreeBranch string
	OriginalDir    string
	OriginalBranch string
}
