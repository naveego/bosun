package cmd

import (
	"context"
	"fmt"
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
 "golang.org/x/oauth2"
	"path/filepath"
	"strconv"
	"strings"
)

// gitCmd represents the git command
var gitCmd = &cobra.Command{
	Use:   "git",
	Short: "Git commands.",
}

var gitTaskCmd = &cobra.Command{
	Use:   "task {story number} {task name}",
	Args: cobra.ExactArgs(2),
	Short: "Creates a task in the current repo for the story, and a branch for that task.",
	Long:"Requires github hub tool to be installed (https://hub.github.com/).",
	RunE: func(cmd *cobra.Command, args []string) error {

		var err error

		viper.BindPFlags(cmd.Flags())
		
		currentDir, _ := os.Getwd()
		segs := filepath.SplitList(currentDir)
		org, repo := segs[len(segs) - 2], segs[len(segs) - 1]
		

		storyNumber, err := strconv.Atoi(strings.Trim(args[0], "#"))
		taskName := args[1]
		if err != nil {
			return errors.Wrap(err, "issue number must be a number")
		}

		token, ok := os.LookupEnv("GITHUB_TOKEN")
		if !ok {
			return errors.New("GITHUB_TOKEN must be set")
		}

		ctx := context.Background()
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		tc := oauth2.NewClient(ctx, ts)

		client := github.NewClient(tc)

		story, _, err := client.Issues.Get(ctx, "naveegoinc", "stories", storyNumber)
		if err != nil {
			return errors.Wrap(err, "get issue")
		}




		body := viper.GetString(ArgGitBody)
		issueRequest := &github.IssueRequest{
			Title: github.String(taskName),
			Body: github.String(fmt.Sprintf("%s\n\nrequired by naveegoinc/stories#%d", body, storyNumber)),
		}
		if story.Assignee != nil {
			issueRequest.Assignee = story.Assignee.Name
		}
		if story.Milestone != nil {
			milestones, _, err := client.Issues.ListMilestones(ctx, org, repo, nil)
			if err != nil {
				return err
			}
			for _, m := range milestones {
				if m.Title == story.Milestone.Title {
					issueRequest.Milestone = m.Number
					break
				}
			}
		}

		pkg.Log.Info("Creating issue.")

		issue, _, err := client.Issues.Create(ctx, org, repo, issueRequest)
		if err != nil {
			return errors.Wrap(err, "creating issue")
		}

		issueNumber := issue.Number
		pkg.Log.WithField("issue", issueNumber).Info("Created issue.")

		slug := strings.Replace(strings.ToLower(taskName), " ", "-", -1)
		branchName := fmt.Sprintf("issue/#%d/%s", issueNumber, slug)
		pkg.Log.WithField("branch", branchName).Info("Creating branch.")
		err = pkg.NewCommand("git", "checkout", "-b", branchName).RunE()
		if err != nil{
			return err
		}

		pkg.Log.WithField("branch", branchName).Info("Pushing branch.")
		err = pkg.NewCommand("git", "push", "-u", "origin", branchName).RunE()
		if err != nil {
			return err
		}

		return nil
	},
}

const(
	ArgGitBody = "body"
)



func init() {

	gitCmd.AddCommand(gitTaskCmd)
	gitTaskCmd.Flags().StringP(ArgGitBody, "m", "", "Issue body.")

	rootCmd.AddCommand(gitCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// gitCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// gitCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
