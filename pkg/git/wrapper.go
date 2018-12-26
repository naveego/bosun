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

func (g GitWrapper) Exec(args ...string) (string, error) {
	args = append([]string{"-C", g.dir}, args...)

	out, err := pkg.NewCommand("git", args...).RunOut()
	if err != nil {
		return "", errors.Errorf("git %s: %s", strings.Join(args, " "), err)
	}
	return out, err
}

func (g GitWrapper) Branch() string {
	o, _ := pkg.NewCommand("git", "-C", g.dir, "rev-parse", "--abbrev-ref", "HEAD").RunOut()
	return o
}

func (g GitWrapper) Commit() string {
	o, _ := pkg.NewCommand("git", "-C", g.dir, "log", "--pretty=format:'%h'", "-n", "1").RunOut()
	return o
}

func (g GitWrapper) Tag() string {
	o, _ := pkg.NewCommand("git", "-C", g.dir, "describe", "--abbrev=0", "--tags").RunOut()
	return o
}

func (g GitWrapper) Pull() error {
	err := pkg.NewCommand("git", "-C", g.dir, "pull").RunE()
	return err

}
