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
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/util/multierr"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"sort"
	"strings"
)

var repoCmd = addCommand(rootCmd, &cobra.Command{
	Use:   "repo",
	Short: "Contains sub-commands for interacting with repos. Has some overlap with the git sub-command.",
	Long: `Most repo sub-commands take one or more optional name parameters. 
If no name parameters are provided, the command will attempt to find a repo which
contains the current working path.`,
	Args: cobra.NoArgs,
})

var _ = addCommand(repoCmd, &cobra.Command{
	Use:          "list",
	Aliases:      []string{"ls"},
	Short:        "Lists the known repos and their clone status.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		repos := getFilterParams(b, []string{}).Chain().Then().Including(filter.FilterMatchAll()).From(b.GetRepos()).([]*bosun.Repo)

		t := tablewriter.NewWriter(os.Stdout)

		t.SetHeader([]string{"Name", "Cloned", "Local Path", "Labels", "Apps"})
		t.SetReflowDuringAutoWrap(false)
		t.SetAutoWrapText(false)

		for _, repo := range repos {

			var name, cloned, path, labels, apps string

			name = repo.Name
			if repo.LocalRepo != nil {
				cloned = "YES"
				path = repo.LocalRepo.Path
			}
			var appNames []string
			for _, app := range repo.Apps {
				appNames = append(appNames, app.Name)
			}
			appNames = util.DistinctStrings(appNames)
			sort.Strings(appNames)
			apps = strings.Join(appNames, "\n")

			var labelKeys []string
			for label := range repo.FilteringLabels {
				labelKeys = append(labelKeys, label)
			}
			sort.Strings(labelKeys)
			var labelsKVs []string
			for _, label := range labelKeys {
				if label != "" {
					labelsKVs = append(labelsKVs, fmt.Sprintf("%s:%s", label, repo.FilteringLabels[label]))
				}
			}
			labels = strings.Join(labelsKVs, "\n")

			t.Append([]string{name, cloned, path, labels, apps})
		}

		t.Render()
		return nil
	},
}, withFilteringFlags)

var _ = addCommand(repoCmd, &cobra.Command{
	Use:   "path {name}",
	Args:  cobra.ExactArgs(1),
	Short: "Outputs the path where the repo is cloned on the local system.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		repo, err := b.GetRepo(args[0])
		if err != nil {
			return err
		}
		if repo.LocalRepo == nil {
			return errors.New("repo is not cloned")
		}

		fmt.Println(repo.LocalRepo.Path)
		return nil
	},
})

var _ = addCommand(
	repoCmd,
	&cobra.Command{
		Use:   "clone {name} [name...]",
		Args:  cobra.MinimumNArgs(1),
		Short: "Clones the named repo(s).",
		Long:  "Uses the first directory in `gitRoots` from the root config.",
		RunE: func(cmd *cobra.Command, args []string) error {
			b := MustGetBosun()

			dir, err := getOrAddGitRoot(b, viper.GetString(ArgAppCloneDir))
			if err != nil {
				return err
			}

			repos, err := getFilterParams(b, args).Chain().ToGetAtLeast(1).FromErr(b.GetRepos())
			if err != nil {
				return err
			}

			ctx := b.NewContext()
			for _, repo := range repos.([]*bosun.Repo) {
				log := ctx.Log().WithField("repo", repo.Name)

				if repo.CheckCloned() == nil {
					pkg.Log.Infof("Repo already cloned to %q", repo.LocalRepo.Path)
					continue
				}
				log.Info("Cloning...")

				err = repo.Clone(ctx, dir)
				if err != nil {
					log.WithError(err).Error("Error cloning.")
				} else {
					log.Info("Cloned.")
				}
			}

			err = b.Save()

			return err
		},
	},
	func(cmd *cobra.Command) {
		cmd.Flags().String(ArgAppCloneDir, "", "The directory to clone into. (The repo will be cloned into `org/repo` in this directory.) ")
	},
	withFilteringFlags,
)

var _ = addCommand(
	repoCmd,
	&cobra.Command{
		Use:           "pull [repo] [repo...]",
		Short:         "Pulls the repo(s).",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			rebase := viper.GetBool("rebase")
			return forEachRepo(args, func(ctx bosun.BosunContext, repo *bosun.Repo) error {
				ctx.Log().Info("Fetching...")
				err := repo.Pull(ctx, rebase)
				return err
			})
		},
	}, withFilteringFlags, func(cmd *cobra.Command) {
		cmd.Flags().Bool("rebase", false, "Rebase rather than merge.")
	})

var _ = addCommand(
	repoCmd,
	&cobra.Command{
		Use:           "fetch [repo] [repo...]",
		Short:         "Fetches the repo(s).",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return forEachRepo(args, func(ctx bosun.BosunContext, repo *bosun.Repo) error {
				ctx.Log().Info("Pulling...")
				err := repo.Fetch()
				return err
			})
		},
	}, withFilteringFlags)

func forEachRepo(args []string, fn func(ctx bosun.BosunContext, repo *bosun.Repo) error) error {
	b := MustGetBosun()
	ctx := b.NewContext()
	repos, err := getFilterParams(b, args).Chain().ToGetAtLeast(1).FromErr(b.GetRepos())
	if err != nil {
		return err
	}

	errs := multierr.New()
	for _, repo := range repos.([]*bosun.Repo) {
		ctx.Log().Infof("Processing %q...", repo.Name)
		err = fn(ctx, repo)
		if err != nil {
			errs.Collect(err)
			ctx.Log().WithError(err).Errorf("Error on repo %q", repo.Name)
		} else {
			ctx.Log().Infof("Completed %q.", repo.Name)
		}
	}
	return errs.ToError()
}
