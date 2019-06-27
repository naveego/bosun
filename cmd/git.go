package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fatih/color"
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// gitCmd represents the git command
var gitCmd = &cobra.Command{
	Use:   "git",
	Short: "Git commands.",
}

func init() {

	gitDeployCmd.AddCommand(gitDeployStartCmd)
	gitDeployCmd.AddCommand(gitDeployUpdateCmd)

	gitCmd.AddCommand(gitDeployCmd)

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

var gitDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy-related commands.",
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

var gitDeployStartCmd = &cobra.Command{
	Use:   "start {cluster}",
	Args:  cobra.ExactArgs(1),
	Short: "Notifies github that a deploy has happened.",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := mustGetGithubClient()

		cluster := args[0]
		sha := pkg.NewCommand("git rev-parse HEAD").MustOut()
		isProd := cluster == "blue"

		deploymentRequest := &github.DeploymentRequest{
			Description:           github.String(fmt.Sprintf("Deployment to %s", cluster)),
			Environment:           &cluster,
			Ref:                   &sha,
			ProductionEnvironment: &isProd,
			Task:                  github.String("deploy"),
		}

		org, repo := getOrgAndRepo()

		deployment, _, err := client.Repositories.CreateDeployment(context.Background(), org, repo, deploymentRequest)
		if err != nil {
			return err
		}

		id := *deployment.ID
		fmt.Println(id)
		return nil
	},
}

var gitDeployUpdateCmd = &cobra.Command{
	Use:   "update {deployment-id} {success|failure}",
	Args:  cobra.ExactArgs(2),
	Short: "Notifies github that a deploy has happened.",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := mustGetGithubClient()

		org, repo := getOrgAndRepo()

		deploymentID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return errors.Wrap(err, "invalid deployment ID")
		}

		req := &github.DeploymentStatusRequest{
			State: &args[1],
		}

		_, _, err = client.Repositories.CreateDeploymentStatus(context.Background(), org, repo, deploymentID, req)
		if err != nil {
			return err
		}
		return nil
	},
}

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
		body = fmt.Sprintf("%s\nCloses #%s", c.Body, rawIssueNumber)
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

const (
	ArgPullRequestReviewers = "reviewer"
	ArgPullRequestTitle     = "title"
	ArgPullRequestBody      = "body"
	ArgPullRequestBase      = "base"
)

var gitAcceptPullRequestCmd = addCommand(gitCmd, &cobra.Command{
	Use:           "accept-pull-request [number] [major|minor|patch|major.minor.patch]",
	Aliases:       []string{"accept-pr", "accept"},
	Args:          cobra.RangeArgs(1, 2),
	SilenceUsage:  true,
	SilenceErrors: true,
	Short:         "Accepts a pull request and merges it into master, optionally bumping the version and tagging the master branch.",
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		var err error
		wd, _ := os.Getwd()
		repoPath, err := git.GetRepoPath(wd)
		if err != nil {
			return err
		}

		prNumber, err := strconv.Atoi(args[0])
		if err != nil {
			return err
		}

		ecmd := GitAcceptPRCommand{
			RepoDirectory: repoPath,
			PRNumber:      prNumber,
		}

		if len(args) > 1 {
			ecmd.VersionBump = args[1]
			b := MustGetBosun()

			appsToBump := viper.GetStringSlice(ArgGitAcceptPRAppVersion)
			app, err := getFilterParams(b, appsToBump).IncludeCurrent().GetApp()

			if err != nil {
				return errors.Wrap(err, "could not get app to version")
			}

			ecmd.AppsToVersion = []*bosun.App{app}
		}

		return ecmd.Execute()
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().StringSlice(ArgGitAcceptPRAppVersion, []string{}, "Apps to apply version bump to.")
})

const ArgGitAcceptPRAppVersion = "app"

type GitAcceptPRCommand struct {
	PRNumber      int
	RepoDirectory string
	// if true, will skip merging the base branch back into the pr branch before merging into the target.
	DoNotMergeBaseIntoBranch bool
	AppsToVersion            []*bosun.App
	VersionBump              string
}

func (c GitAcceptPRCommand) Execute() error {
	var err error
	var out string

	client := mustGetGithubClient()
	repoPath, err := git.GetRepoPath(c.RepoDirectory)
	if err != nil {
		return err
	}
	org, repo := git.GetOrgAndRepoFromPath(repoPath)
	g, _ := git.NewGitWrapper(repoPath)

	number := c.PRNumber

	pr, _, err := client.PullRequests.Get(context.Background(), org, repo, number)
	if err != nil {
		return errors.Errorf("could not get pull request %d: %s", number, err)
	}
	baseBranch := pr.GetBase().GetRef()

	if pr.ClosedAt != nil {
		return errors.Errorf("already closed at %s", *pr.ClosedAt)
	}

	stashed := false
	if g.IsDirty() {
		stashed = true
		pkg.Log.Info("Stashing changes before merge...")
		out, err = g.Exec("stash")
		check(err, "tried to stash before merge, but: %s", out)
	}

	currentBranch := g.Branch()
	defer func() {
		pkg.Log.Info("Returning to branch you were on before merging...")
		checkMsg(g.Exec("checkout", currentBranch))
		if stashed {
			pkg.Log.Info("Applying stashed changes...")
			checkMsg(g.Exec("stash", "apply"))
		}
	}()

	if err = checkHandle(g.Fetch()); err != nil {
		return errors.Wrap(err, "checking handle")
	}

	mergeBranch := pr.GetHead().GetRef()

	out, err = g.Exec("branch")
	if err = checkHandle(err); err != nil {
		return err
	}
	if strings.Contains(out, mergeBranch) {
		pkg.Log.Infof("Checking out merge branch %q", mergeBranch)
		if err = checkHandleMsg(g.Exec("checkout", mergeBranch)); err != nil {
			return err
		}
	} else {
		pkg.Log.Infof("Pulling and checking out merge branch %q", mergeBranch)
		if err = checkHandleMsg(g.Exec("checkout", "-b", mergeBranch, "origin/"+pr.GetBase().GetRef())); err != nil {
			return err
		}
	}

	if err = checkHandleMsg(g.Exec("pull")); err != nil {
		return err
	}

	defer func() {
		pkg.Log.Infof("Cleaning up merge branch %q", mergeBranch)
		g.Exec("branch", "-d", mergeBranch)
	}()

	if !c.DoNotMergeBaseIntoBranch {
		pkg.Log.Infof("Merging %s into %s...", baseBranch, mergeBranch)
		out, err = g.Exec("merge", baseBranch)

		if !pr.GetMergeable() || err != nil {
			return errors.Errorf("merge conflicts exist on branch %s, please resolve before trying again: %s", mergeBranch, err)
		}
	}

	if len(c.AppsToVersion) > 0 {
		b := MustGetBosun()
		var finalVersion string
		bump := c.VersionBump

		for _, app := range c.AppsToVersion {

			pkg.Log.Infof("Bumping version (%s) for %s...", bump, app.Name)

			err = appBump(b, app, bump)
			if err != nil {
				return err
			}

			finalVersion = app.Version.String()
		}

		out, err := g.Exec("add", ".")
		if err != nil {
			return checkHandle(err)
		}
		fmt.Println(out)

		out, err = g.Exec("commit", "-m", fmt.Sprintf("Bumping version to %s while approving PR %d", finalVersion, number))
		if err != nil {
			return checkHandle(err)
		}
		fmt.Println(out)

		pkg.Log.Infof("Tagging merge with (%s)...", finalVersion)
		if err = checkHandleMsg(g.Exec("tag", finalVersion, "--force")); err != nil {
			return err
		}

		pkg.Log.Info("Pushing tagged merge...")
		if err = checkHandleMsg(g.Exec("push", "origin", mergeBranch, "--tags")); err != nil {
			return err
		}
	}

	pkg.Log.Infof("Checking out %s...", baseBranch)
	if err = checkHandleMsg(g.Exec("checkout", baseBranch)); err != nil {
		return err
	}

	pkg.Log.Infof("Pulling %s...", baseBranch)
	if err = checkHandleMsg(g.Exec("pull")); err != nil {
		return err
	}

	pkg.Log.Infof("Merging %s into %s...", mergeBranch, baseBranch)
	out, err = g.Exec("merge", "--no-ff", mergeBranch, "-m", fmt.Sprintf("%s\n%s\nMerge of PR #%d", pr.GetTitle(), pr.GetBody(), number))
	if err != nil {
		return errors.Errorf("could not merge into %s", baseBranch)
	}

	pkg.Log.Infof("Pushing %s...", baseBranch)
	if err = checkHandleMsg(g.Exec("push", "origin", baseBranch)); err != nil {
		return err
	}

	pkg.Log.Info("Merge completed.")


	segs := regexp.MustCompile(`(issue)/\#(\d+)/([\s\S]*)`).FindStringSubmatch(mergeBranch)
	if len(segs) == 0 {
		return errors.New("bad branch")
	}

	b := MustGetBosun()
	_, svc, err := b.GetIssueService(repoPath)
	if err != nil {
		return errors.New("get issue service")
	}

	issNum, err := strconv.Atoi(segs[2])
	if err != nil {
		return errors.New("get issue number from branch name")
	}
	prIssRef := issues.NewIssueRef(org, repo, issNum)

	// move task to "Done"
	err = svc.SetProgress(prIssRef, issues.ColumnDone)
	if err != nil {
		return errors.Wrap(err, "move task to done")
	}

	/* parents, err := svc.GetParents(prIssRef)
	if err != nil {
		return errors.Wrap(err, "get parents for current issue")
	}
	if len(parents) > 0 {

		parent := parents[0]

		parentIssueRef := issues.NewIssueRef(parent.Org, parent.Repo, parent.Number)

		allChildren, err := svc.GetChildren(parentIssueRef)
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



func getOrgAndRepo() (string, string) {
	return git.GetCurrentOrgAndRepo()
}

func dumpJSON(label string, data interface{}) {
	if viper.GetBool(ArgGlobalVerbose) {
		j, _ := json.MarshalIndent(data, "", "  ")
		fmt.Fprintf(os.Stderr, "%s:\n%s\n\n", label, string(j))
	}
}
