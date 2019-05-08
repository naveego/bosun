package cmd

import (
	"github.com/cheynewallace/tabby"
	"github.com/kyokomi/emoji"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"strings"
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
		apps, err := getApps(b, args)
		if err != nil {
			return err
		}

		gitRoots := b.GetGitRoots()
		var trimGitRoot = func(p string) string {
			for _, gitRoot := range gitRoots {
				p = strings.Replace(p, gitRoot, "$GITROOT", -1)
			}
			return p
		}

		t := tabby.New()
		t.AddHeader("APP", "CLONED", "VERSION", "PATH or REPO", "BRANCH", "IMPORTED BY")
		for _, app := range apps {
			var isCloned, pathrepo, branch, version, importedBy string

			if app.IsRepoCloned() {
				isCloned = emoji.Sprint(":heavy_check_mark:")
				pathrepo = trimGitRoot(app.FromPath)
				if app.BranchForRelease {
					branch = app.GetBranchName().String()
				} else {
					branch = ""
				}
				version = app.Version
			} else {
				isCloned = emoji.Sprint("    :x:")
				pathrepo = app.RepoName
				branch = ""
				version = app.Version
				importedBy = trimGitRoot(app.FromPath)
			}

			t.AddLine(app.Name, isCloned, version, pathrepo, branch, importedBy)
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
