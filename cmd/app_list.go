package cmd

import (
	"github.com/cheynewallace/tabby"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var appListCmd = addCommand(appCmd, &cobra.Command{
	Use:          "list",
	Aliases:      []string{"ls"},
	Short:        "Lists the static config of all known apps.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		viper.SetDefault(ArgAppAll, true)

		b := mustGetBosun()
		apps, err := getAppRepos(b, args)
		if err != nil {
			return err
		}

		t := tabby.New()
		t.AddHeader("APP", "CLONED", "VERSION", "PATH or REPO", "BRANCH", "IMPORTED BY")
		for _, app := range apps {
			var check, pathrepo, branch, version, importedBy string

			if app.IsRepoCloned() {
				check = "✔"
				pathrepo = app.FromPath
				branch = app.GetBranch()
				version = app.Version
			} else {
				check = ""
				pathrepo = app.Repo
				branch = ""
				version = app.Version
				importedBy = app.FromPath
			}

			t.AddLine(app.Name, check, version, pathrepo, branch, importedBy)
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
		viper.SetDefault(ArgAppAll, true)

		b := mustGetBosun()
		apps, err := getAppReposOpt(b, args, getAppReposOptions{ifNoFiltersGetCurrent: true})
		if err != nil {
			return err
		}

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
