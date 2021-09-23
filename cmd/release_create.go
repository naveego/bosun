package cmd

import (
	"fmt"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	argReleaseCreateBase = "base"
	argReleaseBump       = "bump"
)

// releaseCmd represents the release command
var releaseCreateCmd = addCommand(releaseCmd, &cobra.Command{
	Use:     "create",
	Short:   "Create a release.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b, p := getReleaseCmdDeps()


		req := bosun.ReleaseCreateSettings{
		}

		var err error
		var baseVersion semver.Version
		baseVersionFlag := viper.GetString(argReleaseCreateBase)
		if baseVersionFlag != "" {
			baseVersion, err = semver.Parse(baseVersionFlag)
			if err != nil {
				return err
			}

			req.Base = &baseVersion
		} else {
			currentRelease, releaseErr := p.GetStableRelease()
			if releaseErr != nil {
				return releaseErr
			}
			req.Base = &currentRelease.Version
		}

		bump := viper.GetString(argReleaseBump)

		if bump == "" {
			_, v := cli.RequestBump("What bump should be applied to the release version?", *req.Base)
			req.Version = *v
		} else {
			req.Version, err = req.Base.Bump(bump)
		}

		r, err := p.CreateRelease(b.NewContext(), req)

		if err != nil {
			return err
		}

		err = p.Save(b.NewContext())
		if err != nil {
			return err
		}

		fmt.Printf("New release created. You are on the release branch %s\n", r.Branch)

		return nil
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().String(argReleaseCreateBase, "", "The release to base the release on. Defaults to the current release.")
	cmd.Flags().String(argReleaseBump, "", "The version for the new release.")
})
