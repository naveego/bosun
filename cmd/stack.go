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
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/mirror"
	"github.com/naveego/bosun/pkg/util/stringsn"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sort"
)

func init() {

}

var stackCmd = addCommand(rootCmd, &cobra.Command{
	Use:   "stack",
	Short: "Contains commands for managing a stack of apps.",
})

var stackCreateCmd = addCommand(stackCmd, &cobra.Command{
	Use:          "create {name} [template]",
	Args:         cobra.RangeArgs(1, 2),
	Short:        "Configures namespaces and other things for the provided stack. Uses the current stack if none is provided.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b, _ := MustGetPlatform()
		env := b.GetCurrentEnvironment()

		cluster := env.Cluster()

		name := args[0]
		var templateName string
		if len(args) == 2 {
			templateName = args[1]
		} else {

			var templateChoices []string
			for _, template := range cluster.GetStackTemplates() {
				templateChoices = append(templateChoices, template.Name)
			}

			sort.Strings(templateChoices)

			templateName = cli.RequestChoice("Choose a template for your stack", templateChoices...)
		}

		stack, err := cluster.CreateStack(name, templateName)
		if err != nil {
			return err
		}

		err = stack.Initialize()

		return err
	},
})

var stackShowCmd = addCommand(stackCmd, &cobra.Command{
	Use:          "show [name]",
	Args:         cobra.MaximumNArgs(1),
	Aliases:      []string{"view"},
	Short:        "Shows the current state of a stack or the current stack",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b, _ := MustGetPlatform()
		env := b.GetCurrentEnvironment()

		cluster := env.Cluster()

		var err error
		var stack *kube.Stack

		if len(args) == 1 {
			stackName := args[0]
			stack, err = cluster.GetStack(stackName)
			if err != nil {
				return err
			}
		} else {
			stack = env.Stack()
		}

		stackState, err := stack.GetState(viper.GetBool(argStackShowSync))
		if err != nil {
			return err
		}

		y, _ := yaml.MarshalString(stackState)

		fmt.Println(y)

		return nil
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(argStackShowSync, false, "Force reload of stack info from kubernetes.")
})

const (
	argStackShowSync = "sync"
)

var stackEnsureCmd = addCommand(stackCmd, &cobra.Command{
	Use:          "configure [name]",
	Args:         cobra.MaximumNArgs(1),
	Short:        "Configures namespaces and other things for the provided stack. Uses the current stack if none is provided.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		return configureStack(args, func(stack *kube.Stack) (bool, error) {
			err := stack.ConfigureNamespaces()
			if err != nil {
				color.Red("Could not configure namespaces: %+v", err)
			}

			err = stack.ConfigurePullSecrets()
			if err != nil {
				color.Red("Could not configure pull secrets: %+v", err)
			}

			err = stack.ConfigureCerts()
			if err != nil {
				color.Red("Could not configure certs: %+v", err)
			}

			err = stack.Save()
			return false, err
		})
	},
})

var stackEnsureCertsCmd = addCommand(stackEnsureCmd, &cobra.Command{
	Use:          "certs [name]",
	Args:         cobra.MaximumNArgs(1),
	Short:        "Configures certs for the provided stack. Uses the current stack if none is provided.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		return configureStack(args, func(stack *kube.Stack) (bool, error) {
			err := stack.ConfigureCerts()
			return true, err
		})
	},
})

var stackEnsureNamespacesCmd = addCommand(stackEnsureCmd, &cobra.Command{
	Use:          "namespaces [name]",
	Args:         cobra.MaximumNArgs(1),
	Short:        "Configures namespaces for the provided stack. Uses the current stack if none is provided.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		return configureStack(args, func(stack *kube.Stack) (bool, error) {
			err := stack.ConfigureNamespaces()
			return true, err
		})
	},
})

var stackEnsurePullSecretsCmd = addCommand(stackEnsureCmd, &cobra.Command{
	Use:          "pull-secrets [name]",
	Args:         cobra.MaximumNArgs(1),
	Short:        "Configures pull-secrets for the provided stack. Uses the current stack if none is provided.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		return configureStack(args, func(stack *kube.Stack) (bool, error) {
			err := stack.ConfigureNamespaces()
			return true, err
		})
	},
})

func configureStack(args []string, fn func(stack *kube.Stack) (bool, error)) error {
	b, _ := MustGetPlatform()
	env := b.GetCurrentEnvironment()

	cluster := env.Cluster()

	var err error
	var stack *kube.Stack
	if len(args) == 1 {
		stack, err = cluster.GetStack(args[0])
		if err != nil {
			return err
		}
	} else {
		stack = env.Stack()
	}

	var save bool
	save, err = fn(stack)
	if err != nil {
		return err
	}

	if save {
		err = stack.Save()
		if err != nil {
			return err
		}

		fmt.Println("Stack configured and saved.")
	}

	return nil

}

var stackDestroyCmd = addCommand(stackCmd, &cobra.Command{
	Use:          "destroy [name]",
	Aliases:      []string{"delete"},
	Args:         cobra.MaximumNArgs(1),
	Short:        "Deletes the configured stack and all of it's resources.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		return configureStack(args, func(stack *kube.Stack) (bool, error) {

			confirmed := cli.RequestConfirmFromUser("Are you sure you want to delete the stack %q and all of its resources", stack.Name)

			if !confirmed {
				return false, nil
			}

			err := stack.Destroy()
			return false, err
		})
	},
})

var stackLsCmd = addCommand(stackCmd, &cobra.Command{
	Use:          "ls",
	Aliases:      []string{"list", "view"},
	Short:        "Lists the stacks in the current cluster.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b, _ := MustGetPlatform()
		env := b.GetCurrentEnvironment()

		cluster := env.Cluster()

		stacks, err := cluster.GetStackStates()
		if err != nil {
			return err
		}

		return renderOutput(stacks)
	},
})

var stackAppsCmd = addCommand(stackCmd, &cobra.Command{
	Use:          "apps",
	Short:        "Shows the apps in the current stack.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b, p := MustGetPlatform()
		env := b.GetCurrentEnvironment()

		stack := env.Stack()

		knownApps := p.GetKnownAppMap()

		skippedGlobalApps := 0
		skippedEnvApps := 0

		stackState, err := stack.GetState(true)
		if err != nil {
			return err
		}

		for _, knownApp := range knownApps {

			_, deployed := stackState.DeployedApps[knownApp.Name]
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

			disabledForCluster := env.Stack().IsAppDisabled(knownApp.Name)

			if disabledForCluster {
				if !viper.GetBool(argStackIncludeEnvApps) && !viper.GetBool(argStackIncludeAllApps) {
					skippedEnvApps++
					continue
				}

				stackApp.Details += " Disabled for stack"
			}

			stackState.DeployedApps[knownApp.Name] = stackApp

		}

		ctx := b.NewContext()
		if skippedGlobalApps > 0 {
			ctx.Log().Infof("Omitted %d apps which are disabled for this environment, use --%s flag to show them", skippedGlobalApps, argStackIncludeAllApps)
		}
		if skippedEnvApps > 0 {
			ctx.Log().Infof("Omitted %d apps which are disabled for this cluster, use --%s flag to show them", skippedEnvApps, argStackIncludeEnvApps)
		}

		return printOutputWithDefaultFormat("table", stackState)
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

		return resetCurrentStack(b, p, args[0])
	},
})

var stackRedeployCmd = addCommand(stackCmd, &cobra.Command{
	Use:          "redeploy [apps...]",
	Short:        "Redeploys apps deployed to the stack using --force to ensure they are deployed. If no apps are provided as args then all will be redeployed.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b, p := MustGetPlatform(cli.Parameters{Force: true})

		var appNames []string

		env := b.GetCurrentEnvironment()

		var err error

		for appName := range p.GetKnownAppMap() {
			if !env.IsAppDisabled(appName) && !env.Stack().IsAppDisabled(appName) {
				appNames = append(appNames, appName)
			}
		}

		stackState, err := env.Stack().GetState(true)
		if err != nil {
			return err
		}

		ctx := b.NewContext()

		for appName, stackApp := range stackState.DeployedApps {

			appLog := ctx.Log().WithField("app", appName)

			if len(args) > 0 && !stringsn.Contains(args, appName) {
				appLog.Info("Skipping deploy because it wasn't requested")
				continue
			}

			appLog.Info("Re-deploying...")

			var req = bosun.CreateDeploymentPlanRequest{
				IgnoreDependencies:    true,
				AutomaticDependencies: false,
				AppOptions:            map[string]bosun.AppProviderRequest{},
			}

			appReq := bosun.AppProviderRequest{
				Name:   appName,
				Path:   "",
				Branch: stackApp.Branch,
			}

			if stackApp.Provider == bosun.SlotStable || stackApp.Provider == bosun.SlotUnstable {
				appReq.ProviderPriority = []string{stackApp.Provider}
			} else {
				appReq.Path = stackApp.Provider
			}

			req.AppOptions[appName] = appReq
			planCreator := bosun.NewDeploymentPlanCreator(b, p)

			plan, appErr := planCreator.CreateDeploymentPlan(req)

			if appErr != nil {
				appLog.WithError(appErr).Error("Failed when creating deployment plan.")
				return appErr
			}

			executeRequest := bosun.ExecuteDeploymentPlanRequest{
				Plan: plan,
			}

			executor := bosun.NewDeploymentPlanExecutor(b, p)

			_, appErr = executor.Execute(executeRequest)
			if appErr != nil {
				appLog.WithError(appErr).Error("Failed when executing deployment plan.")
			}
			appLog.WithField("req", appReq).Info("Re-deployed app.")
		}

		return nil
	},
})

func resetCurrentStack(b *bosun.Bosun, p *bosun.Platform, provider string) error {
	var appNames []string

	env := b.GetCurrentEnvironment()

	var err error

	for appName := range p.GetKnownAppMap() {
		if !env.IsAppDisabled(appName) && !env.Stack().IsAppDisabled(appName) {
			appNames = append(appNames, appName)
		}
	}

	stackState, err := env.Stack().GetState(true)
	if err != nil {
		return err
	}

	var providers []string
	switch provider {
	case bosun.SlotStable:
		providers = []string{bosun.SlotStable, bosun.SlotUnstable, bosun.WorkspaceProviderName}
	case bosun.SlotUnstable:
		providers = []string{bosun.SlotUnstable, bosun.WorkspaceProviderName}
	case bosun.WorkspaceProviderName:
		providers = []string{bosun.WorkspaceProviderName, bosun.SlotUnstable, bosun.SlotStable}
	default:
		return errors.Errorf("invalid release slot %q", provider)
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

	var appsNeedingReset []*bosun.AppDeploymentPlan

	ctx := b.NewContext()

	for _, app := range plan.Apps {

		log := ctx.Log().WithField("app", app.Name).WithField("branch", app.Manifest.Branch).WithField("version", app.Manifest.Version.String())

		deployedApp, isDeployed := stackState.DeployedApps[app.Name]
		if isDeployed && !ctx.GetParameters().Force {
			if app.Manifest.Branch == deployedApp.Branch &&
				app.Manifest.Version.String() == deployedApp.Version {
				log.Info("Skipping app reset because branch and version match what is currently deployed.")
				continue
			}
		}

		appsNeedingReset = append(appsNeedingReset, app)
	}

	mirror.Sort(appsNeedingReset, func(a, b *bosun.AppDeploymentPlan) bool { return a.Name < b.Name })

	appSelect := map[string]*bosun.AppDeploymentPlan{}
	appSelectOptions := []string{}

	for _, app := range appsNeedingReset {
		key := fmt.Sprintf("%s @ %s from %s", app.Name, app.Tag, app.ManifestPath)
		appSelect[key] = app
		appSelectOptions = append(appSelectOptions, key)
	}

	confirmed := cli.RequestMultiChoice("Which apps do you want to reset?", appSelectOptions)
	if len(confirmed) == 0 {
		return errors.New("User cancelled")
	} else {
		plan.Apps = nil
		for _, key := range confirmed {
			plan.Apps = append(plan.Apps, appSelect[key])
		}
	}

	executeRequest := bosun.ExecuteDeploymentPlanRequest{
		Plan: plan,
	}

	executor := bosun.NewDeploymentPlanExecutor(b, p)

	_, err = executor.Execute(executeRequest)

	return err

}
