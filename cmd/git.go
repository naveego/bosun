package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/git"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
	"log"
	"os"
	"path/filepath"
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

func getGitClient() *github.Client {
	token, ok := os.LookupEnv("GITHUB_TOKEN")
	if !ok {
		log.Fatal("GITHUB_TOKEN must be set")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	return client
}

var gitDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy-related commands.",
}

var gitDeployStartCmd = &cobra.Command{
	Use:   "start {cluster}",
	Args:  cobra.ExactArgs(1),
	Short: "Notifies github that a deploy has happened.",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getGitClient()

		cluster := args[0]
		sha := pkg.NewCommand("git rev-parse HEAD").MustOut()
		isProd := cluster == "blue"

		deploymentRequest := &github.DeploymentRequest{
			Description: github.String(fmt.Sprintf("Deployment to %s", cluster)),
			Environment: &cluster,
			Ref:         &sha,
			ProductionEnvironment: &isProd,
			Task:github.String("deploy"),
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
		client := getGitClient()

		org, repo := getOrgAndRepo()

		deploymentID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return errors.Wrap(err, "invalid deployment ID")
		}

		req := &github.DeploymentStatusRequest{
			State:&args[1],
		}

		_, _, err = client.Repositories.CreateDeploymentStatus(context.Background(), org, repo, deploymentID, req)
		if err != nil {
			return err
		}
		return nil
	},
}

var ArgPullRequestReviewers = "reviewer"
var ArgPullRequestTitle = "title"
var ArgPullRequestBody = "body"

var issueNumberRE = regexp.MustCompile(`issue/#?(\d+)`)

var gitPullRequestCmd = addCommand(gitCmd, &cobra.Command{
	Use:   "pull-request",
	Aliases:[]string{"pr"},
	Short: "Opens a pull request.",
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		client := getGitClient()

		org, repo := getOrgAndRepo()
		wd, _ := os.Getwd()
		g, _ := git.NewGitWrapper(wd)
		branch := g.Branch()
		m := issueNumberRE.FindStringSubmatch(branch)
		if len(m) == 0 {
			return errors.Errorf("could not find issue number in branch %q", branch)
		}

		issueNumber := m[1]

		title := viper.GetString(ArgPullRequestTitle)
		if title == "" {
			title =fmt.Sprintf("Merge %s", branch)
		}
		body := fmt.Sprintf("%s\nCloses #%s", viper.GetString(ArgPullRequestBody), issueNumber)

		req := &github.NewPullRequest{
			Title: &title,
			Body: &body,
			Base: github.String("master"),
			Head: &branch,
		}

		issue, _, err := client.PullRequests.Create(context.Background(), org, repo, req)

		if err != nil {
			return err
		}

		fmt.Printf("Created PR #%d.\n", *issue.ID)

		reviewers := viper.GetStringSlice(ArgPullRequestReviewers)
		if len(reviewers) > 0 {
			revRequest := github.ReviewersRequest{
				Reviewers: reviewers,
			}
			_, _, err = client.PullRequests.RequestReviewers(context.Background(), org, repo, *issue.Number, revRequest)
			if err != nil {
				return err
			}
		}

		return nil
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().StringSlice(ArgPullRequestReviewers, []string{}, "Reviewers to request.")
	cmd.Flags().String(ArgPullRequestTitle, "", "Title of PR")
	cmd.Flags().String(ArgPullRequestBody, "", "Body of PR")
})


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
		client := getGitClient()
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
	ArgGitTaskStory  = "story"
	ArgGitTaskParentOrg  = "parent-org"
	ArgGitTaskParentRepo = "parent-repo"
)

func getOrgAndRepo() (string,string){
	currentDir, _ := os.Getwd()
	repo := filepath.Base(currentDir)
	org := filepath.Base(filepath.Dir(currentDir))
	return org, repo
}

func dumpJSON(label string, data interface{}) {
	if viper.GetBool(ArgGlobalVerbose) {
		j, _ := json.MarshalIndent(data, "", "  ")
		fmt.Fprintf(os.Stderr, "%s:\n%s\n\n", label, string(j))
	}
}
