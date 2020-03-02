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
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"path/filepath"
	"sort"
	"strings"
)

func init() {

}

var deployPlanCmd = addCommand(deployCmd, &cobra.Command{
	Use:          "plan [release]",
	Short:        "Plans a deployment, optionally of an existing release.",
	Args:cobra.RangeArgs(0, 1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		if len(args) > 0 && args[0] == "release" {
			return releaseDeployPlan()
		}

		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}
		ctx := b.NewContext()
		log := ctx.Log()

		path, _ := cmd.Flags().GetString(argDeployPlanPath)
		if path == "" {
			path = userChooseStringWithDefault("Where should the plan be saved?", filepath.Join(p.GetDeploymentsDir(), "default/plan.yaml"))
		}
		log.Debugf("Saving plan to %q", path)

		var req = bosun.CreateDeploymentPlanRequest{
			Path:                  path,
			IgnoreDependencies:    viper.GetBool(argDeployPlanIgnoreDeps),
			AutomaticDependencies: viper.GetBool(argDeployPlanAutoDeps),
		}
		update := viper.GetBool(argDeployPlanUpdate)
		var previousPlan *bosun.DeploymentPlan
		if update {
			previousPlan, err = bosun.LoadDeploymentPlanFromFile(path)
			if err != nil {
				return errors.Wrap(err, "could not load existing plan")
			}
			req.ProviderPriority = previousPlan.ProviderPriority
			for _, app := range previousPlan.Apps {
				req.Apps = append(req.Apps, app.Name)
			}
		} else {
			provider := viper.GetString(argDeployPlanProviderPriority)
			if provider == "" {
				provider = userChooseProvider(provider)
			}
			req.ProviderPriority = []string{provider}

			log.Debugf("Obtaining apps from provider %q", req.ProviderPriority)

			req.Apps = viper.GetStringSlice(argDeployPlanApps)
			if len(req.Apps) == 0 {
				for _, a := range p.GetApps(ctx) {
					req.Apps = append(req.Apps, a.Name)
				}
				sort.Strings(req.Apps)
				if !viper.GetBool(argDeployPlanAll) {
					req.Apps = userChooseApps("Choose apps to deploy", req.Apps)
				}
			}
		}

		planCreator := bosun.NewDeploymentPlanCreator(b, p)

		plan, err := planCreator.CreateDeploymentPlan(req)

		if err != nil {
			return err
		}

		if update && previousPlan != nil {
			plan.EnvironmentDeployProgress = previousPlan.EnvironmentDeployProgress
		}

		err = plan.Save()

		if err != nil {
			return err
		}

		color.Blue("Saved deployment plan to %s", req.Path)

		color.White("To run this plan again, use this:")
		cli := fmt.Sprintf("bosun deploy plan --path %s --provider %s ", req.Path, req.ProviderPriority)
		if req.IgnoreDependencies {
			cli += "--ignore-deps "
		}
		if req.AutomaticDependencies {
			cli += "--auto-deps "
		}
		if viper.GetBool(argDeployPlanAll) {
			cli += "--all "
		} else {
			cli += "--apps "
			cli += strings.Join(req.Apps, ",")
		}
		color.White(cli)

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().String(argDeployPlanPath, "", "Dir where plan should be stored.")
	cmd.Flags().String(argDeployPlanProviderPriority, "", "Provider to use to deploy apps (current, stable, unstable, or workspace).")
	cmd.Flags().StringSlice(argDeployPlanApps, []string{}, "Apps to include.")
	cmd.Flags().Bool(argDeployPlanAll, false, "Deploy all apps which target the current environment.")
	cmd.Flags().Bool(argDeployPlanIgnoreDeps, false, "Don't validate dependencies.")
	cmd.Flags().Bool(argDeployPlanAutoDeps, false, "Automatically include dependencies.")
	cmd.Flags().Bool(argDeployPlanUpdate, false, "Update an existing plan rather than creating a new one.")
})

const (
	argDeployPlanPath             = "path"
	argDeployPlanApps             = "apps"
	argDeployPlanAll              = "all"
	argDeployPlanProviderPriority = "provider"
	argDeployPlanIgnoreDeps       = "ignore-deps"
	argDeployPlanAutoDeps         = "auto-deps"
	argDeployPlanUpdate           = "update"
)

var deployReleasePlanCmd = addCommand(deployPlanCmd, &cobra.Command{
	Use:          "plan",
	Short:        "Plan the deployment of the current release",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		return releaseDeployPlan()
	},
})

func releaseDeployPlan() error {

	b := MustGetBosun()
	p, err := b.GetCurrentPlatform()
	if err != nil {
		return err
	}
	r, err := p.GetCurrentRelease()
	if err != nil {
		return err
	}

	deploymentPlanPath := filepath.Join(p.GetDeploymentsDir(), fmt.Sprintf("%s/plan.yaml", r.Version.String()))

	previousPlan, _ := bosun.LoadDeploymentPlanFromFile(deploymentPlanPath)

	var req = bosun.CreateDeploymentPlanRequest{
		Path:                  deploymentPlanPath,
		ProviderPriority:      []string{r.Slot},
		IgnoreDependencies:    true,
		AutomaticDependencies: false,
		ReleaseVersion: &r.Version,
	}
	for name, included := range r.UpgradedApps {
		if included {
			req.Apps = append(req.Apps, name)
		}
	}

	planCreator := bosun.NewDeploymentPlanCreator(b, p)

	plan, err := planCreator.CreateDeploymentPlan(req)

	if err != nil {
		return err
	}

	if previousPlan != nil {
		plan.EnvironmentDeployProgress = previousPlan.EnvironmentDeployProgress
	}

	err = plan.Save()
	if err != nil {
		return err
	}

	fmt.Println(deploymentPlanPath)

	return nil
}

