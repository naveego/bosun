package cmd

import (
	"github.com/cheynewallace/tabby"
	"github.com/kyokomi/emoji"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
)

var appListCmd = addCommand(appCmd, &cobra.Command{
	Use:          "list",
	Aliases:      []string{"ls"},
	Short:        "Lists the static config of all known apps.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		viper.SetDefault(ArgFilteringAll, true)

		b := mustGetBosun()

		apps := getFilterParams(b, args).GetApps()

		wd, _ := os.Getwd()

		ctx := b.NewContext()

		t := tabby.New()
		t.AddHeader("APP", "CLONED", "VERSION", "REPO", "PATH", "BRANCH")
		for _, app := range apps {
			var isCloned, repo, path, branch, version string
			repo = app.RepoName

			if app.IsRepoCloned() {
				isCloned = emoji.Sprint(":heavy_check_mark:")
				if app.BranchForRelease {
					branch = app.GetBranchName().String()
				} else {
					branch = ""
				}
				version = app.Version.String()
			} else {
				isCloned = emoji.Sprint("    :x:")
				branch = ""
				version = app.Version.String()
			}

			if app.IsFromManifest {
				manifest, _ := app.GetManifest(ctx)
				path, _ = filepath.Rel(wd, manifest.AppConfig.FromPath)
			} else {
				path, _ = filepath.Rel(wd, app.AppConfig.FromPath)
			}
			t.AddLine(app.Name, isCloned, version, repo, path, branch)
		}

		t.Print()

		return nil
	},
})

var appListActionsCmd = addCommand(appListCmd, &cobra.Command{
	Use:          "actions [app]",
	Aliases:      []string{"action"},
	Short:        "Lists the actions for an app. If no app is provided, lists all actions.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		viper.SetDefault(ArgFilteringAll, true)

		b := mustGetBosun()
		apps := getFilterParams(b, args).GetApps()

		t := tabby.New()
		t.AddHeader("APP", "ACTION", "WHEN", "WHERE", "DESCRIPTION")
		for _, app := range apps {

			for _, action := range app.Actions {
				t.AddLine(app.Name, action.Name, action.When, action.Where, action.Description)
			}
		}

		t.Print()

		return nil
	},
})
