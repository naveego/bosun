package git

import (
	"context"
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"os"
	"path/filepath"
)

func GetCurrentOrgAndRepo() (string, string) {
	currentDir, _ := os.Getwd()
	repoDir, err := GetRepoPath(currentDir)
	if err != nil {
		panic(err)
	}
	return GetOrgAndRepoFromPath(repoDir)
}

func GetRepoPath(path string) (string, error) {
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
	repo := filepath.Base(path)
	org := filepath.Base(filepath.Dir(path))
	return org, repo
}

func mustGetGitClient() *github.Client {
	token, ok := os.LookupEnv("GITHUB_TOKEN")
	if !ok {
		pkg.Log.Fatal("GITHUB_TOKEN must be set")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	return client
}
