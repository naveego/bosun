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
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
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
	b := mustGetBosun()
	ws := b.GetWorkspace()
	ctx := b.NewContext().WithDir(ws.Path)
	if ws.GithubToken == nil {
		fmt.Println("Github token was not found. Please provide a command that can be run to obtain a github token.")
		fmt.Println(`Simple example: echo "9uha09h39oenhsir98snegcu"`)
		fmt.Println(`Better example: cat $HOME/.tokens/github.token"`)
		fmt.Println(`Secure example: lpass show "Tokens/GithubCLIForBosun" --notes"`)
		script := pkg.RequestStringFromUser("Command")

		ws.GithubToken = &bosun.CommandValue{
			Command: bosun.Command{
				Script: script,
			},
		}

		_, err := ws.GithubToken.Resolve(ctx)
		if err != nil {
			return "", errors.Errorf("script failed: %s\nscript:\n%s", err, script)
		}

		err = b.Save()
		if err != nil {
			return "", errors.Errorf("save failed: %s", err)
		}
	}

	token, err := ws.GithubToken.Resolve(ctx)
	if err != nil {
		return "", err
	}

	err = os.Setenv("GITHUB_TOKEN", token)
	if err != nil {
		return "", err
	}

	token, ok := os.LookupEnv("GITHUB_TOKEN")
	if !ok {
		return "", errors.Errorf("GITHUB_TOKEN must be set")
	}

	return token, nil
}

func mustGetGithubClient() *github.Client {

	token, err := getGithubToken()
	if err != nil {
		log.Fatal(err)
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(context.Background(), ts)

	client := github.NewClient(tc)

	return client
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

var gitPullRequestCmd = addCommand(gitCmd, &cobra.Command{
	Use:     "pull-request",
	Aliases: []string{"pr"},
	Short:   "Opens a pull request.",
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		repoPath, err := git.GetCurrentRepoPath()
		if err != nil {
			return err
		}

		g, err := git.NewGitWrapper(repoPath)
		if err != nil {
			return err
		}

		prCmd := GitPullRequestCommand{
			LocalRepoPath: repoPath,
			Reviewers:     viper.GetStringSlice(ArgPullRequestReviewers),
			Base:          viper.GetString(ArgPullRequestBase),
			FromBranch:    g.Branch(),
			Body:          viper.GetString(ArgPullRequestBody),
		}

		_, err = prCmd.Execute()

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().StringSlice(ArgPullRequestReviewers, []string{}, "Reviewers to request.")
	cmd.Flags().String(ArgPullRequestTitle, "", "Title of PR")
	cmd.Flags().String(ArgPullRequestBody, "", "Body of PR")
	cmd.Flags().String(ArgPullRequestBase, "master", "Target branch for merge.")
})

type GitPullRequestCommand struct {
	Reviewers     []string
	Title         string
	Body          string
	Base          string
	FromBranch    string
	LocalRepoPath string
}

func (c GitPullRequestCommand) Execute() (prNumber int, err error) {
	client := mustGetGithubClient()

	repoPath := c.LocalRepoPath
	org, repo := git.GetOrgAndRepoFromPath(repoPath)

	branch := c.FromBranch
	m := issueNumberRE.FindStringSubmatch(branch)
	var issueNumber string
	if len(m) == 0 {
		color.Yellow("No issue number in branch.")
	} else {
		issueNumber = m[1]
	}

	title := c.Title
	if title == "" {
		title = fmt.Sprintf("Merge %s into %s", branch, c.Base)
	}

	body := fmt.Sprintf("%s\nCloses #%s", c.Body, issueNumber)

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
		return 0, err
	}

	fmt.Printf("Created PR #%d.\n", issue.GetNumber())

	if len(c.Reviewers) > 0 {
		revRequest := github.ReviewersRequest{
			Reviewers: c.Reviewers,
		}
		_, _, err = client.PullRequests.RequestReviewers(context.Background(), org, repo, *issue.Number, revRequest)
		if err != nil {
			return 0, err
		}
	}

	return issue.GetNumber(), nil
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
			b := mustGetBosun()

			app, err := getFilterParams(b, args).GetApp()

			if err != nil {
				return errors.Wrap(err, "could not get app to version")
			}

			ecmd.AppsToVersion = []*bosun.AppRepo{app}
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
	AppsToVersion            []*bosun.AppRepo
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
		return err
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
		b := mustGetBosun()
		var finalVersion string
		bump := c.VersionBump

		for _, app := range c.AppsToVersion {

			pkg.Log.Infof("Bumping version (%s) for %s...", bump, app.Name)

			err = appBump(b, app, bump)
			if err != nil {
				return err
			}

			finalVersion = app.Version
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

	return nil
}

var gitTaskCmd = addCommand(gitCmd, &cobra.Command{
	Use:   "task {task name}",
	Args:  cobra.ExactArgs(1),
	Short: "Creates a task in the current repo, and a branch for that task. Optionally attaches task to a story, if flags are set.",
	Long:  `Requires github hub tool to be installed (https://hub.github.com/).`,
	RunE: func(cmd *cobra.Command, args []string) error {

		var err error

		viper.BindPFlags(cmd.Flags())

		org, repo := git.GetCurrentOrgAndRepo()

		body := viper.GetString(ArgGitBody)
		taskName := args[0]
		client := mustGetGithubClient()
		issueRequest := &github.IssueRequest{
			Title: github.String(taskName),
		}
		ctx := context.Background()

		storyNumber := viper.GetInt(ArgGitTaskStory)
		if storyNumber > 0 {

			parentOrg := viper.GetString(ArgGitTaskParentOrg)
			parentRepo := viper.GetString(ArgGitTaskParentRepo)

			storyNumber := viper.GetInt(ArgGitTaskStory)

			story, _, err := client.Issues.Get(ctx, parentOrg, parentRepo, storyNumber)
			if err != nil {
				return errors.Wrap(err, "get issue")
			}

			body = fmt.Sprintf("%s\n\nrequired by %s/%s#%d", body, parentOrg, parentRepo, storyNumber)
			// dumpJSON("story", story)

			if story.Assignee != nil {
				issueRequest.Assignee = story.Assignee.Name
			}
			if story.Milestone != nil {
				milestones, _, err := client.Issues.ListMilestones(ctx, org, repo, nil)
				dumpJSON("milestones", milestones)

				if err != nil {
					return err
				}
				for _, m := range milestones {
					if *m.Title == *story.Milestone.Title {
						pkg.Log.WithField("title", *m.Title).Info("Attaching milestone.")
						issueRequest.Milestone = m.Number
						break
					}
				}
			}
		}

		issueRequest.Body = &body

		dumpJSON("creating issue", issueRequest)

		issue, _, err := client.Issues.Create(ctx, org, repo, issueRequest)
		if err != nil {
			return errors.Wrap(err, "creating issue")
		}

		issueNumber := *issue.Number
		pkg.Log.WithField("issue", issueNumber).Info("Created issue.")

		slug := regexp.MustCompile(`\W+`).ReplaceAllString(strings.ToLower(taskName), "-")
		branchName := fmt.Sprintf("issue/#%d/%s", issueNumber, slug)
		pkg.Log.WithField("branch", branchName).Info("Creating branch.")
		err = pkg.NewCommand("git", "checkout", "-b", branchName).RunE()
		if err != nil {
			return err
		}

		pkg.Log.WithField("branch", branchName).Info("Pushing branch.")
		err = pkg.NewCommand("git", "push", "-u", "origin", branchName).RunE()
		if err != nil {
			return err
		}

		return nil
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().StringP(ArgGitBody, "m", "", "Issue body.")
	cmd.Flags().String(ArgGitTaskParentOrg, "naveegoinc", "Story org.")
	cmd.Flags().String(ArgGitTaskParentRepo, "stories", "Story repo.")
	cmd.Flags().Int(ArgGitTaskStory, 0, "Number of the story to use as a parent.")
})

const (
	ArgGitBody           = "body"
	ArgGitTaskStory      = "story"
	ArgGitTaskParentOrg  = "parent-org"
	ArgGitTaskParentRepo = "parent-repo"
)

func getOrgAndRepo() (string, string) {
	return git.GetCurrentOrgAndRepo()
}

func dumpJSON(label string, data interface{}) {
	if viper.GetBool(ArgGlobalVerbose) {
		j, _ := json.MarshalIndent(data, "", "  ")
		fmt.Fprintf(os.Stderr, "%s:\n%s\n\n", label, string(j))
	}
}
