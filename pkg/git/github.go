package git

import (
	"context"
	"github.com/google/go-github/v20/github"
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
