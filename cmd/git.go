package cmd

import (
	"context"
	"fmt"
	"github.com/fatih/color"
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/git"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"log"
	"regexp"
	"strconv"
)

// gitCmd represents the git command
var gitCmd = &cobra.Command{
	Use:   "git",
	Short: "Git commands.",
}

func init() {

	rootCmd.AddCommand(gitCmd)
}

func getGithubToken() (string, error) {
	b := MustGetBosun()
	token, err := b.GetGithubToken()

	return token, err
}

func mustGetGithubClient() *github.Client {

	token, err := getGithubToken()
	if err != nil {
		log.Fatal(err)
	}

	client := git.NewGithubClient(token)
	return client
}

func getMaybeAuthenticatedGithubClient() *github.Client {
	token, _ := getGithubToken()
	if token == "" {
		pkg.Log.Warn("No github token could be found, you may be using up a quota with each request.")
	}

	return git.NewGithubClient(token)
}

func getUnauthenticatedGithubClient() *github.Client {
		return git.NewGithubClient("")
}

var gitTokenCmd = addCommand(gitCmd, &cobra.Command{
	Use:   "token",
	Short: "Prints the github token.",
	RunE: func(cmd *cobra.Command, args []string) error {

		token, err := getGithubToken()
		if err != nil {
			return err
		}
		fmt.Println(token)
		return nil
	},
})

var gitRepoCommand = addCommand(gitCmd, &cobra.Command{
	Use:   "repo {app}",
	Args:  cobra.ExactArgs(1),
	Short: "Prints the repo info for the app.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		app := mustGetApp(b, args)

		repoPath, err := git.GetRepoPath(app.FromPath)
		if err != nil {
			return err
		}

		fmt.Println(repoPath)

		org, repo := git.GetOrgAndRepoFromPath(repoPath)

		fmt.Printf("org : %s\n", org)
		fmt.Printf("repo: %s\n", repo)

		return nil
	},
})

var issueNumberRE = regexp.MustCompile(`issue/#?(\d+)`)

type GitPullRequestCommand struct {
	Reviewers     []string
	Title         string
	Body          string
	Base          string
	FromBranch    string
	LocalRepoPath string
}

func (c GitPullRequestCommand) Execute() (issueNmb, prNumber int, err error) {
	client := mustGetGithubClient()

	repoPath := c.LocalRepoPath
	org, repo := git.GetOrgAndRepoFromPath(repoPath)

	branch := c.FromBranch
	m := issueNumberRE.FindStringSubmatch(branch)
	var rawIssueNumber string
	var issueNum int
	if len(m) == 0 {
		color.Yellow("No issue number in branch.")
	} else {
		rawIssueNumber = m[1]
		issueNum, err = strconv.Atoi(rawIssueNumber)
		if err != nil {
			color.Yellow("Invalid issue number %q, will not be able to close issue.", rawIssueNumber)
		}
	}

	title := c.Title
	if title == "" {
		title = fmt.Sprintf("Merge %s into %s", branch, c.Base)
	}

	body := c.Body

	if issueNum > 0 {
		body = fmt.Sprintf("%s\nCloses #%s", body, rawIssueNumber)
	}

	target := c.Base
	if target == "" {
		target = "master"
	}

	req := &github.NewPullRequest{
		Title: &title,
		Body:  &body,
		Base:  &target,
		Head:  &branch,
	}

	issue, _, err := client.PullRequests.Create(context.Background(), org, repo, req)

	if err != nil {
		return 0, 0, err
	}

	fmt.Printf("Created PR #%d.\n", issue.GetNumber())

	if len(c.Reviewers) > 0 {
		revRequest := github.ReviewersRequest{
			Reviewers: c.Reviewers,
		}
		_, _, err = client.PullRequests.RequestReviewers(context.Background(), org, repo, *issue.Number, revRequest)
		if err != nil {
			return 0, 0, err
		}
	}

	return issueNum, issue.GetNumber(), nil
}

func (c GitPullRequestCommand) rebase() error {
	g, err := git.NewGitWrapper(c.LocalRepoPath)
	if err != nil {
		return err
	}

	err = g.Fetch()
	if err != nil {
		return err
	}

	_, err = g.Exec("rebase", "origin", c.Base)
	if err != nil {
		color.Red("Rebase failed, please fix merges and try again.")
		return err
	}

	err = g.Push()
	if err != nil {
		return errors.Wrap(err, "push rebased branch")
	}

	return nil
}

const (
	ArgPullRequestReviewers = "reviewer"
	ArgPullRequestTitle     = "title"
	ArgPullRequestBody      = "body"
	ArgPullRequestBase      = "base"
)
