package git

import (
	"context"
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"os"
	"path/filepath"
	"strings"
)

func GetCurrentOrgAndRepo() issues.RepoRef {
	currentDir, _ := os.Getwd()
	repoDir, err := GetRepoPath(currentDir)
	if err != nil {
		panic(err)
	}
	return GetRepoRefFromPath(repoDir)
}

func GetCurrentRepoPath() (string, error) {

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return GetRepoPath(wd)
}

func GetRepoPath(path string) (string, error) {
	var err error
	if path == "" {
		path, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	original := path
	path, _ = filepath.Abs(path)
	stat, err := os.Stat(path)
	if err == nil && !stat.IsDir() {
		path = filepath.Dir(path)
	}

	repoPath, err := pkg.NewShellExe("git", "-C", path, "rev-parse", "--show-toplevel").RunOut()
	if err != nil {
		return "", errors.Errorf("could not get repo for path %q (based on path %q): %s", path, original, err)
	}
	return repoPath, nil
}

func GetRepoRefFromPath(path string) issues.RepoRef {

	g, _ := NewGitWrapper(path)
	out, _ := g.Exec("config", "--get", "remote.origin.url")
	repoURL := strings.Split(out, ":")
	if len(repoURL) > 1 {
		path = strings.TrimSuffix(repoURL[1], ".git")
	}

	return issues.RepoRef{
		Repo: filepath.Base(path),
		Org: filepath.Base(filepath.Dir(path)),
	}
}

func mustGetGitClient(token string) *github.Client {
	if token == "" {
		var ok bool
		token, ok = os.LookupEnv("GITHUB_TOKEN")
		if !ok {
			pkg.Log.Fatal("GITHUB_TOKEN must be set")
		}
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	return client
}
