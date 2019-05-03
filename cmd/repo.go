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
	"github.com/manifoldco/promptui"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/util"
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
})

var repoListCmd = addCommand(repoCmd, &cobra.Command{
	Use:          "list",
	Aliases:      []string{"ls"},
	Short:        "Lists the known repos and their clone status.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := mustGetBosun()
		repos := b.GetRepos()

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
})

var repoPathCmd = addCommand(repoCmd, &cobra.Command{
	Use:   "path [name]",
	Args:  cobra.RangeArgs(0, 1),
	Short: "Outputs the path where the repo is cloned on the local system.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()
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

var repoCloneCmd = addCommand(
	repoCmd,
	&cobra.Command{
		Use:   "clone [name]",
		Short: "Clones the named repo.",
		Long:  "Uses the first directory in `gitRoots` from the root config.",
		RunE: func(cmd *cobra.Command, args []string) error {
			viper.BindPFlags(cmd.Flags())
			b := mustGetBosun()

			dir := viper.GetString(ArgAppCloneDir)
			roots := b.GetGitRoots()
			var err error
			if dir == "" {
				if len(roots) == 0 {
					p := promptui.Prompt{
						Label: "Provide git root (apps will be cloned to ./org/repo in the dir you specify)",
					}
					dir, err = p.Run()
					if err != nil {
						return err
					}
				} else {
					dir = roots[0]
				}
			}
			rootExists := false
			for _, root := range roots {
				if root == dir {
					rootExists = true
					break
				}
			}
			if !rootExists {
				b.AddGitRoot(dir)
				err := b.Save()
				if err != nil {
					return err
				}
				b = mustGetBosun()
			}

			repos := b.GetRepos()
			filters := getFilters(args)

			repos = bosun.ApplyFilter(repos, filters).([]*bosun.Repo)

			ctx := b.NewContext()
			for _, repo := range repos {
				log := ctx.Log.WithField("repo", repo.Name)

				if repo.IsRepoCloned() {
					pkg.Log.Infof("Repo already cloned to %q", repo.LocalRepo.Path)
					continue
				}
				log.Info("Cloning...")

				err = repo.CloneRepo(ctx, dir)
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
	})
