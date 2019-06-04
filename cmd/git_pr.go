package cmd

import (
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/zenhub"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"log"
)

func mustGetIssueService() issues.IssueService {
	svc, err := getIssueService()
	if err != nil {
		log.Fatal(err)
	}
	return svc
}

func getIssueService() (issues.IssueService, error) {


	b := mustGetBosun()

	p, err := b.GetCurrentPlatform()
	if err != nil {
		return nil, errors.Wrap(err, "get current platform")
	}
	zc := *p.ZenHubConfig


	zc.GithubToken, err = getGithubToken()
	if err != nil {
		return nil, err
	}

	zc.ZenhubToken, err = getZenhubToken()
	if err != nil {
		return nil,errors.Wrap(err, "get zenhub token")
	}

	repoPath, err := git.GetCurrentRepoPath()
	if err != nil {
		return nil, err
	}

	g, err := git.NewGitWrapper(repoPath)
	if err != nil {
		return nil,err
	}

	svc, err := zenhub.NewIssueService(zc, g, pkg.Log.WithField("cmp", "zenhub"))
	if err != nil {
		return nil,errors.Wrapf(err, "get story service with tokens %q, %q", zc.GithubToken, zc.ZenhubToken)
	}
	return svc, nil

}

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

		svc := mustGetIssueService()

		//taskName := args[0]

		org0 := "naveegoinc"
		repo0 := "stories"

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
				i := 0
				ok := true
				for i < len(children) {
					if !children[i].IsClosed {
						ok = false
					}
				}
				if ok {
					err = svc.SetProgress(parent, column)
					if err != nil {
						return errors.Wrap(err, "move parent story to Waiting for merge after checking children")
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




