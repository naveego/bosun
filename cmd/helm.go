// Copyright © 2018 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"github.com/prometheus/common/log"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/naveego/bosun/internal"
	"github.com/spf13/cobra"
)

// helmCmd contains wrappers for helm commands
var helmCmd = &cobra.Command{
	Use:   "helm",
	Short: "Wrappers for custom helm commands.",
	Long:  `If there's a sequence of helm commands that you use a lot, add them as a command under this one.`,
}

var helmPublishForce bool

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

		if helmPublishForce{
			log.Warn("Force publishing all matched charts.")
		}

		plugins, err := internal.NewCommand("helm", "plugin", "list").RunOut()
		check(err, plugins)
		if !strings.Contains(plugins, "s3") {
			internal.NewCommand("helm plugin install https://github.com/hypnoglow/helm-s3.git").MustRun()
		}
		repos, err := internal.NewCommand("helm repo list").RunOut()
		check(err, repos)
		if !strings.Contains(repos, "s3://helm.n5o.black") {
			internal.NewCommand("helm repo add helm.n5o.black s3://helm.n5o.black").MustRun()
		}

		if len(args) == 0 {
			args = []string{"./"}
		}

		var charts []string

		for _, path := range args {
			expanded, err := filepath.Glob(path)
			if err != nil {
				return err
			}
			charts = append(charts, expanded...)
		}

		sort.Strings(charts)

		for _, chart := range charts {

			stat, err := os.Stat(chart)
			if !stat.IsDir() {
				continue
			}
			if strings.HasPrefix(filepath.Base(chart), ".") {
				continue
			}

			chartName := filepath.Base(chart)
			log := internal.Log.WithField("path", chart).WithField("@chart", chartName)

			chartText, err := new(internal.Command).WithExe("helm").WithArgs("inspect", "chart", chart).RunOut()
			if err != nil {
				log.WithError(err).Error("Could not inspect chart")
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

			repoContent, err := new(internal.Command).WithExe("helm").WithEnvValue("AWS_DEFAULT_PROFILE", "black").WithArgs("search", qualifiedName, "--versions").RunOut()
			if err != nil {
				log.WithError(err).Error("Could not search repo")
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

			out, err := internal.NewCommand("helm", "package", chart).RunOut()
			if err != nil {
				log.WithError(err).Error("could not create package")
				continue
			}

			check(err, out)
			f := strings.Fields(out)
			packagePath := f[len(f)-1]

			helmArgs := []string{"s3", "push", packagePath, "helm.n5o.black"}
			if helmPublishForce {
				helmArgs = append(helmArgs, "--force")
			}

			err = internal.NewCommand("helm", helmArgs...).WithEnvValue("AWS_DEFAULT_PROFILE", "black").RunE()

			if err != nil {
				log.WithError(err).Error("could not publish chart")
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
