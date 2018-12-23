package git

import "github.com/naveego/bosun/pkg"

type GitWrapper struct {
	dir string
}

func NewGitWrapper(pathHint string) (GitWrapper, error) {
	dir, err := GetRepoPath(pathHint)
	if err != nil {
		return GitWrapper{}, err
	}
	return GitWrapper{
		dir:dir,
	}, nil
}

func (g GitWrapper) Branch() string {
	o, _ := pkg.NewCommand("git", "-C", g.dir, "rev-parse", "--abbrev-ref", "HEAD").RunOut()
	return o
}

func (g GitWrapper) Tag() string {
	o, _ := pkg.NewCommand("git", "-C", g.dir, "describe", "--abbrev=0", "--tags").RunOut()
	return o
}