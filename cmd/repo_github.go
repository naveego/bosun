package cmd

import (
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/spf13/cobra"
)

var _ = addCommand(
	repoCmd,
	&cobra.Command{
		Use:           "configure-github [repo] [repo...]",
		Short:         "Pulls the repo(s).",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {

			client := mustGetGithubClient()

			return forEachRepo(args, func(ctx bosun.BosunContext, repo *bosun.Repo) error {

				log := ctx.WithLogField("repo", repo.Name).Log()
				ref, err := repo.GetRef()
				if err != nil {
					return err
				}
				g, err := repo.LocalRepo.Git()
				if err != nil {
					return err
				}

				err = g.Fetch()
				if err != nil {
					return err
				}

				log.Infof("Setting default branch to %q...", repo.Branching.Develop)

				_, _, err = client.Repositories.Edit(ctx.Ctx(), ref.Org, ref.Repo, &github.Repository{
					DefaultBranch: &repo.Branching.Develop,
				})
				if err != nil {
					return err
				}

				for _, branch := range []string{repo.Branching.Develop, repo.Branching.Master} {
					log.Infof("Adding branch protection for %q...", branch)
					_, _, err = client.Repositories.UpdateBranchProtection(ctx.Ctx(), ref.Org, ref.Repo, branch, &github.ProtectionRequest{
						RequiredPullRequestReviews: &github.PullRequestReviewsEnforcementRequest{
							DismissStaleReviews:          true,
							RequiredApprovingReviewCount: 1,
						},
						Restrictions: &github.BranchRestrictionsRequest{
							Users: []string{"chriscerk", "roehlerw"},
							Teams: []string{},
						},
					})
					if err != nil {
						return err
					}
				}
				return err
			})
		},
	})
