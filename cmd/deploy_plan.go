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
	"path/filepath"
	"sort"
)

func init() {

}

var _ = addCommand(deployCmd, &cobra.Command{
	Use:          "plan",
	Args:         cobra.ExactArgs(0),
	Short:        "Plans a deployment.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}
		ctx := b.NewContext()
		log := ctx.Log()

		path, _ := cmd.Flags().GetString(argDeployPlanPath)
		if path == "" {
			path = userChooseStringWithDefault("Where should the plan be saved?", filepath.Join(filepath.Dir(p.FromPath), "/deployments/default/plan.yaml"))
		}
		log.Debugf("Saving plan to %q", path)
		provider, _ := cmd.Flags().GetString(argDeployPlanProvider)
		if provider == "" {
			provider = userChooseProvider(provider)
		}
		log.Debugf("Obtaining apps from provider %q", provider)

		apps := viper.GetStringSlice(argDeployPlanApps)
		if len(apps) == 0 {
			for _, a := range p.Apps {
				apps = append(apps, a.Name)
			}
			sort.Strings(apps)
			apps = userChooseApps("Choose apps to deploy", apps)
		}

		var req = bosun.CreateDeploymentPlanRequest{
			Apps:                  apps,
			Path:                  path,
			ProviderName:          provider,
			IgnoreDependencies:    viper.GetBool(argDeployPlanIgnoreDeps),
			AutomaticDependencies: viper.GetBool(argDeployPlanAutoDeps),
		}

		planCreator := bosun.NewDeploymentPlanCreator(b, p)

		plan, err := planCreator.CreateDeploymentPlan(req)

		if err != nil {
			return err
		}

		err = plan.Save()

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().String(argDeployPlanPath, "", "Path where plan should be stored.")
	cmd.Flags().String(argDeployPlanProvider, "", "Provider to use to deploy apps (current, stable, unstable, or workspace).")
	cmd.Flags().StringSlice(argDeployPlanApps, []string{}, "Apps to include.")
	cmd.Flags().Bool(argDeployPlanIgnoreDeps, false, "Don't validate dependencies.")
	cmd.Flags().Bool(argDeployPlanAutoDeps, false, "Automatically include dependencies.")
})

const (
	argDeployPlanPath       = "path"
	argDeployPlanApps       = "apps"
	argDeployPlanProvider   = "provider"
	argDeployPlanIgnoreDeps = "ignore-deps"
	argDeployPlanAutoDeps   = "auto-deps"
)
