package cmd

import (
	"context"
	"encoding/json"
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

const(
	ArgGitBody = "body"
	ArgGitTaskParentOrg = "parent-org"
	ArgGitTaskParentRepo = "parent-repo"
)



func init() {

	gitCmd.AddCommand(gitTaskCmd)
	gitTaskCmd.Flags().StringP(ArgGitBody, "m", "", "Issue body.")
	gitTaskCmd.Flags().String(ArgGitTaskParentOrg, "naveegoinc", "Parent org.")
	gitTaskCmd.Flags().String(ArgGitTaskParentRepo, "stories", "Parent repo.")

	rootCmd.AddCommand(gitCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// gitCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// gitCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}



var gitTaskCmd = &cobra.Command{
	Use:   "task {parent-number} {task name}",
	Args: cobra.ExactArgs(2),
	Short: "Creates a task in the current repo for the story, and a branch for that task.",
	Long:"Requires github hub tool to be installed (https://hub.github.com/).",
	RunE: func(cmd *cobra.Command, args []string) error {

		var err error

		viper.BindPFlags(cmd.Flags())
		
		currentDir, _ := os.Getwd()
		repo := filepath.Base(currentDir)
		org := filepath.Base(filepath.Dir(currentDir))
		fmt.Println(org, repo)

		parentOrg := viper.GetString(ArgGitTaskParentOrg)
		parentRepo := viper.GetString(ArgGitTaskParentRepo)

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

		story, _, err := client.Issues.Get(ctx, parentOrg, parentRepo, storyNumber)
		if err != nil {
			return errors.Wrap(err, "get issue")
		}

		dumpJSON("story", story)

		body := viper.GetString(ArgGitBody)
		issueRequest := &github.IssueRequest{
			Title: github.String(taskName),
			Body: github.String(fmt.Sprintf("%s\n\nrequired by %s/%s#%d", body, parentOrg, parentRepo, storyNumber)),
		}
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

		dumpJSON("creating issue", issueRequest)

		issue, _, err := client.Issues.Create(ctx, org, repo, issueRequest)
		if err != nil {
			return errors.Wrap(err, "creating issue")
		}

		issueNumber := *issue.Number
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

func dumpJSON(label string, data interface{}) {
	if viper.GetBool(ArgGlobalVerbose) {
		j, _ := json.MarshalIndent(data, "", "  ")
		fmt.Fprintf(os.Stderr, "%s:\n%s\n\n", label, string(j))
	}
}