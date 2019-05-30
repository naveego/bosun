package cmd

import (
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/zenhub"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var gitPullRequestCmd = addCommand(gitCmd, &cobra.Command{
	Use:     "pull-request",
	Aliases: []string{"pr"},
	Short:   "Opens a pull request.",
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		githubToken, err := getGithubToken()
		if err != nil {
			return err
		}

		zenhubToken, err := getZenhubToken()
		if err != nil {
			return errors.Wrap(err, "get zenhub token")
		}

		repoPath, err := git.GetCurrentRepoPath()
		if err != nil {
			return err
		}

		g, err := git.NewGitWrapper(repoPath)
		if err != nil {
			return err
		}

		svc, err := zenhub.NewIssueService(githubToken, zenhubToken, g, pkg.Log.WithField("cmp", "zenhub"))
		if err != nil {
			return errors.Wrapf(err, "get story service with tokens %q, %q", githubToken, zenhubToken)
		}

		//taskName := args[0]

		org0 := "naveegoinc"
		repo0 := "stories"
		//git.GetCurrentOrgAndRepo()
		//dumpJSON("org0", org0)
		//dumpJSON("repo0", repo0)
		pkg.Log.WithField("org", org0).Info("org from GetCurrentOrgAndRepo")
		pkg.Log.WithField("repo", repo0).Info("repo from...")

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
		pkg.Log.WithField("issueRf", issueRf).Info("show issueRf")

		parents, err := svc.GetParents(issueRf)
		if err != nil {
			return errors.Wrap(err, "get parents for current issue")
		}
		//pkg.Log.WithField("parents", parents).Info("parents returned")

		var parent issues.IssueRef
		if len(parents) > 0 {
			parentOrg := parents[0].Org
			parentRepo := parents[0].Repo
			parentNumber := parents[0].Number
			parent = issues.NewIssueRef(parentOrg, parentRepo, parentNumber)
		}


		column := "Waiting for Merge"
		err = svc.SetProgress(issueRf, column)
		if err != nil {
			return errors.Wrap(err, "move issue to Waiting for Merge")
		}

		if len(parent) > 0 {
			children, err := svc.GetChildren(parent)

			if children == nil {
				err = svc.SetProgress(parent, column)
				if err != nil {
					return errors.Wrap(err, "move parent story to Waiting for Merge when no child")
				}
			} else {
				ok, err := svc.ChildrenAllClosed(children)
				if err != nil {
					return errors.Wrap(err, "check if children are all closed")
				}
				if ok {
					err = svc.SetProgress(parent, column)
					if err != nil {
						return errors.Wrap(err, "move parent story to Waiting for Merge when all children closed")
					}
				}
			}
		}



		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().StringSlice(ArgPullRequestReviewers, []string{}, "Reviewers to request.")
	cmd.Flags().String(ArgPullRequestTitle, "", "Title of PR")
	cmd.Flags().String(ArgPullRequestBody, "", "Body of PR")
	cmd.Flags().String(ArgPullRequestBase, "master", "Target branch for merge.")
})




