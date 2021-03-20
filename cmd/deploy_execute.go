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
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"path/filepath"
)

func init() {

}

var _ = addCommand(deployCmd, &cobra.Command{
	Use:          "execute {path | {release|stable|unstable}} [apps...]",
	Args:         cobra.MinimumNArgs(1),
	Short:        "Executes a deployment against the current environment.",
	Long:         "If apps are provided, only those apps will be deployed.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()

		check(b.ConfirmEnvironment())

		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}
		req := bosun.ExecuteDeploymentPlanRequest{
			Validate:       !viper.GetBool(argDeployExecuteSkipValidate),
			DiffOnly:       viper.GetBool(argDeployExecuteDiffOnly),
			DumpValuesOnly: viper.GetBool(argDeployExecuteValuesOnly),
			UseSudo:        viper.GetBool(ArgGlobalSudo),
			Clusters:       map[string]bool{},
		}

		pathOrSlot := args[0]
		switch pathOrSlot{
		case "release","current", bosun.SlotStable, bosun.SlotUnstable:
			r, folder, resolveReleaseErr := getReleaseAndPlanFolderName(b, pathOrSlot)
			if resolveReleaseErr != nil {
				return resolveReleaseErr
			}
			expectedReleaseHash, hashErr := r.GetChangeDetectionHash()

			if hashErr != nil {
				return hashErr
			}

			req.Path = filepath.Join(p.GetDeploymentsDir(), fmt.Sprintf("%s/plan.yaml", folder))
			req.Plan, resolveReleaseErr = bosun.LoadDeploymentPlanFromFile(req.Path)
			if resolveReleaseErr != nil {
				return resolveReleaseErr
			}

			if req.Plan.BasedOnHash != "" && req.Plan.BasedOnHash != expectedReleaseHash {
				confirmed := cli.RequestConfirmFromUser("The release has changed since this plan was created, are you sure you want to continue?")
				if !confirmed {

					color.Yellow("You may want to run `bosun deploy plan release` to update the deployment plan\n")
					return nil
				}
			}
			break
		default:
			req.Path = pathOrSlot
		}

		if len(args) > 1 {
			req.IncludeApps = args[1:]
		}

		clusters := viper.GetStringSlice(argDeployExecuteClusters)
		if len(clusters) > 0 {
			for _, cluster := range clusters {
				req.Clusters[cluster] = true
			}
		} else {
			for _, cluster := range b.GetCurrentEnvironment().Clusters {
				if cluster.Protected {
					req.Clusters[cluster.Name] = cli.RequestConfirmFromUser("This deployment will affect protected cluster %s, are you sure you want to do this?", cluster.Name)
				} else {
					req.Clusters[cluster.Name] = true
				}
			}
		}

		var atLeastOneCluster = false
		for _, enabled := range req.Clusters {
			if enabled {
				atLeastOneCluster = true
				break
			}
		}
		if !atLeastOneCluster {
			return errors.Errorf("no clusters confirmed")
		}

		executor := bosun.NewDeploymentPlanExecutor(b, p)

		_, err = executor.Execute(req)

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(argDeployExecuteSkipValidate, false, "Skip validation")
	cmd.Flags().Bool(argDeployExecuteDiffOnly, false, "Display the diffs for the deploy, but do not actually execute.")
	cmd.Flags().Bool(argDeployExecuteValuesOnly, false, "Display the values which would be used for the deploy, but do not actually execute.")
	cmd.Flags().StringSlice(argDeployExecuteClusters, []string{}, "Clusters to deploy to, defaults to all.")
})

const (
	argDeployExecuteSkipValidate = "skip-validation"
	argDeployExecuteDiffOnly     = "diff-only"
	argDeployExecuteValuesOnly   = "values-only"
	argDeployExecuteClusters     = "clusters"
)
