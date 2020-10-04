package cmd

import (
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	// "github.com/naveego/bosun/pkg/zenhub"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	// "log"
)

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

		b := MustGetBosun()
		issueSvc, err := b.GetIssueService()
		if err != nil {
			return errors.New("get issue service")
		}

		app, err := getCurrentApp(b)
		if err != nil {
			return errors.Wrap(err, "could not find app in current directory")
		}

		//taskName := args[0]

		org0, repo0 := git.GetCurrentOrgAndRepo().OrgAndRepo()

		prCmd := GitPullRequestCommand{
			LocalRepoPath: repoPath,
			Reviewers:     viper.GetStringSlice(ArgPullRequestReviewers),
			Base:          viper.GetString(ArgPullRequestBase),
			FromBranch:    g.Branch(),
			Body:          viper.GetString(ArgPullRequestBody),
		}

		if prCmd.Base == "" {
			prCmd.Base = app.Branching.Develop
		}

		var issueNmb int
		issueNmb, _, err = prCmd.Execute()
		if err != nil {
			return errors.Wrap(err, "execute pr")
		}

		issueRf := issues.NewIssueRef(org0, repo0, issueNmb)

		column := issues.ColumnWaitingForMerge
		err = issueSvc.SetProgress(issueRf, column)
		if err != nil {
			return errors.Wrap(err, "move issue to Ready for Merge")
		}

		return err

	},
}, func(cmd *cobra.Command) {
	cmd.Flags().StringSlice(ArgPullRequestReviewers, []string{}, "Reviewers to request.")
	cmd.Flags().String(ArgPullRequestTitle, "", "Title of PR")
	cmd.Flags().String(ArgPullRequestBody, "", "Body of PR")
	cmd.Flags().String(ArgPullRequestBase, "", "Target branch for merge.")
})
