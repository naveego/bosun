package cmd

import (
	"github.com/naveego/bosun/pkg/semver"
	"github.com/spf13/cobra"
)

var releaseHotfixCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "hotfix {version}",
	Args:  cobra.ExactArgs(1),
	Short: "Creates a hotfix release which is based on the current release. Add apps to the hotfix using bosun release add",
	RunE: func(cmd *cobra.Command, args []string) error {
		b, p := getReleaseCmdDeps()

		version, err := semver.Parse(args[0])

		ctx := b.NewContext()
		_, err = p.CreateHotfix(ctx, version)
		if err != nil {
			return err
		}

		err = p.Save(ctx)

		return err
	},
})
