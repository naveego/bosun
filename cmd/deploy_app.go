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
	"github.com/naveego/bosun/pkg/values"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sort"
)

func init() {

}

var _ = addCommand(deployCmd, &cobra.Command{
	Use:          "app [apps...]",
	Aliases:      []string{"apps"},
	Short:        "Deploys one or more apps in one step (combines plan and execute).",
	SilenceUsage: true,
	RunE:         deployApp,
}, deployAppFlags)

const (
	argDeployAppClusters = "clusters"
	argDeployAppRecycle  = "recycle"
	argDeployAppTag      = "tag"
	argDeployAppDiffOnly = "diff-only"
)

func deployAppFlags(cmd *cobra.Command) {
	cmd.Flags().StringSlice(argDeployPlanProviderPriority, []string{bosun.WorkspaceProviderName, bosun.SlotUnstable, bosun.SlotStable}, "Providers in priority order to use to deploy apps (current, stable, unstable, or workspace).")
	cmd.Flags().StringSlice(argDeployPlanApps, []string{}, "Apps to include.")
	cmd.Flags().Bool(argDeployPlanAll, false, "Deploy all apps.")
	cmd.Flags().String(argDeployAppTag, "", "Tag to use when deploying the app or apps.")
	cmd.Flags().Bool(argDeployPlanIgnoreDeps, true, "Don't validate dependencies.")
	cmd.Flags().Bool(argDeployPlanAutoDeps, false, "Automatically include dependencies.")
	cmd.Flags().StringSlice(argDeployAppClusters, []string{}, "Whitelist of specific clusters to deploy to.")
	cmd.Flags().Bool(argAppDeployPreview, false, "Just dump the values which would be used to deploy, then exit.")
	cmd.Flags().Bool(ArgAppLatest, false, "Force bosun to pull the latest of the app and deploy that.")
	cmd.Flags().Bool(argDeployAppRecycle, false, "Recycle the app after deploying it.")
	cmd.Flags().Bool(argDeployAppDiffOnly, false, "Display the impact of running the deploy but don't actually run it.")
	cmd.Flags().StringSliceP(ArgAppValueSet, "v", []string{}, "Additional value sets to include in this deploy.")
	cmd.Flags().StringSliceP(ArgAppSet, "s", []string{}, "Value overrides to set in this deploy, as key=value pairs.")
}

func deployApp(cmd *cobra.Command, args []string) error {
	b := MustGetBosun()
	p, err := b.GetCurrentPlatform()
	if err != nil {
		return err
	}
	ctx := b.NewContext()

	apps := args
	if len(apps) == 0 {
		for _, a := range p.GetApps(ctx) {
			apps = append(apps, a.Name)
		}
		sort.Strings(apps)
		if !viper.GetBool(argDeployPlanAll) {
			apps = userChooseApps("Choose apps to deploy", apps)
		}
	}

	valueSets, err := getValueSetSlice(b, b.GetCurrentEnvironment())
	if err != nil {
		return err
	}

	if viper.GetBool(ArgAppLatest) {
		workspaceApps, getAppsErr := getKnownApps(b, apps)
		if getAppsErr != nil {
			return getAppsErr
		}
		getAppsErr = pullApps(ctx, workspaceApps, true)
		valueSets = append(valueSets, values.ValueSet{Static: map[string]interface{}{"tag": "latest"}})
	}

	err = deployApps(b, p, apps, valueSets, args)

	return err
}

// deployApps deploys the provided app names from the specified platform with the provided value sets
func deployApps(b *bosun.Bosun, p *bosun.Platform, appNames []string, valueSets values.ValueSets, forceAppNames []string) error {
	var req = bosun.CreateDeploymentPlanRequest{
		Apps:                  appNames,
		ProviderPriority:      viper.GetStringSlice(argDeployPlanProviderPriority),
		IgnoreDependencies:    viper.GetBool(argDeployPlanIgnoreDeps),
		AutomaticDependencies: viper.GetBool(argDeployPlanAutoDeps),
	}

	planCreator := bosun.NewDeploymentPlanCreator(b, p)

	plan, err := planCreator.CreateDeploymentPlan(req)

	if err != nil {
		return err
	}

	tag := viper.GetString(argDeployAppTag)
	if tag != "" {
		for _, app := range plan.Apps {
			app.Tag = tag
		}
	}

	executeRequest := bosun.ExecuteDeploymentPlanRequest{
		Plan:        plan,
		IncludeApps: forceAppNames,
		ValueSets:   valueSets,
		Recycle:     viper.GetBool(argDeployAppRecycle),
		PreviewOnly: viper.GetBool(argAppDeployPreview),
		DiffOnly:    viper.GetBool(argDeployAppDiffOnly),
	}

	clustersWhitelist := viper.GetStringSlice(argDeployAppClusters)
	if len(clustersWhitelist) > 0 {
		executeRequest.Clusters = map[string]bool{}
		for _, cluster := range clustersWhitelist {
			executeRequest.Clusters[cluster] = true
		}
	}

	executor := bosun.NewDeploymentPlanExecutor(b, p)

	_, err = executor.Execute(executeRequest)

	return err
}
