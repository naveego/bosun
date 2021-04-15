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

var stackEnsureCmd = addCommand(stackCmd, &cobra.Command{
	Use:          "create [name]",
	Aliases:      []string{"create"},
	Args:         cobra.MaximumNArgs(1),
	Short:        "Configures namespaces and other things for the provided stack. Uses the current stack if none is provided.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		return configureStack(args, func(stack *kube.Stack) error {
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
			return nil
		})
	},
})

var stackEnsureCertsCmd = addCommand(stackEnsureCmd, &cobra.Command{
	Use:          "certs [name]",
	Args:         cobra.MaximumNArgs(1),
	Short:        "Configures certs for the provided stack. Uses the current stack if none is provided.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		return configureStack(args, func(stack *kube.Stack) error {
			return stack.ConfigureCerts()
		})
	},
})

var stackEnsureNamespacesCmd = addCommand(stackEnsureCmd, &cobra.Command{
	Use:          "namespaces [name]",
	Args:         cobra.MaximumNArgs(1),
	Short:        "Configures namespaces for the provided stack. Uses the current stack if none is provided.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		return configureStack(args, func(stack *kube.Stack) error {
			return stack.ConfigureNamespaces()
		})
	},
})

var stackEnsurePullSecretsCmd = addCommand(stackEnsureCmd, &cobra.Command{
	Use:          "pull-secrets [name]",
	Args:         cobra.MaximumNArgs(1),
	Short:        "Configures pull-secrets for the provided stack. Uses the current stack if none is provided.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		return configureStack(args, func(stack *kube.Stack) error {
			return stack.ConfigureNamespaces()
		})
	},
})

func configureStack(args []string, fn func(stack *kube.Stack) error) error {
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

	err = fn(stack)
	if err != nil {
		return err
	}

	err = stack.Save()
	if err != nil {
		return err
	}

	fmt.Println("Stack configured and saved.")

	return nil

}

var stackDestroyCmd = addCommand(stackCmd, &cobra.Command{
	Use:          "destroy [name]",
	Aliases:      []string{"delete"},
	Args:         cobra.MaximumNArgs(1),
	Short:        "Deletes the configured stack and all of it's resources.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {


		return configureStack(args, func(stack *kube.Stack) error {

			confirmed := cli.RequestConfirmFromUser("Are you sure you want to delete the stack %q and all of its resources", stack.Name)

			if !confirmed {
				return nil
			}

			return stack.Destroy()

			return nil
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

		stacks, err := cluster.GetStackConfigs()
		if err != nil {
			return err
		}

		for name := range stacks {
			fmt.Println(name)
		}

		return nil
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

		stackState, err := stack.GetState()
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
		env := b.GetCurrentEnvironment()

		var appNames []string

		for appName := range p.GetKnownAppMap() {
			if !env.IsAppDisabled(appName) && !env.Stack().IsAppDisabled(appName) {
				appNames = append(appNames, appName)
			}
		}

		stackState, err := env.Stack().GetState()
		if err != nil {
			return err
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

		var appsNeedingReset []*bosun.AppDeploymentPlan

		ctx := b.NewContext()

		for _, app := range plan.Apps {

			log := ctx.Log().WithField("app", app.Name).WithField("branch", app.Manifest.Branch).WithField("version", app.Manifest.Version.String())

			deployedApp, isDeployed := stackState.DeployedApps[app.Name]
			if !isDeployed {
				appsNeedingReset = append(appsNeedingReset, app)
				continue
			}

			if app.Manifest.Branch == deployedApp.Branch &&
				app.Manifest.Version.String() == deployedApp.Version {
				log.Info("Skipping app reset because branch and version match what is currently deployed.")
				continue
			}
		}

		plan.Apps = appsNeedingReset

		executeRequest := bosun.ExecuteDeploymentPlanRequest{
			Plan: plan,
		}

		executor := bosun.NewDeploymentPlanExecutor(b, p)

		_, err = executor.Execute(executeRequest)

		return nil
	},
})
