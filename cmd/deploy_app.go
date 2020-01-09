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
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sort"
)

func init() {

}

var _ = addCommand(deployCmd, &cobra.Command{
	Use:          "app {app}",
	Args:         cobra.ExactArgs(1),
	Short:        "Deploys a single app in one step (combines plan and execute).",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}
		ctx := b.NewContext()
		log := ctx.Log()

		provider, _ := cmd.Flags().GetString(argDeployPlanProvider)
		if provider == "" {
			provider = userChooseProvider(provider)
		}
		log.Debugf("Obtaining apps from provider %q", provider)

		apps := args
		if len(apps) == 0 {
			for _, a := range p.Apps {
				apps = append(apps, a.Name)
			}
			sort.Strings(apps)
			apps = userChooseApps("Choose apps to deploy", apps)
		}

		var req = bosun.CreateDeploymentPlanRequest{
			Apps:                  apps,
			ProviderName:          provider,
			IgnoreDependencies:    viper.GetBool(argDeployPlanIgnoreDeps),
			AutomaticDependencies: viper.GetBool(argDeployPlanAutoDeps),
		}

		planCreator := bosun.NewDeploymentPlanCreator(b, p)

		plan, err := planCreator.CreateDeploymentPlan(req)

		if err != nil {
			return err
		}

		executeRequest := bosun.ExecuteDeploymentPlanRequest{
			Plan: plan,
		}

		executor := bosun.NewDeploymentPlanExecutor(b, p)

		err = executor.Execute(executeRequest)

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().String(argDeployPlanProvider, "workspace", "Provider to use to deploy apps (current, stable, unstable, or workspace).")
	cmd.Flags().StringSlice(argDeployPlanApps, []string{}, "Apps to include.")
	cmd.Flags().Bool(argDeployPlanIgnoreDeps, true, "Don't validate dependencies.")
	cmd.Flags().Bool(argDeployPlanAutoDeps, false, "Automatically include dependencies.")
})
