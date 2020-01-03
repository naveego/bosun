package git

import (
	"context"
	"fmt"
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"net/http"
)

// GetGithubClient gets a github client. If token == "" the client will not be authenticated.
func NewGithubClient(token string) *github.Client {
	if token == "" {
		return github.NewClient(http.DefaultClient)
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(context.Background(), ts)

	client := github.NewClient(tc)

	return client
}

func CloneRepo(ref issues.RepoRef, protocol string, dir string) error {
	var remote string
	switch protocol {
	case "ssh":
		pkg.Log.Infof("Cloning %s using SSH protocol (set `githubCloneProtocol: https` in workspace to use HTTPS).", ref)
		remote = fmt.Sprintf("git@github.com:%s.git", ref)
	case "http", "https":
		pkg.Log.Infof("Cloning %s using HTTPS protocol (set `githubCloneProtocol: ssh` in workspace to use SSH).", ref)
		remote = fmt.Sprintf("https://github.com/%s.git", ref)
	default:
		return errors.Errorf("invalid protocol %q", protocol)
	}

	err := pkg.NewShellExe("git", "clone", remote).WithDir(dir).RunE()
	return err
}
