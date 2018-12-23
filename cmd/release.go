package cmd

import (
	"github.com/cheynewallace/tabby"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {

	releaseCmd.AddCommand(releaseListCmd)

	releaseCmd.AddCommand(releaseCreateCmd)

	releaseAddCmd.Flags().BoolP(ArgAppAll, "a", false, "Apply to all known microservices.")
	releaseAddCmd.Flags().StringSliceP(ArgAppLabels, "i", []string{}, "Apply to microservices with the provided labels.")


	releaseCmd.AddCommand(releaseAddCmd)

	rootCmd.AddCommand(releaseCmd)
}

// releaseCmd represents the release command
var releaseCmd = &cobra.Command{
	Use:   "release",
	Aliases:[]string{"rel", "r"},
	Short: "Release commands.",
}


var releaseListCmd = &cobra.Command{
	Use:   "list",
	Aliases:[]string{"ls"},
	Short: "Lists known releases.",
	Run: func(cmd *cobra.Command, args []string) {
		b := mustGetBosun()
		t := tabby.New()
		t.AddHeader("RELEASE", "PATH")
		releases := b.GetReleases()
		for _, release := range releases {
			t.AddLine(release.Name, release.FromPath)
		}

		t.Print()
	},
}

var releaseCreateCmd = &cobra.Command{
	Use:   "create {name} {path}",
	Args:cobra.ExactArgs(2),
	Short: "Creates a new release.",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, path := args[0], args[1]
		c := bosun.ConfigFragment{
			FromPath:path,
			Releases: []*bosun.Release{
				&bosun.Release{
					Name:name,
				},
			},
		}

		err := c.Save()
		return err
	},
}

var releaseAddCmd = &cobra.Command{
	Use:   "add {release-name} [names...]",
	Args: cobra.MinimumNArgs(1),
	Short: "Adds one or more apps to a release.",
	Long:"Provide app names or use labels.",
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		b := mustGetBosun()
		release, err := b.GetRelease(args[0])
		if err != nil {
			return err
		}
		apps,err:= getApps(b, args[1:])
		if err != nil {
			return err
		}

		ctx := b.NewContext("")

		for _, app := range apps {

			_, ok := release.Apps[app.Name]
			if ok {
				pkg.Log.Warnf("Overwriting existing app %q.", app.Name)
			} else {
				ctx.Log.Infof("Adding app %q", app.Name)
			}

			release.Apps[app.Name], err = app.MakeAppRelease()
			if err != nil {
				return errors.Errorf("could not make release for app %q", app.Name)
			}
		}

		err = release.Fragment.Save()
		return err
	},
}
