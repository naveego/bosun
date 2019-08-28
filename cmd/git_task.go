package cmd

import (
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	// "os"
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
		org, repo := git.GetCurrentOrgAndRepo()
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
		repoPath, err := git.GetCurrentRepoPath()
		if err != nil {
			return err
		}
		b := MustGetBosun(bosun.Parameters{ProviderPriority: []string{bosun.WorkspaceProviderName}})

		app, err := getCurrentApp(b)
		if err != nil {
			return err
		}

		svc, err := b.GetIssueService(repoPath)
		if err != nil {
			return errors.New("get issue service")
		}

		var parent *issues.IssueRef

		storyNumber := viper.GetInt(ArgGitTaskStory)
		if storyNumber > 0 {

			parentOrg := viper.GetString(ArgGitTaskParentOrg)
			parentRepo := viper.GetString(ArgGitTaskParentRepo)

			tmp := issues.NewIssueRef(parentOrg, parentRepo, storyNumber)
			parent = &tmp

		}

		issue := issues.Issue{
			Title:         title,
			Body:          body,
			Org:           org,
			Repo:          repo,
			IsClosed:      false,
			BranchPattern: app.Branching.Feature,
		}

		_, err = svc.Create(issue, parent)
		if err != nil {
			return err
		}

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

var gitTaskShow = addCommand(gitTaskCmd, &cobra.Command{
	Use:   "issue [app]",
	Short: "Shows the issue for the current branch, if any.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun(bosun.Parameters{ProviderPriority: []string{bosun.WorkspaceProviderName}})

		app := mustGetApp(b, args)

		localRepo := app.Repo.LocalRepo

		branch := app.GetBranchName()

		if !app.Branching.IsFeature(branch) {
			return errors.Errorf("%q is not a feature branch (template is %q)", branch, app.Branching.Feature)
		}

		issueNumber, err := app.Branching.GetIssueNumber(branch)
		if err != nil {
			return err
		}

		svc, err := b.GetIssueService(localRepo.Path)
		if err != nil {
			return err
		}

		org, repo := git.GetOrgAndRepoFromPath(localRepo.Path)

		ref := issues.NewIssueRef(org, repo, issueNumber)

		issue, err := svc.GetIssue(ref)
		if err != nil {
			return err
		}

		return renderOutput(issue)
	},
})
