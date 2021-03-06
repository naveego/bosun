package cmd

import (
	"fmt"
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

const (
	AppListColName    = "app"
	AppListColCloned  = "cloned"
	AppListColVersion = "version"
	AppListColRepo    = "repo"
	AppListColBranch  = "branch"
	AppListColDirty   = "dirty"
	AppListColStale   = "stale"
	AppListColPath    = "path"
	AppListColImages    = "images"
	AppListColLabels    = "meta-labels"
)

var appListAlwaysColumns = []string{
	AppListColName,
	AppListColRepo,
	AppListColCloned,
	AppListColVersion,
	AppListColBranch,
}

var appListOptionalColumns = []string{
	AppListColDirty,
	AppListColStale,
	AppListColPath,
	AppListColLabels,
	AppListColImages,
}

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

			row[AppListColName] = app.Name
			row[AppListColRepo] = app.RepoName


			if app.IsRepoCloned() {
				row[AppListColCloned] = fmtBool(true)
				row[AppListColBranch] = app.GetBranchName().String()
				row[AppListColVersion] = app.Version.String()
				if colsMap[AppListColDirty] {
					row[AppListColDirty] = fmtBool(app.Repo.LocalRepo.IsDirty())
				}
				if colsMap[AppListColStale] {
					row[AppListColStale] = app.Repo.LocalRepo.GetUpstreamStatus()
				}
			} else {
				row[AppListColCloned] = fmtBool(false)
				row[AppListColVersion] = app.Version.String()
			}

			if app.IsFromManifest {
				manifest, _ := app.GetManifest(ctx)
				row[AppListColPath], _ = filepath.Rel(wd, manifest.AppConfig.FromPath)
			} else {
				row[AppListColPath], _ = filepath.Rel(wd, app.AppConfig.FromPath)
			}

			if colsMap[AppListColLabels] {
				var labelLines []string
				for _, k := range util.SortedKeys(app.Labels) {
					labelLines = append(labelLines, fmt.Sprintf("%s: %s", k, app.Labels[k]))
				}
				row[AppListColLabels] = strings.Join(labelLines, "\n")
			}
			if colsMap[AppListColImages] {
				var imageLines []string
				images := app.GetImages()
				for _, image := range images {
					imageLines = append(imageLines, image.GetFullName())
				}
				row[AppListColImages] = strings.Join(imageLines, "\n")
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
