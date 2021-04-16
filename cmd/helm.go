package cmd

import (
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/prometheus/common/log"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"github.com/spf13/cobra"
)

// helmCmd contains wrappers for helm commands
var helmCmd = &cobra.Command{
	Use:   "helm",
	Short: "Wrappers for custom helm commands.",
	Long:  `If there's a sequence of helm commands that you use a lot, add them as a command under this one.`,
}

var helmPublishForce bool

var _ = addCommand(helmCmd, &cobra.Command{
	Use:   "init",
	Short: "Initializes helm/tiller.",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := command.NewShellExe("helm", "init").RunE()
		return err
	},
})

var helmPublishCmd = &cobra.Command{
	Use:   "publish [chart-paths...]",
	Short: "Publishes one or more charts to our helm repo.",
	Long: `The [chart-path] parameter defaults to ./ 

If chart-path ends in a * (like devops/charts/*) this command will publish
each chart in the folder, and will not error if the chart has already been published.

If multiple charts paths are provided, each will be published and it will not 
error if the chart has already been published. 
`,

	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		if helmPublishForce {
			log.Warn("Force publishing all matched charts.")
		}

		plugins, err := command.NewShellExe("helm", "plugin", "list").RunOut()
		check(err, plugins)
		if !strings.Contains(plugins, "s3") {
			command.NewShellExe("helm plugin install https://github.com/hypnoglow/helm-s3.git").MustRun()
		}
		repos, err := command.NewShellExe("helm repo list").RunOut()
		check(err, repos)
		if !strings.Contains(repos, "s3://helm.n5o.black") {
			command.NewShellExe("helm repo add helm.n5o.black s3://helm.n5o.black").WithEnvValue("AWS_DEFAULT_REGION", "us-east-1").MustRun()
		}

		if len(args) == 0 {
			args = []string{"./"}
		}

		var charts []string

		for _, path := range args {
			expanded, globErr := filepath.Glob(path)
			if globErr != nil {
				return globErr
			}
			charts = append(charts, expanded...)
		}

		sort.Strings(charts)

		for _, chart := range charts {

			stat, statErr := os.Stat(chart)
			if err != nil {
				return err
			}
			if !stat.IsDir() {
				continue
			}
			if strings.HasPrefix(filepath.Base(chart), ".") {
				continue
			}

			chartName := filepath.Base(chart)
			log := core.Log.WithField("path", chart).WithField("@chart", chartName)

			chartText, statErr := new(command.ShellExe).WithExe("helm").WithArgs("inspect", "chart", chart).RunOut()
			if statErr != nil {
				log.WithError(statErr).Error("Could not inspect chart")
				continue
			}
			thisVersionMatch := versionExtractor.FindStringSubmatch(chartText)
			if len(thisVersionMatch) != 2 {
				log.Error("Chart did not have version.")
				continue
			}
			thisVersion := thisVersionMatch[1]

			log = log.WithField("@version", thisVersion)
			qualifiedName := "helm.n5o.black/" + chartName

			repoContent, statErr := new(command.ShellExe).WithExe("helm").WithEnvValue("AWS_DEFAULT_PROFILE", "black").WithArgs("search", qualifiedName, "--versions").RunOut()
			if statErr != nil {
				log.WithError(statErr).Error("Could not search repo")
				continue
			}

			searchLines := strings.Split(repoContent, "\n")
			versionExists := false
			for _, line := range searchLines {
				f := strings.Fields(line)
				if len(f) > 2 && f[1] == thisVersion {
					versionExists = true
				}
			}

			if versionExists && !helmPublishForce {
				log.Warn("version already exists (use --force to overwrite)")
				continue
			}

			out, statErr := command.NewShellExe("helm", "package", chart).RunOut()
			if statErr != nil {
				log.WithError(statErr).Error("could not create package")
				continue
			}

			check(statErr, out)
			f := strings.Fields(out)
			packagePath := f[len(f)-1]

			helmArgs := []string{"s3", "push", packagePath, "helm.n5o.black"}
			if helmPublishForce {
				helmArgs = append(helmArgs, "--force")
			}

			statErr = command.NewShellExe("helm", helmArgs...).WithEnvValue("AWS_DEFAULT_PROFILE", "black").RunE()

			if statErr != nil {
				log.WithError(statErr).Error("could not publish chart")
			} else {
				log.Info("publish complete")
			}

			os.Remove(packagePath)
		}

		return nil
	},
}

var versionExtractor = regexp.MustCompile("version: (.*)")

var (
	ArgPublishChartForce = "force"
)

func init() {
	helmPublishCmd.Flags().BoolVarP(&helmPublishForce, ArgPublishChartForce, "f", false, "Force helm to publish the chart even if the version already exists.")

	helmCmd.AddCommand(helmPublishCmd)

	rootCmd.AddCommand(helmCmd)
}
