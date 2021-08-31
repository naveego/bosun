package cmd

import (
	"fmt"
	"github.com/cheynewallace/tabby"
	"github.com/fatih/color"
	"github.com/kyokomi/emoji"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/util"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
	"strings"
)

var appListAlwaysColumns = []string{
	bosun.AppListColName,
	bosun.AppListColRepo,
	bosun.AppListColCloned,
	bosun.AppListColVersion,
	bosun.AppListColBranch,
}

var appListOptionalColumns = []string{
	bosun.AppListColDirty,
	bosun.AppListColStale,
	bosun.AppListColPath,
	bosun.AppListColLabels,
	bosun.AppListColImages,
}

var appListCmd = addCommand(appCmd, &cobra.Command{
	Use:          "list",
	Aliases:      []string{"ls"},
	Short:        "Lists the static config of all known apps.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		viper.SetDefault(ArgFilteringAll, true)

		b := MustGetBosunNoEnvironment()

		apps := getFilterParams(b, args).GetApps()


		wd, _ := os.Getwd()

		ctx := b.NewContext()

		cols := appListAlwaysColumns[0:]
		colsMap := map[string]bool{}
		for _, col := range appListOptionalColumns {
			if viper.GetBool(col) {
				cols = append(cols, col)
			}
		}
		for _, col := range cols {
			colsMap[col] = true
		}

		var out []map[string]string

		for _, app := range apps {
			row := map[string]string{}

			row[bosun.AppListColName] = app.Name
			row[bosun.AppListColRepo] = app.RepoName

			if app.IsRepoCloned() {
				row[bosun.AppListColCloned] = fmtBool(true)
				row[bosun.AppListColBranch] = app.GetBranchName().String()
				row[bosun.AppListColVersion] = app.Version.String()
				if colsMap[bosun.AppListColDirty] {
					row[bosun.AppListColDirty] = fmtBool(app.Repo.LocalRepo.IsDirty())
				}
				if colsMap[bosun.AppListColStale] {
					row[bosun.AppListColStale] = app.Repo.LocalRepo.GetUpstreamStatus()
				}
			} else {
				row[bosun.AppListColCloned] = fmtBool(false)
				row[bosun.AppListColVersion] = app.Version.String()
			}

			if app.IsFromManifest {
				manifest, _ := app.GetManifest(ctx)
				row[bosun.AppListColPath], _ = filepath.Rel(wd, manifest.AppConfig.FromPath)
			} else {
				row[bosun.AppListColPath], _ = filepath.Rel(wd, app.AppConfig.FromPath)
			}

			if colsMap[bosun.AppListColLabels] {
				var labelLines []string
				for _, k := range util.SortedKeys(app.Labels) {
					labelLines = append(labelLines, fmt.Sprintf("%s: %s", k, app.Labels[k]))
				}
				row[bosun.AppListColLabels] = strings.Join(labelLines, "\n")
			}
			if colsMap[bosun.AppListColImages] {
				var imageLines []string
				images := app.GetImages()
				for _, image := range images {
					imageLines = append(imageLines, image.GetFullName())
				}
				row[bosun.AppListColImages] = strings.Join(imageLines, "\n")
			}
			out = append(out, row)
		}

		return renderOutput(out, cols...)
	},
}, func(cmd *cobra.Command) {

	for _, col := range appListOptionalColumns {
		cmd.Flags().Bool(col, false, fmt.Sprintf("Include %q column", col))
	}
})

func fmtBool(b bool) string {
	if b {
		return emoji.Sprint("YES    ")
	} else {
		return emoji.Sprint("     NO")
	}
}

var appListReposCmd = addCommand(appCmd, &cobra.Command{
	Use:          "list-repos",
	Aliases:      []string{"lsr", "ls-repo"},
	Short:        "Lists the repos and their current state.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b := MustGetBosun()

		r, err := b.GetRepoInfo()
		if err != nil {
			return err
		}

		return printOutput(r, "app", "name", "branch", "isDirty", "path")
	},
})

var appListSrcCmd = addCommand(appCmd, &cobra.Command{
	Use:          "list-versions",
	Aliases:      []string{"lsv", "ls-versions", "ls-p"},
	Short:        "Lists all apps from all providers.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()

		apps, err := b.GetAllVersionsOfAllApps()
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
				t.AddLine(app.Name, action.Name, action.When, action.WhereFilter, action.Description)
			}
		}

		t.Print()

		return nil
	},
})
