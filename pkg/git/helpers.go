package git

import (
	"context"
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"os"
	"path/filepath"
	"strings"
)

func GetCurrentOrgAndRepo() (string, string) {
	currentDir, _ := os.Getwd()
	repoDir, err := GetRepoPath(currentDir)
	if err != nil {
		panic(err)
	}
	return GetOrgAndRepoFromPath(repoDir)
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

	repoPath, err := pkg.NewCommand("git", "-C", path, "rev-parse", "--show-toplevel").RunOut()
	if err != nil {
		return "", errors.Errorf("could not get repo for path %q (based on path %q): %s", path, original, err)
	}
	return repoPath, nil
}

func GetOrgAndRepoFromPath(path string) (string, string) {

	g, _ := NewGitWrapper(path)
	out, _ := g.Exec("config", "--get", "remote.origin.url")
	repoURL := strings.Split(out, ":")
	if len(repoURL) > 0 {
		path = strings.TrimSuffix(repoURL[1], ".git")
	}

	repo := filepath.Base(path)
	org := filepath.Base(filepath.Dir(path))
	return org, repo
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
