package cmd

import (
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var releaseCommitCmd = addCommand(releaseCmd, &cobra.Command{
	Use:           "commit",
	Short:         "Commands for merging a release branch back to develop and master.",
	SilenceErrors: true,
	SilenceUsage:  true,
})

var releaseCommitPlanCmd = addCommand(releaseCommitCmd, &cobra.Command{
	Use:           "plan [apps...]",
	Short:         "Plans the commit of the release branch back to master for each app in the release, and the platform repository.",
	Long: "If no apps are provided, all apps are planned",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b := MustGetBosun()

		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		committer, err := bosun.NewReleaseCommitter(p, b)
		if err != nil {
			return err
		}

		err = committer.Plan(bosun.PlanReleaseCommitRequest{Apps: args})

		if err != nil {
			return err
		}

		plan, err := committer.GetPlan()

		if err != nil {
			return err
		}

		return printOutput(plan)
	},
})

var releaseCommitShowCmd = addCommand(releaseCommitCmd, &cobra.Command{
	Use:           "show",
	Short:         "Shows the plan the commit of the release branch back to master for each app in the release, and the platform repository.",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b := MustGetBosun()

		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		committer, err := bosun.NewReleaseCommitter(p, b)
		if err != nil {
			return err
		}

		plan, err := committer.GetPlan()

		if err != nil {
			return err
		}

		return printOutput(plan)
	},
})

var releaseCommitExecuteCmd = addCommand(releaseCommitCmd, &cobra.Command{
	Use:           "execute",
	Short:         "Merges the release branch back to master for each app in the release, and the platform repository.",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := MustGetBosun()

		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		committer, err := bosun.NewReleaseCommitter(p, b)
		if err != nil {
			return err
		}


		err = committer.Execute()

		return err
	},
})