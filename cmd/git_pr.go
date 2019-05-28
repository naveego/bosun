package cmd

import (
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/zenhub"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"strings"
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

		/*var parent *issues.IssueRef
		storyNumber := viper.GetInt(ArgGitTaskStory)
		var pOrg, pRepo string
		if storyNumber > 0 {

			parentOrg := viper.GetString(ArgGitTaskParentOrg)
			pOrg = parentOrg
			parentRepo := viper.GetString(ArgGitTaskParentRepo)
			pRepo = parentRepo

			tmp := issues.NewIssueRef(parentOrg, parentRepo, storyNumber)
			parent = &tmp

		} */


		//taskName := args[0]

		/*title := viper.GetString(ArgGitTitle)
		if title == "" {
			if len(taskName) > 50 {
				title = taskName[:50] + "..."
			} else {
				title = taskName
			}
		} */
		//body := viper.GetString(ArgPullRequestBody)

		_, repo0 := git.GetCurrentOrgAndRepo()
		repoSplitted := strings.FieldsFunc(repo0, zenhub.Split)
		org := repoSplitted[0]
		repo := repoSplitted[1]

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

		issueRf := issues.NewIssueRef(org, repo, issueNmb)
		//zenhub.log.WithField("title", issue.Title).Info("Setting assignee")
		parents, err := svc.GetParents(issueRf)
		if err != nil {
			return errors.Wrap(err, "get parents for current issue")
		}
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
		err = svc.SetProgress(parent, column)
		if err != nil {
			return errors.Wrap(err, "move parent story to Waiting for Merge")
		}

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().StringSlice(ArgPullRequestReviewers, []string{}, "Reviewers to request.")
	cmd.Flags().String(ArgPullRequestTitle, "", "Title of PR")
	cmd.Flags().String(ArgPullRequestBody, "", "Body of PR")
	cmd.Flags().String(ArgPullRequestBase, "master", "Target branch for merge.")
})

