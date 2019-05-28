package cmd

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/zenhub"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
)

var gitTaskCmd = addCommand(gitCmd, &cobra.Command{
	Use:   "task {task name}",
	Args:  cobra.ExactArgs(1),
	Short: "Creates a task in the current repo, and a branch for that task. Optionally attaches task to a story, if flags are set.",
	Long:  `Requires github hub tool to be installed (https://hub.github.com/).`,
	RunE: func(cmd *cobra.Command, args []string) error {

		var err error

		err = viper.BindPFlags(cmd.Flags())
		if err != nil {
			return err
		}

		taskName := args[0]
		//org, repo := git.GetCurrentOrgAndRepo()
		title := viper.GetString(ArgGitTitle)
		if title == "" {
			if len(taskName) > 50 {
				title = taskName[:50] + "..."
			} else {
				title = taskName
			}
		}
		body := viper.GetString(ArgGitBody)
		if body == "" {
			body = taskName
		}

		githubToken, err := getGithubToken()
		if err != nil {
			return err
		}

		// Temporally hard-coded for test
		// Needs to use similar get method like what is done for github token
		//zenhubToken := "c009263e9b2b89920a2e27d918b0aaeda9d67f86323bdb9d9c0c873d1752697afc37c38014ea6b10"
		zenhubToken, err := getZenhubToken()
		if err != nil {
			return errors.Wrap(err, "get zenhub token")
		}

		currentRepoPath, err := git.GetCurrentRepoPath()
		if err != nil {
			return err
		}
		g, err := git.NewGitWrapper(currentRepoPath)

		svc, err := zenhub.NewIssueService(githubToken, zenhubToken, g, pkg.Log.WithField("cmp", "zenhub"))
		if err != nil {
			return errors.Wrapf(err, "get story service with tokens %q, %q", githubToken, zenhubToken)
		}

		issue := issues.Issue{
			Title:title,
			Body:body,
		}

		var parent *issues.IssueRef

		storyNumber := viper.GetInt(ArgGitTaskStory)
		if storyNumber > 0 {

			parentOrg := viper.GetString(ArgGitTaskParentOrg)
			parentRepo := viper.GetString(ArgGitTaskParentRepo)

			tmp := issues.NewIssueRef(parentOrg, parentRepo, storyNumber)
			parent = &tmp


			/*parentIssue, _, err := client.Issues.Get(ctx, parentOrg, parentRepo, storyNumber)
			if err != nil {
				return errors.Wrap(err, "get issue")
			} */

			//body = fmt.Sprintf("%s\n\nrequired by %s/%s#%d", body, parentOrg, parentRepo, storyNumber)
			// dumpJSON("story", story)

			/*if story.Milestone != nil {
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
			} */
		}

		err = svc.Create(issue, parent)
		if err != nil {
			return err
		}

		//issueRequest.Body = &body
		//dumpJSON("creating issue", issueRequest)

		/*issue, _, err := client.Issues.Create(ctx, org, repo, issueRequest)
		if err != nil {
			return errors.Wrap(err, "creating issue")
		}

		issueNumber := *issue.Number
		pkg.Log.WithField("issue", issueNumber).Info("Created issue.") */

		/*slug := regexp.MustCompile(`\W+`).ReplaceAllString(strings.ToLower(taskName), "-")
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
		} */

		return nil
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().StringP(ArgGitTitle, "n", "", "Issue title.")
	cmd.Flags().StringP(ArgGitBody, "m", "", "Issue body.")
	cmd.Flags().String(ArgGitTaskParentOrg, "naveegoinc", "Issue org.")
	cmd.Flags().String(ArgGitTaskParentRepo, "stories", "Issue repo.")
	cmd.Flags().Int(ArgGitTaskStory, 0, "Number of the story to use as a parent.")
})

const (
	ArgGitTitle          = "title"
	ArgGitBody           = "body"
	ArgGitTaskStory      = "story"
	ArgGitTaskParentOrg  = "parent-org"
	ArgGitTaskParentRepo = "parent-repo"
)

func getZenhubToken() (string, error) {
	b := mustGetBosun()
	ws := b.GetWorkspace()
	ctx := b.NewContext().WithDir(ws.Path)
	if ws.ZenhubToken == nil {
		fmt.Println("Zenhub token was not found. Please generate a new Zenhub token. https://app.zenhub.com/dashboard/tokens")
		fmt.Println(`Simple example: echo "9uha09h39oenhsir98snegcu"`)
		fmt.Println(`Better example: cat $HOME/.tokens/zenhub.token"`)
		fmt.Println(`Secure example: lpass show "Tokens/GithubCLIForBosun" --notes"`)
		script := pkg.RequestStringFromUser("Command")

		ws.ZenhubToken = &bosun.CommandValue{
			Command: bosun.Command{
				Script: script,
			},
		}

		_, err := ws.ZenhubToken.Resolve(ctx)
		if err != nil {
			return "", errors.Errorf("script failed: %s\nscript:\n%s", err, script)
		}

		err = b.Save()
		if err != nil {
			return "", errors.Errorf("save failed: %s", err)
		}
	}

	token, err := ws.ZenhubToken.Resolve(ctx)
	if err != nil {
		return "", err
	}

	err = os.Setenv("ZENHUB_TOKEN", token)
	if err != nil {
		return "", err
	}

	token, ok := os.LookupEnv("ZENHUB_TOKEN")
	if !ok {
		return "", errors.Errorf("ZENHUB_TOKEN must be set")
	}

	return token, nil
}