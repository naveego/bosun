package git

import (
	"context"
	"fmt"
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/pkg/errors"
	"regexp"
	"strconv"
	"time"
)

type GitAcceptPRCommand struct {
	PRNumber      int
	RepoDirectory string
	// if true, will skip merging the base branch back into the pr branch before merging into the target.
	DoNotMergeBaseIntoBranch bool
	Client                   *github.Client
	IssueService             issues.IssueService
}

func (c GitAcceptPRCommand) Execute() error {
	var err error

	client := c.Client
	repoPath, err := GetRepoPath(c.RepoDirectory)
	if err != nil {
		return err
	}
	org, repo := GetRepoRefFromPath(repoPath).OrgAndRepo()

	number := c.PRNumber

GetPR:
	pr, _, err := client.PullRequests.Get(context.Background(), org, repo, number)
	if err != nil {
		return errors.Errorf("could not get pull request %d: %s", number, err)
	}
	mergeBranch := pr.GetHead().GetRef()

	if pr.ClosedAt != nil {
		return errors.Errorf("already closed at %s", *pr.ClosedAt)
	}

	if pr.CreatedAt.After(time.Now().Add(-5 * time.Second)) {
		pkg.Log.Warnf("PR is very new, waiting a little while before trying to merge.")
		<-time.After(5 * time.Second)
		goto GetPR
	}

	if pr.GetMergeable() == false {
		return errors.Errorf(`pr not mergeable: %s; please merge %s into %s, push to github, then try again`,
			pr.GetMergeableState(),
			pr.GetBase().GetRef(),
			pr.GetHead().GetRef())
	}

	mergeMessage := fmt.Sprintf("%s\n%s\nMerge of PR #%d", pr.GetTitle(), pr.GetBody(), number)
	mergeResult, _, err := client.PullRequests.Merge(context.Background(), org, repo, number, mergeMessage, &github.PullRequestOptions{})

	if err != nil {
		return errors.Wrap(err, "github merge failed")
	}

	if !mergeResult.GetMerged() {
		return errors.Errorf("merge failed mysteriously with message %q", mergeResult.GetMessage())
	}


	segs := regexp.MustCompile(`(issue)/#?(\d+)/([\s\S]*)`).FindStringSubmatch(mergeBranch)
	if len(segs) == 0 {
		pkg.Log.Warn("Branch did not contain an issue number, not attempting to close issues.")
		return nil
	}

	issNum, err := strconv.Atoi(segs[2])
	if err != nil {
		return errors.New("get issue number from branch name")
	}
	prIssRef := issues.NewIssueRef(org, repo, issNum)

	// move task to "Done"
	if c.IssueService != nil {
		err = c.IssueService.SetProgress(prIssRef, issues.ColumnDone)
		if err != nil {
			pkg.Log.Warnf("Could not move issue %s to done status, you'll need to do that yourself. (error: %s)", prIssRef, err)
		}
	}

	/* parents, err := svc.GetParentRefs(prIssRef)
	if err != nil {
		return errors.Wrap(err, "get parents for current issue")
	}
	if len(parents) > 0 {

		parent := parents[0]

		parentIssueRef := issues.NewIssueRef(parent.Org, parent.Repo, parent.Number)

		allChildren, err := svc.GetChildRefs(parentIssueRef)
		if err != nil {
			return errors.New("get all children of parent issue")
		}

		var ok = true
		for _, child := range allChildren {
			if !child.IsClosed {
				ok = false
				break
			}
		}
		if ok {
			err = svc.SetProgress(parentIssueRef, issues.ColumnWaitingForUAT)
			if err != nil {
				return errors.New("move parent story to Waiting for UAT")
			}
		}
	} */

	return nil
}
