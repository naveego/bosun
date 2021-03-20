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
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {

}

var stackCmd = addCommand(rootCmd, &cobra.Command{
	Use:   "stack",
	Short: "Contains commands for managing a stack of apps.",
})

var stackShowCmd = addCommand(stackCmd, &cobra.Command{
	Use:          "show",
	Aliases:      []string{"ls", "list", "view"},
	Short:        "Shows the current stack for the current cluster or subcluster.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b, p := MustGetPlatform()
		env := b.GetCurrentEnvironment()
		cluster := env.GetCluster()

		stack, err := cluster.GetStackState()

		if err != nil {
			return err
		}

		knownApps := p.GetKnownAppMap()

		skippedGlobalApps := 0
		skippedEnvApps := 0

		for _, knownApp := range knownApps {

			_, deployed := stack.Apps[knownApp.Name]
			if deployed {
				continue
			}

			stackApp := kube.StackApp{
				Name:    knownApp.Name,
				Details: "Not deployed;",
			}

			disabledForEnv := env.IsAppDisabled(knownApp.Name)

			if disabledForEnv && !viper.GetBool(argStackIncludeAllApps) {
				skippedGlobalApps++
				continue
			}

			disabledForCluster := env.Cluster.IsAppDisabled(knownApp.Name)

			if disabledForCluster {
				if !viper.GetBool(argStackIncludeEnvApps) && !viper.GetBool(argStackIncludeAllApps) {
					skippedEnvApps++
					continue
				}

				stackApp.Details += " Disabled for cluster"
			}

			stack.Apps[knownApp.Name] = stackApp

		}

		ctx := b.NewContext()
		if skippedGlobalApps > 0 {
			ctx.Log().Infof("Omitted %d apps which are disabled for this environment, use --%s flag to show them", skippedGlobalApps, argStackIncludeAllApps)
		}
		if skippedEnvApps > 0 {
			ctx.Log().Infof("Omitted %d apps which are disabled for this cluster, use --%s flag to show them", skippedEnvApps, argStackIncludeEnvApps)
		}

		return printOutputWithDefaultFormat("table", stack)
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(argStackIncludeEnvApps, false, "Include apps enabled for this environment but disabled for this cluster.")
	cmd.Flags().Bool(argStackIncludeAllApps, false, "Show all apps, even if not assigned to environment or cluster.")
})

const (
	argStackIncludeAllApps = "include-all"
	argStackIncludeEnvApps = "include-env"
)

var stackResetCmd = addCommand(stackCmd, &cobra.Command{
	Use:   "reset {stable|unstable}",
	Args:  cobra.ExactArgs(1),
	Short: "Resets the stack to use the specified release for all apps.",
	Long: "This will attempt to deploy all apps for the current cluster using the requested slot. " +
		"If apps are not found in the requested slot it will fail back to the less stable providers.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b, p := MustGetPlatform()
		env := b.GetCurrentEnvironment()

		var appNames []string

		for appName := range p.GetKnownAppMap() {
			if !env.IsAppDisabled(appName) && !env.Cluster.IsAppDisabled(appName) {
				appNames = append(appNames, appName)
			}
		}

		var providers []string
		switch args[0] {
		case bosun.SlotStable:
			providers = []string{bosun.SlotStable, bosun.SlotUnstable, bosun.WorkspaceProviderName}
		case bosun.SlotUnstable:
			providers = []string{bosun.SlotUnstable, bosun.WorkspaceProviderName}

		default:
			return errors.Errorf("invalid release slot %q", args[0])
		}

		var req = bosun.CreateDeploymentPlanRequest{
			Apps:                  appNames,
			ProviderPriority:      providers,
			IgnoreDependencies:    true,
			AutomaticDependencies: false,
		}

		planCreator := bosun.NewDeploymentPlanCreator(b, p)

		plan, err := planCreator.CreateDeploymentPlan(req)

		if err != nil {
			return err
		}

		executeRequest := bosun.ExecuteDeploymentPlanRequest{
			Plan:     plan,
			Clusters: map[string]bool{},
		}

		executor := bosun.NewDeploymentPlanExecutor(b, p)

		_, err = executor.Execute(executeRequest)

		return nil
	},
})
