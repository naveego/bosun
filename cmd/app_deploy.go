package cmd

import (
	"fmt"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var appDeployCmd = addCommand(appCmd, &cobra.Command{
	Use:   "deploy [name] [name...]",
	Short: "Deploys the requested app.",
	Long: `If app is not specified, the first app in the nearest bosun.yaml file is used.

If you want to apply specific value sets to this deployment, use the --value-sets (or -v) flag.
You can view the available value-sets using "bosun env value-sets". 

A common use case for using value-sets is if you want to change the tag and pull behavior so that
the pod will use an image you just built using the minikube docker agent. The "latest" and "pullIfNotPresent"
value-sets are available for this. To use them:

bosun app deploy {appName} --value-sets latest,pullIfNotPresent
`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := MustGetBosun()

		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

		ctx := b.NewContext()

		apps := mustGetAppsIncludeCurrent(b, args)

		// fmt.Println("App Configs:")
		// for _, app := range apps {
		// 	fmt.Println(MustYaml(app.AppConfig))
		// }

		valueSets, err := getValueSetSlice(b, b.GetCurrentEnvironment())
		if err != nil {
			return err
		}
		includeDeps := viper.GetBool(ArgAppDeployDeps)
		deploySettings := bosun.DeploySettings{
			Environment:        ctx.Env,
			ValueSets:          valueSets,
			UseLocalContent:    true,
			IgnoreDependencies: !includeDeps,
			Apps:               map[string]*bosun.App{},
		}

		for _, app := range apps {
			ctx.Log().WithField("app", app.Name).Debug("Including in release.")
			deploySettings.Apps[app.Name] = app
			if includeDeps {
				ctx.Log().Debug("Including dependencies of all apps...")
				deps, err := b.GetAppDependencies(app.Name)
				if err != nil {
					return errors.Wrapf(err, "getting deps for %q", app.Name)
				}
				for _, depName := range deps {
					if _, ok := deploySettings.Apps[depName]; !ok {

						depApp, err := b.GetApp(depName)
						if err != nil {
							return errors.Wrapf(err, "getting app for dep %q", app.Name)
						}
						deploySettings.Apps[depApp.Name] = depApp
					}
				}
			}
		}

		fromRelease := viper.GetString(ArgAppFromRelease)

		if fromRelease != "" {
			ctx.Log().Warnf("Deploying app from %q rather than your local clone", fromRelease)
			var p *bosun.Platform
			p, err = b.GetCurrentPlatform()
			if err != nil {
				return err
			}

			stableRelease, err := p.GetStableRelease()
			if err != nil {
				return errors.Wrap(err, "no stable release; do you have the latest devops master?")
			}
			if stableRelease.Name != fromRelease {
				return errors.Errorf("currently can only deploy from stable release, which is %q", stableRelease.ReleaseMetadata.String())
			}

			deploySettings.Manifest = stableRelease
			// reset the default deploy apps so that only the selected apps are deployed
			deploySettings.Manifest.UpgradedApps = map[string]bool{}
			for _, app := range apps {
				deploySettings.Manifest.UpgradedApps[app.Name] = true
			}
		} else if viper.GetBool(ArgAppLatest) {
			err = pullApps(ctx, apps, true)
			deploySettings.ValueSets = append(deploySettings.ValueSets, values.ValueSet{Static: map[string]interface{}{"tag": "latest"}})
		}

		r, err := bosun.NewDeploy(ctx, deploySettings)
		if err != nil {
			return err
		}

		ctx.Log().Debugf("Created deploy")

		if viper.GetBool(argAppDeployPreview) {
			for _, app := range r.AppDeploys {
				values, err := app.GetResolvedValues(ctx)
				if err != nil {
					return errors.Wrap(err, "get resolved values")
				}
				fmt.Printf("%s:\n", app.Name)
				y := util.MustYaml(values)
				fmt.Println(y)
				fmt.Println()
			}
			return nil
		}

		err = r.Deploy(ctx)

		if err != nil {
			return errors.Wrap(err, "deploy failed")
		}

		err = b.Save()

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(ArgAppDeployDeps, false, "Also deploy all dependencies of the requested apps.")
	cmd.Flags().Bool(argAppDeployPreview, false, "Just dump the values which would be used to deploy, then exit.")
	cmd.Flags().StringP(ArgAppFromRelease, "r", "", "Deploy using the specified release from the platform, rather than your local clone.")
	cmd.Flags().Bool(ArgAppLatest, false, "Force bosun to pull the latest of the app and deploy that.")
	cmd.Flags().StringSliceP(ArgAppValueSet, "v", []string{}, "Additional value sets to include in this deploy.")
	cmd.Flags().StringSliceP(ArgAppSet, "s", []string{}, "Value overrides to set in this deploy, as key=value pairs.")

})

const (
	argAppDeployPreview = "preview"
)
