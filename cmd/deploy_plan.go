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
	"sort"
	"strings"
)

func init() {

}

var deployPlanCmd = addCommand(deployCmd, &cobra.Command{
	Use:          "plan [release|stable|unstable]",
	Short:        "Plans a deployment, optionally of an existing release.",
	Args:         cobra.RangeArgs(0, 1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		if len(args) > 0 {
			switch args[0] {
			case "release", "current", "stable", "unstable":
				return releaseDeployPlan(args[0])
			}
		}

		b := MustGetBosun(cli.Parameters{NoEnvironment: true})
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
		replace := viper.GetBool(argDeployPlanReplace)
		var previousPlan *bosun.DeploymentPlan

		if !replace {

			previousPlan, err = bosun.LoadDeploymentPlanFromFile(path)
			if err == nil {

				req.ProviderPriority = previousPlan.ProviderPriority
				for _, app := range previousPlan.Apps {
					req.Apps = append(req.Apps, app.Name)
				}
			}
		}

		if len(req.Apps) == 0 {

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

		if previousPlan != nil {
			plan.AppDeploymentProgress = previousPlan.AppDeploymentProgress
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
}, applyDeployPlanFlags)

func applyDeployPlanFlags(cmd *cobra.Command) {
	cmd.Flags().String(argDeployPlanPath, "", "Dir where plan should be stored.")
	cmd.Flags().String(argDeployPlanProviderPriority, "", "Provider to use to deploy apps (current, stable, unstable, or workspace).")
	cmd.Flags().StringSlice(argDeployPlanApps, []string{}, "Apps to include.")
	cmd.Flags().Bool(argDeployPlanAll, false, "Deploy all apps which target the current environment.")
	cmd.Flags().Bool(argDeployPlanIgnoreDeps, false, "Don't validate dependencies.")
	cmd.Flags().Bool(argDeployPlanAutoDeps, false, "Automatically include dependencies.")
	cmd.Flags().Bool(argDeployPlanReplace, false, "Replace an existing plan rather than updating it if it already exists.")
}

const (
	argDeployPlanPath             = "path"
	argDeployPlanApps             = "apps"
	argDeployPlanAll              = "all"
	argDeployPlanProviderPriority = "providers"
	argDeployPlanIgnoreDeps       = "ignore-deps"
	argDeployPlanAutoDeps         = "auto-deps"
	argDeployPlanReplace          = "replace"
)

var deployReleasePlanCmd = addCommand(deployPlanCmd, &cobra.Command{
	Use:          "plan",
	Short:        "Plan the deployment of the current release",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		return releaseDeployPlan("release")
	},
})

func getReleaseAndPlanFolderName(b *bosun.Bosun, slotDescription string) (*bosun.ReleaseManifest, string, error) {

	p, err := b.GetCurrentPlatform()
	if err != nil {
		return nil, "", err
	}

	var folder string

	var r *bosun.ReleaseManifest
	switch slotDescription {
	case "release", "current":
		r, err = p.GetCurrentRelease()
		if err == nil {
			folder = r.Version.String()
		}
		break
	case bosun.SlotStable:
		r, err = p.GetStableRelease()
		folder = bosun.SlotStable
		break
	case bosun.SlotUnstable:
		r, err = p.GetUnstableRelease()
		folder = bosun.SlotUnstable
		break
	default:
		err = errors.Errorf("unsupported release slot description %q", slotDescription)
	}
	if r == nil {
		return nil, "", errors.Errorf("no release loaded for slotDescription %q", slotDescription)
	}

	return r, folder, err
}

func releaseDeployPlan(slotDescription string) error {

	b := MustGetBosun(cli.Parameters{NoEnvironment: true})
	p, err := b.GetCurrentPlatform()
	if err != nil {
		return err
	}

	r, folder, err := getReleaseAndPlanFolderName(b, slotDescription)
	if err != nil {
		return err
	}

	deploymentPlanPath := filepath.Join(p.GetDeploymentsDir(), fmt.Sprintf("%s/plan.yaml", folder))

	previousPlan, _ := bosun.LoadDeploymentPlanFromFile(deploymentPlanPath)
	basedOnHash, err := r.GetChangeDetectionHash()
	if err != nil {
		return err
	}
	var req = bosun.CreateDeploymentPlanRequest{
		Path:                  deploymentPlanPath,
		ProviderPriority:      []string{r.Slot},
		IgnoreDependencies:    true,
		AutomaticDependencies: false,
		ReleaseVersion:        &r.Version,
		BasedOnHash:           basedOnHash,
	}

	knownApps, err := r.GetAppManifests()
	ctx := b.NewContext()

	if viper.GetBool(argDeployPlanAll) {
		ctx.Log().Info("Adding all apps in release to the plan...")

		for _, app := range knownApps {
			req.Apps = append(req.Apps, app.Name)
			ctx.Log().Infof("Adding %s", app.Name)

		}
	} else {
		pinnedApps, pinnedAppsErr := r.GetAppManifestsPinnedToRelease()
		if pinnedAppsErr != nil {
			return pinnedAppsErr
		}

		for name := range pinnedApps {
			req.Apps = append(req.Apps, name)
		}
	}

	planCreator := bosun.NewDeploymentPlanCreator(b, p)

	plan, err := planCreator.CreateDeploymentPlan(req)

	if err != nil {
		return err
	}

	if previousPlan != nil {
		plan.AppDeploymentProgress = previousPlan.AppDeploymentProgress
	}

	err = plan.Save()
	if err != nil {
		return err
	}

	fmt.Println(deploymentPlanPath)

	return nil
}
