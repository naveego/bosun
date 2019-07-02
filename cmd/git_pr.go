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
		issueSvc, err := b.GetIssueService(repoPath)
		if err != nil {
			return errors.New("get issue service")
		}

		//taskName := args[0]

		org0, repo0 := git.GetCurrentOrgAndRepo()

		prCmd := GitPullRequestCommand{
			LocalRepoPath: repoPath,
			Reviewers:     viper.GetStringSlice(ArgPullRequestReviewers),
			Base:          viper.GetString(ArgPullRequestBase),
			FromBranch:    g.Branch(),
			Body:          viper.GetString(ArgPullRequestBody),
		}

		var issueNmb int
		issueNmb, _, err = prCmd.Execute()
		if err != nil {
			return errors.Wrap(err, "execute pr")
		}

		issueRf := issues.NewIssueRef(org0, repo0, issueNmb)

		// TODO: move parent story if possible
		// parents, err := issueSvc.GetParents(issueRf)
		// if err != nil {
		// 	return errors.Wrap(err, "get parents for current issue")
		// }
		//pkg.Log.WithField("parents", parents).Info("parents returned")

		// var parent issues.IssueRef
		// if len(parents) > 0 {
		// 	parentOrg := parents[0].Org
		// 	parentRepo := parents[0].Repo
		// 	parentNumber := parents[0].Number
		// 	parent = issues.NewIssueRef(parentOrg, parentRepo, parentNumber)
		// 	pkg.Log.WithField("parent ref", parent)
		// 	columnUAT := issues.ColumnWaitingForUAT
		// 	err = issueSvc.SetProgress(parent, columnUAT)
		// 	if err != nil {
		// 		return errors.Wrap(err, "move parent story to UAT")
		// 	}
		// }

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
	cmd.Flags().String(ArgPullRequestBase, "master", "Target branch for merge.")
})
