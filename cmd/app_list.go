package cmd

import (
	"github.com/cheynewallace/tabby"
	"github.com/fatih/color"
	"github.com/kyokomi/emoji"
	"github.com/naveego/bosun/pkg/util"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
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

		b := MustGetBosun()

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

var appListSrcCmd = addCommand(appCmd, &cobra.Command{
	Use:          "list-versions",
	Aliases:      []string{"lsv", "ls-versions", "ls-p"},
	Short:        "Lists all apps from all providers.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()

		apps, err := b.GetAllProvidedApps()
		if err != nil {
			return err
		}

		t := tablewriter.NewWriter(os.Stdout)
		t.SetRowLine(true)
		t.SetHeader([]string{"APPS", "PROVIDER", "VERSION"})

		grouped := apps.GroupByAppThenProvider()
		providerNames := b.GetAllProviderNames()
		for _, appName := range util.SortedKeys(grouped) {
			byProvider := grouped[appName]

			var providers []string
			var versions []string
			versionsChanged := false
			previousVersion := ""
			for _, provider := range providerNames {
				app, ok := byProvider[provider]
				if !ok {
					continue
				}
				providers = append(providers, provider)
				currentVersion := app.Version.String()
				versions = append(versions, currentVersion)
				if previousVersion != "" && previousVersion != currentVersion {
					versionsChanged = true
				}
				previousVersion = currentVersion
			}
			providerSummary := strings.Join(providers, "\n")
			versionSummary := strings.Join(versions, "\n")
			if versionsChanged {
				appName = color.YellowString("*%s", appName)
			}
			t.Append([]string{appName, providerSummary, versionSummary})

		}

		t.Render()

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

		b := MustGetBosun()
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
