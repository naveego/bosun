package git

import (
	"context"
	"fmt"
	"github.com/fatih/color"
	"github.com/google/go-github/v20/github"
	"regexp"
	"strconv"
)

type GitPullRequestCommand struct {
	Reviewers     []string
	Title         string
	Body          string
	Base          string
	FromBranch    string
	LocalRepoPath string
	Client        *github.Client
}

var issueNumberRE = regexp.MustCompile(`issue/#?(\d+)`)

func (c GitPullRequestCommand) Execute() (issueNmb, prNumber int, err error) {
	client := c.Client

	repoPath := c.LocalRepoPath
	org, repo := GetRepoRefFromPath(repoPath).OrgAndRepo()

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
