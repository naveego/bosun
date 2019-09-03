package git

import (
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"strings"
)

type GitWrapper struct {
	dir string
}

func NewGitWrapper(pathHint string) (GitWrapper, error) {
	dir, err := GetRepoPath(pathHint)
	if err != nil {
		return GitWrapper{}, err
	}
	return GitWrapper{
		dir: dir,
	}, nil
}

func (g GitWrapper) ExecLines(args ...string) ([]string, error) {
	text, err := g.Exec(args...)
	if err != nil {
		return nil, err
	}
	if len(text) == 0 {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

func (g GitWrapper) Exec(args ...string) (string, error) {
	args = append([]string{"-C", g.dir}, args...)

	out, err := pkg.NewCommand("git", args...).RunOut()
	if err != nil {
		return "", errors.Errorf("git %s\nOutput: %s\nError: %s", strings.Join(args, " "), out, err)
	}
	return out, err
}

func (g GitWrapper) Branch() string {
	o, _ := pkg.NewCommand("git", "-C", g.dir, "rev-parse", "--abbrev-ref", "HEAD").RunOut()
	return o
}

func (g GitWrapper) Commit() string {
	o, _ := pkg.NewCommand("git", "-C", g.dir, "log", "--pretty=format:'%h'", "-n", "1").RunOut()
	return strings.Trim(o, "'")
}

func (g GitWrapper) Tag() string {
	o, _ := pkg.NewCommand("git", "-C", g.dir, "describe", "--abbrev=0", "--tags").RunOut()
	return o
}

func (g GitWrapper) Pull() error {
	err := pkg.NewCommand("git", "-C", g.dir, "pull").RunE()
	return err

}

var fetched = map[string]bool{}

func (g GitWrapper) Fetch(flags ...string) error {
	// don't fetch again if we've already fetched during this run
	if !fetched[g.dir] {
		args := append([]string{"-C", g.dir, "fetch"}, flags...)
		err := pkg.NewCommand("git", args...).RunE()
		if err != nil {
			return err
		}
		fetched[g.dir] = true
	}
	return nil
}

func (g GitWrapper) IsDirty() bool {
	out, err := pkg.NewCommand("git", "-C", g.dir, "diff", "HEAD").RunOut()
	if len(out) > 0 || err != nil {
		return true
	}
	out, err = pkg.NewCommand("git", "-C", g.dir, "status", "--short").RunOut()
	if len(out) > 0 || err != nil {
		return true
	}

	return false
}

func (g GitWrapper) Log(args ...string) ([]string, error) {
	args = append([]string{"-C", g.dir, "log"}, args...)
	out, err := pkg.NewCommand("git", args...).RunOut()
	lines := strings.Split(out, "\n")
	return lines, err
}

func (g GitWrapper) CreateBranch(branch string) error {
	pkg.Log.Infof("Creating branch %q.", branch)
	_, err := g.Exec("checkout", "-B", branch)
	if err != nil {
		return errors.Wrap(err, "create branch")
	}

	pkg.Log.WithField("branch", branch).Info("Pushing branch.")
	err = pkg.NewCommand("git", "push", "-u", "origin", branch).RunE()
	if err != nil {
		return errors.Wrap(err, "push branch")
	}

	return nil
}
