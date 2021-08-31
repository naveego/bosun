package cmd

import (
	"fmt"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	argReleaseCreateBase = "base"
	argReleaseCreateVersion = "version"
)

// releaseCmd represents the release command
var releaseCreateCmd = addCommand(releaseCmd, &cobra.Command{
	Use:     "create {release version}",
	Short:   "Create a release.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		b, p := getReleaseCmdDeps()

		version, err := semver.Parse(args[0])

		if err != nil {
			return err
		}

		req := bosun.ReleaseCreateSettings{
			Version: version,
		}

		var baseVersion semver.Version
		baseVersionFlag := viper.GetString(argReleaseCreateBase)
		if baseVersionFlag != "" {

			baseVersion, err = semver.Parse(baseVersionFlag)
			if err != nil {
				return err
			}

			req.Base = &baseVersion
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
	cmd.Flags().String(argReleaseCreateVersion, "", "The version for the new release.")
})
