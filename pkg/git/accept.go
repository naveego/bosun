package git

import (
	"context"
	"fmt"
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
	"regexp"
	"strconv"
	"strings"
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
	var out string

	client := c.Client
	repoPath, err := GetRepoPath(c.RepoDirectory)
	if err != nil {
		return err
	}
	org, repo := GetOrgAndRepoFromPath(repoPath)
	g, _ := NewGitWrapper(repoPath)

	number := c.PRNumber

	pr, _, err := client.PullRequests.Get(context.Background(), org, repo, number)
	if err != nil {
		return errors.Errorf("could not get pull request %d: %s", number, err)
	}
	baseBranch := pr.GetBase().GetRef()

	if pr.ClosedAt != nil {
		return errors.Errorf("already closed at %s", *pr.ClosedAt)
	}

	if pr.GetMergeable() == false {
		return errors.Errorf(`branch %s not mergeable: %s; please merge %s into %s, push to github, then try again`,
			pr.GetBase().GetRef(),
			pr.GetMergeableState(),
			pr.GetHead().GetRef())
	}

	stashed := false
	if g.IsDirty() {
		stashed = true
		pkg.Log.Info("Stashing changes before merge...")
		out, err = g.Exec("stash")
		if err != nil {
			return errors.Wrapf(err, "tried to stash before merge, but: %s", out)
		}
	}

	currentBranch := g.Branch()
	defer func() {
		pkg.Log.Info("Returning to branch you were on before merging...")
		if err = util.CheckHandleMsg(g.Exec("checkout", currentBranch)); err != nil {
			pkg.Log.WithError(err).Error("Could not return to branch.")
		}
		if stashed {
			pkg.Log.Info("Applying stashed changes...")
			if err = util.CheckHandleMsg(g.Exec("stash", "apply")); err != nil {
				pkg.Log.WithError(err).Error("Could not apply stashed changes.")
			}
		}
	}()

	if err = util.CheckHandle(g.Fetch("--all")); err != nil {
		return errors.Wrap(err, "fetching")
	}

	mergeBranch := pr.GetHead().GetRef()

	out, err = g.Exec("branch")
	if err = util.CheckHandle(err); err != nil {
		return err
	}

	if strings.Contains(out, mergeBranch) {
		pkg.Log.Infof("Checking out merge branch %q", mergeBranch)
		if err = util.CheckHandleMsg(g.Exec("checkout", mergeBranch)); err != nil {
			return err
		}
	} else {
		pkg.Log.Infof("Pulling and checking out merge branch %q", mergeBranch)
		if err = util.CheckHandleMsg(g.Exec("checkout", "-b", mergeBranch, "origin/"+pr.GetBase().GetRef())); err != nil {
			return err
		}
	}

	if err = util.CheckHandleMsg(g.Exec("pull")); err != nil {
		return err
	}

	if !c.DoNotMergeBaseIntoBranch {
		pkg.Log.Infof("Merging %s into %s...", baseBranch, mergeBranch)
		out, err = g.Exec("merge", baseBranch)

		if !pr.GetMergeable() || err != nil {
			return errors.Errorf("merge conflicts exist on branch %s, please resolve before trying again: %s", mergeBranch, err)
		}
	}

	pkg.Log.Infof("Checking out %s...", baseBranch)
	if err = util.CheckHandleMsg(g.Exec("checkout", baseBranch)); err != nil {
		return err
	}

	pkg.Log.Infof("Pulling %s...", baseBranch)
	if err = util.CheckHandleMsg(g.Exec("pull")); err != nil {
		return err
	}

	pkg.Log.Infof("Merging %s into %s...", mergeBranch, baseBranch)
	out, err = g.Exec("merge", "--no-ff", mergeBranch, "-m", fmt.Sprintf("%s\n%s\nMerge of PR #%d", pr.GetTitle(), pr.GetBody(), number))
	if err != nil {
		return errors.Errorf("could not merge into %s", baseBranch)
	}

	pkg.Log.Infof("Pushing %s...", baseBranch)
	if err = util.CheckHandleMsg(g.Exec("push", "origin", baseBranch)); err != nil {
		return err
	}

	pkg.Log.Info("Merge completed.")

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
	err = c.IssueService.SetProgress(prIssRef, issues.ColumnDone)
	if err != nil {
		return errors.Wrap(err, "move task to done")
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
