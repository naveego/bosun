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
	"github.com/AlecAivazis/survey/v2"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"path/filepath"
	"sort"
)

func init() {

}

var platformDeployCmd = addCommand(platformCmd, &cobra.Command{
	Use:   "deploy",
	Short: "Contains commands for planning and executing a deploy.",
})

var _ = addCommand(platformDeployCmd, &cobra.Command{
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

		path, _ := cmd.Flags().GetString(argPlatformDeployPlanPath)
		if path == "" {
			path = userChooseStringWithDefault("Where should the plan be saved?", filepath.Join(filepath.Dir(p.FromPath), "/deployments/default/plan.yaml"))
		}
		log.Debugf("Saving plan to %q", path)
		provider, _ := cmd.Flags().GetString(argPlatformDeployPlanProvider)
		if provider == "" {
			provider = userChooseProvider(provider)
		}
		log.Debugf("Obtaining apps from provider %q", provider)

		apps := viper.GetStringSlice(argPlatformDeployPlanApps)
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
			IgnoreDependencies:    viper.GetBool(argPlatformDeployPlanIgnoreDeps),
			AutomaticDependencies: viper.GetBool(argPlatformDeployPlanAutoDeps),
		}

		err = p.CreateDeploymentPlan(b.NewContext(), req)

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().String(argPlatformDeployPlanPath, "", "Path where plan should be stored.")
	cmd.Flags().String(argPlatformDeployPlanProvider, "", "Provider to use to deploy apps (current, stable, unstable, or workspace).")
	cmd.Flags().StringSlice(argPlatformDeployPlanApps, []string{}, "Apps to include.")
	cmd.Flags().Bool(argPlatformDeployPlanIgnoreDeps, false, "Don't validate dependencies.")
	cmd.Flags().Bool(argPlatformDeployPlanAutoDeps, false, "Automatically include dependencies.")
})

const (
	argPlatformDeployPlanPath       = "path"
	argPlatformDeployPlanApps       = "apps"
	argPlatformDeployPlanProvider   = "provider"
	argPlatformDeployPlanIgnoreDeps = "ignore-deps"
	argPlatformDeployPlanAutoDeps   = "auto-deps"
)

func userChooseApps(message string, apps []string) []string {
	const invertKey = "Invert (select apps to exclude)"
	options := append([]string{invertKey}, apps...)
	var selected []string
	prompt := &survey.MultiSelect{
		Message: message,
		Options: options,
	}

	check(survey.AskOne(prompt, &selected))

	if len(selected) == 0 {
		return []string{}
	}

	selections := map[string]bool{}
	selectedMeansInclude := true
	for _, s := range selected {
		if s == invertKey {
			selectedMeansInclude = false
			continue
		}
		selections[s] = true
	}
	var out []string
	for _, o := range apps {
		if selectedMeansInclude && selections[o] {
			out = append(out, o)
		} else if !selectedMeansInclude && !selections[o] {
			out = append(out, o)
		}
	}

	return out
}

func userChooseStringWithDefault(message string, value string) string {
	prompt := &survey.Input{
		Message: message,
		Default: value,
	}
	check(survey.AskOne(prompt, &value))
	return value
}

func userChooseProvider(provider string) string {
	if provider != "" {
		return provider
	}
	prompt := &survey.Select{
		Message: "Choose a provider",
		Options: []string{bosun.SlotStable, bosun.SlotUnstable, bosun.SlotCurrent, bosun.WorkspaceProviderName},
	}
	check(survey.AskOne(prompt, &provider))
	return provider
}
