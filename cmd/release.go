package cmd

import (
	"fmt"
	"github.com/aryann/difflib"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/util"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/vbauerster/mpb/v4"
	"github.com/vbauerster/mpb/v4/decor"
	"gopkg.in/yaml.v2"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

func runParentPersistentPreRunE(cmd *cobra.Command, args []string) error {
	parent := cmd.Parent()
	for parent != nil {
		if parent.PersistentPreRunE != nil {
			err := parent.PersistentPreRunE(cmd, args)
			if err != nil {
				return errors.Wrapf(err, "parent.PersistentPreRunE (%s)", parent.Name())
			}
		}
		parent = parent.Parent()
	}
	return nil
}
func runParentPersistentPostRunE(cmd *cobra.Command, args []string) error {
	parent := cmd.Parent()
	for parent != nil {
		if parent.PersistentPreRunE != nil {
			err := parent.PersistentPostRunE(cmd, args)
			if err != nil {
				return errors.Wrapf(err, "parent.PersistentPreRunE (%s)", parent.Name())
			}
		}
		parent = parent.Parent()
	}
	return nil
}

// releaseCmd represents the release command
var releaseCmd = addCommand(rootCmd, &cobra.Command{
	Use:     "release",
	Aliases: []string{"rel", "r"},
	Short:   "Contains sub-commands for releases.",
})

var originalCurrentRelease *string

const (
	ArgReleaseName = "release"
)

var _ = addCommand(releaseCmd, &cobra.Command{
	Use:          "use {current|stable|unstable|name}",
	Args:         cobra.ExactArgs(1),
	Short:        "Sets the current release.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()

		err := b.UseRelease(args[0])
		if err != nil {
			return err
		}

		return b.Save()
	},
})

var releaseListCmd = addCommand(releaseCmd, &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "Lists known releases.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()

		t := tablewriter.NewWriter(os.Stdout)
		t.SetCenterSeparator("")
		t.SetColumnSeparator("")
		t.SetHeader([]string{"", "RELEASE", "VERSION", "PATH"})
		platform, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		current, err := b.GetStableReleaseManifest()
		if err != nil {
			return err
		}

		for _, release := range platform.GetReleaseMetadataSortedByVersion(true) {
			name := release.Name
			currentMark := ""
			if current != nil && release.Name == current.Name {
				currentMark = "*"
				name = color.GreenString("%s", name)
			}

			t.Append([]string{currentMark, name, release.Version.String(), release.Description})
		}

		t.Render()

		if current == nil {
			color.Red("No current release selected (use `bosun release use {name}` to select one).")
		} else {
			color.White("(* indicates currently active release)")
		}
		return nil
	},
})

var releaseReplanCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "replan [apps...]",
	Short: "Returns the release to the planning stage.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b, p := getReleaseCmdDeps()

		apps, err := getKnownApps(b, args)
		if err != nil {
			return err
		}

		confirmMsg := "Replanning release for all apps."
		if len(apps) > 0 {
			appNames := []string{}
			for _, app := range apps {
				appNames = append(appNames, app.Name)
			}
			confirmMsg = "Replanning release for these apps: " + strings.Join(appNames, ", ")
		}
		confirm(confirmMsg)

		ctx := b.NewContext()
		_, err = p.RePlanRelease(ctx, apps...)
		if err != nil {
			return err
		}

		err = p.Save(ctx)

		return err
	},
})

var releaseShowCmd = addCommand(releaseCmd, &cobra.Command{
	Use:     "show",
	Aliases: []string{"dump"},
	Short:   "Lists the apps in the current release.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}
		rm := mustGetRelease(p, bosun.SlotCurrent, bosun.SlotCurrent)
		if err != nil {
			return err
		}

		err = printOutput(rm)
		return err
	},
})

var releaseDotCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "dot",
	Short: "Prints a dot diagram of the release.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		rm, err := b.GetStableReleaseManifest()
		if err != nil {
			return err
		}

		out, err := rm.ExportDiagram()
		fmt.Println(out)
		return err
	},
})

func getReleaseCmdDeps() (*bosun.Bosun, *bosun.Platform) {
	b := MustGetBosun()
	p, err := b.GetCurrentPlatform()
	if err != nil {
		log.Fatal(err)
	}
	return b, p
}

var releaseImpactCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "impact",
	Short: "Reports on the changes deploying the release will inflict on the current environment.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Re-implement release impact command
		return errors.New("needs to be re-implemented after release manifest refactor")
		// b, p := getReleaseCmdDeps()
		//
		// if len(viper.GetStringSlice(ArgFilteringLabels)) == 0 && len(args) == 0 {
		// 	viper.Set(ArgFilteringAll, true)
		// }
		//
		// apps, err := getAppsIncludeCurrent(b, args)
		// if err != nil {
		// 	return err
		// }
		// requestedApps := map[string]bool{}
		// for _, app := range apps {
		// 	requestedApps[app.Name] = true
		// }
		//
		// total := len(requestedApps)
		// complete := 0
		//
		// appReleases := r.AppReleases
		// wg := new(sync.WaitGroup)
		// wg.Add(len(appReleases))
		// for _, appRelease := range appReleases {
		// 	if !requestedApps[appRelease.Name] {
		// 		continue
		// 	}
		// 	go func(appRelease *bosun.AppDeploy) {
		// 		defer wg.Done()
		//
		// 		ctx := b.NewContext().WithAppDeploy(appRelease)
		// 		values, err := appRelease.GetResolvedValues(ctx)
		// 		if err != nil {
		// 			ctx.Log().WithError(err).Error("Could not create values map for app release.")
		// 			return
		// 		}
		//
		// 		ctx = ctx.WithPersistableValues(values)
		// 		err = appRelease.LoadActualState(ctx, true)
		// 		if err != nil {
		// 			ctx.Log().WithError(err).Error("Could not load actual state.")
		// 			return
		// 		}
		// 		complete += 1
		// 		color.White("Loaded %s (%d/%d)", appRelease.Name, complete, total)
		// 		wg.Done()
		// 	}(appRelease)
		// }
		// wg.Wait()
		//
		// for _, appRelease := range appReleases {
		// 	color.Blue("%s\n", appRelease.Name)
		// 	if appRelease.ActualState.Diff == "" {
		// 		color.White("No diff detected.")
		// 	} else {
		// 		color.Yellow("Diff:\n")
		// 		fmt.Println(appRelease.ActualState.Diff)
		// 		fmt.Println()
		// 	}
		// }
		//
		// color.Blue("SUMMARY:\n")
		// for _, appRelease := range appReleases {
		// 	color.Blue(appRelease.Name)
		// 	if appRelease.ActualState.Diff == "" {
		// 		color.White("No diff detected.\n")
		// 	} else {
		// 		fmt.Println("Has changes (see above).")
		// 	}
		// }
		//
		return nil
	},
}, withFilteringFlags)

var releaseShowValuesCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "show-values {app}",
	Args:  cobra.ExactArgs(1),
	Short: "Shows the values which will be used for a release.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b, _ := getReleaseCmdDeps()
		releaseManifest := mustGetActiveRelease(b)

		appManifest, err := releaseManifest.GetAppManifest(args[0])
		if appManifest == nil {
			return errors.Errorf("app %q not in this release", args[0])
		}

		values, err := getResolvedValuesFromAppManifest(b, appManifest)

		yml, err := yaml.Marshal(values)
		if err != nil {
			return err
		}

		fmt.Println(string(yml))

		return nil
	},
})

var releaseAddCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "add {current|unstable} {app} {bump}",
	Args:  cobra.ExactArgs(3),
	Short: "Adds an app to the release.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()

		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		r := mustGetRelease(p, args[0], bosun.SlotUnstable, bosun.SlotCurrent)

		app, err := b.GetApp(args[1])
		if err != nil {
			return err
		}

		ctx := b.NewContext()

		branch := viper.GetString(ArgReleaseAddBranch)

		appManifest, err := r.PrepareAppManifest(ctx, app, semver.Bump(args[2]), branch)
		if err != nil {
			return err
		}

		err = r.AddApp(appManifest, true)
		if err != nil {
			return err
		}

		err = p.Save(ctx)
		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().String(ArgReleaseAddBranch, "", "The branch to add the app from (defaults to the branch pattern for the slot).")
})

const (
	ArgReleaseAddBranch = "branch"
)

//
// var releaseExcludeCmd = addCommand(releaseCmd, &cobra.Command{
// 	Use:   "exclude [names...]",
// 	Short: "Excludes and removes one or more apps from a release.",
// 	Long: "Provide app names or use labels. The matched apps will be removed " +
// 		"from the release and will not be re-added even if apps which depend on " +
// 		"them are added or synced. If the app is explicitly added it will be " +
// 		"removed from the exclude list.",
// 	RunE: func(cmd *cobra.Command, args []string) error {
// 		viper.BindPFlags(cmd.Flags())
// 		b := MustGetBosun()
// 		release := mustGetCurrentRelease(b)
//
// 		apps, err := getAppsIncludeCurrent(b, args)
// 		if err != nil {
// 			return err
// 		}
//
// 		for _, app := range apps {
// 			delete(release.AppReleaseConfigs, app.Name)
// 			release.Exclude[app.Name] = true
// 		}
//
// 		err = release.Parent.Save()
// 		return err
// 	},
// }, withFilteringFlags)

var releaseValidateCmd = addCommand(releaseCmd, &cobra.Command{
	Use:           "validate [names...]",
	Short:         "Validates the release.",
	Long:          "Validation checks that all apps (or the named apps) in the current release have a published chart and docker image.",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		b := MustGetBosun()
		release := mustGetActiveRelease(b)

		valueSets, err := getValueSetSlice(b, b.GetCurrentEnvironment())
		if err != nil {
			return err
		}
		ctx := b.NewContext()

		deploySettings := bosun.DeploySettings{
			Environment: ctx.Env,
			ValueSets:   valueSets,
			Manifest:    release,
		}

		getFilterParams(b, args).ApplyToDeploySettings(&deploySettings)

		deploy, err := bosun.NewDeploy(ctx, deploySettings)
		if err != nil {
			return err
		}

		return validateDeploy(b, ctx, deploy)
	},
},
	withFilteringFlags,
	func(cmd *cobra.Command) {
		cmd.Flags().Bool(ArgReleaseValidateNoProgress, false, "Do not emit progress bars.")
	})

const (
	ArgReleaseValidateNoProgress = "no-progress"
)

func validateDeploy(b *bosun.Bosun, ctx bosun.BosunContext, release *bosun.Deploy) error {

	showProgress := !viper.GetBool(ArgReleaseValidateNoProgress)

	hasErrors := false

	apps := release.AppDeploys

	var wg sync.WaitGroup
	// pass &wg (optional), so p will wait for it eventually
	var p *mpb.Progress
	if showProgress {
		p = mpb.New(mpb.WithWaitGroup(&wg))
	}

	errmu := new(sync.Mutex)

	// ctx.GetMinikubeDockerEnv()

	err := pkg.NewCommand("helm", "repo", "update").RunE()
	if err != nil {
		return errors.Wrap(err, "update repo indexes")
	}

	errs := map[string][]error{}
	start := time.Now()
	for i := range apps {
		app := apps[i]
		if app.Excluded {
			continue
		}
		var bar *mpb.Bar
		wg.Add(1)
		if showProgress {
			bar = p.AddBar(100, mpb.PrependDecorators(decor.Name(app.Name)),
				mpb.AppendDecorators(decor.OnComplete(decor.EwmaETA(decor.ET_STYLE_GO, 60), "done")))
		}

		go func() {
			err := app.Validate(ctx)
			if err != nil {
				errmu.Lock()
				defer errmu.Unlock()
				errs[app.Name] = err
			}
			if showProgress {
				bar.IncrBy(100, time.Since(start))
			}
			wg.Done()
		}()

	}
	if showProgress {
		p.Wait()
	} else {
		wg.Wait()
	}

	t := tablewriter.NewWriter(os.Stdout)
	t.SetHeader([]string{"app", "state"})

	for _, appName := range util.SortedKeys(apps) {
		appErrs := errs[appName]
		var state []string

		if len(appErrs) == 0 {
			state = append(state, colorOK.Sprint("OK"))
		} else {
			hasErrors = true
			for _, err := range appErrs {
				state = append(state, colorError.Sprint(err))
			}
		}
		t.Append([]string{appName, strings.Join(state, "\n")})
	}

	t.Render()

	if hasErrors {
		return errors.New("Some apps are invalid.")
	}

	return nil
}

var releaseTestCmd = addCommand(releaseCmd, &cobra.Command{
	Use:           "test",
	Short:         "Runs the tests for the apps in the release.",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		b := MustGetBosun()
		ctx := b.NewContext()
		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

		appReleases := getFilterParams(b, args).MustGetAppDeploys()

		for _, appRelease := range appReleases {

			ctx = ctx.WithAppDeploy(appRelease)
			for _, action := range appRelease.AppConfig.Actions {
				if action.Test != nil {
					err := action.Execute(ctx)
					if err != nil {
						ctx.Log().WithError(err).Error("Test failed.")
					}
				}
			}
		}

		return nil
	},
}, withFilteringFlags)

var releaseDeployCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "deploy [apps...]",
	Short: "Deploys the release.",
	Long: `Deploys the current release to the current environment. If apps are provided, 
only those apps will be deployed. Otherwise, all apps in the release will be deployed (subject to --include flags).`,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := MustGetBosun()
		release := mustGetActiveRelease(b)
		ctx := b.NewContext()
		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

		valueSets, err := getValueSetSlice(b, b.GetCurrentEnvironment())
		if err != nil {
			return err
		}

		deploySettings := bosun.DeploySettings{
			Environment:     ctx.Env,
			ValueSets:       valueSets,
			Manifest:        release,
			Recycle:         viper.GetBool(ArgReleaseRecycle),
			ForceDeployApps: map[string]bool{},
		}
		for _, appName := range args {
			deploySettings.ForceDeployApps[appName] = true
		}

		getFilterParams(b, args).ApplyToDeploySettings(&deploySettings)

		deploy, err := bosun.NewDeploy(ctx, deploySettings)
		if err != nil {
			return err
		}

		color.Yellow("About to deploy the following apps:")
		for _, name := range util.SortedKeys(deploy.AppDeploys) {
			app := deploy.AppDeploys[name]
			fmt.Printf("- %s: %s (tag %s)\n", name, app.Version, deploySettings.GetImageTag(app.AppMetadata))
		}

		if !confirm("Is this what you expected") {
			return errors.New("Deploy cancelled.")
		}

		if viper.GetBool(ArgReleaseSkipValidate) {
			ctx.Log().Warn("Validation disabled.")
		} else {
			ctx.Log().Info("Validating...")
			err = validateDeploy(b, ctx, deploy)
			if err != nil {
				return err
			}
		}

		err = deploy.Deploy(ctx)

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(ArgReleaseSkipValidate, false, "Skips running validation before deploying the release.")
	cmd.Flags().Bool(ArgReleaseRecycle, false, "Recycles apps after they are deployed.")
},
	withFilteringFlags,
	withValueSetFlags)

var releaseCommitCmd = addCommand(releaseCmd, &cobra.Command{
	Use:           "commit",
	Short:         "Merges the release branch back to master for each app in the release, and the platform repository.",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := MustGetBosun()
		ctx := b.NewContext()

		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		err = p.CommitCurrentRelease(ctx)
		if err != nil {
			return err
		}

		return nil
	},
}, withFilteringFlags)

var releaseUpdateCmd = addCommand(releaseCmd, &cobra.Command{
	Use:           "update {stable|unstable|current} [apps...]",
	Short:         "Updates the release with the correct values from the apps in it.",
	Args:          cobra.MinimumNArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := MustGetBosun()
		ctx := b.NewContext()

		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}
		release, err := p.GetReleaseManifestBySlot(args[0])
		if err != nil {
			return err
		}

		apps, err := getKnownApps(b, args[1:])
		if err != nil {
			return err
		}

		confirmMsg := "OK to refresh all apps"
		if len(apps) > 0 {
			appNames := []string{}
			for _, app := range apps {
				appNames = append(appNames, app.Name)
			}
			confirmMsg = fmt.Sprintf("OK to refresh release %s for these apps: %s", release, strings.Join(appNames, ", "))
		}
		confirm(confirmMsg)

		err = release.RefreshApps(ctx, apps...)
		if err != nil {
			return err
		}

		err = p.Save(ctx)

		return err
	},
}, withFilteringFlags)

var releaseChangelogCmd = addCommand(releaseCmd, &cobra.Command{
	Use:           "change-log",
	Short:         "Outputs the changelog for the release.",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := MustGetBosun()
		ctx := b.NewContext()

		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		err = p.CommitCurrentRelease(ctx)
		if err != nil {
			return err
		}

		return nil
	},
}, withFilteringFlags)

const ArgReleaseSkipValidate = "skip-validation"
const ArgReleaseRecycle = "recycle"

var releaseDiffCmd = addCommand(
	releaseCmd,
	&cobra.Command{
		Use:   "diff {app} [release/]{env} [release]/{env}",
		Short: "Reports the differences between the values for an app in two scenarios.",
		Long:  `If the release part of the scenario is not provided, a transient release will be created and used instead.`,
		Example: `This command will show the differences between the values deployed 
to the blue environment in release 2.4.2 and the current values for the
green environment:

diff go-between 2.4.2/blue green
`,
		Args:          cobra.ExactArgs(3),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			b := MustGetBosun()
			app := mustGetApp(b, []string{args[0]})

			env1 := args[1]
			env2 := args[2]

			p, err := b.GetCurrentPlatform()
			if err != nil {
				return err
			}

			getValuesForEnv := func(scenario string) (string, error) {

				segs := strings.Split(scenario, "/")
				var releaseName, envName string
				var appDeploy *bosun.AppDeploy
				switch len(segs) {
				case 1:
					envName = segs[0]
				case 2:
					releaseName = segs[0]
					envName = segs[1]
				default:
					return "", errors.Errorf("invalid scenario %q", scenario)
				}

				env, err := b.GetEnvironment(envName)
				if err != nil {
					return "", errors.Wrap(err, "environment")
				}
				ctx := b.NewContext().WithEnv(env)

				var ok bool
				if releaseName != "" {
					releaseManifest, err := p.GetReleaseManifestBySlot(releaseName)

					valueSets, err := getValueSetSlice(b, env)
					if err != nil {
						return "", err
					}

					deploySettings := bosun.DeploySettings{
						Environment: ctx.Env,
						ValueSets:   valueSets,
						Manifest:    releaseManifest,
					}

					deploy, err := bosun.NewDeploy(ctx, deploySettings)
					if err != nil {
						return "", err
					}

					appDeploy, ok = deploy.AppDeploys[app.Name]
					if !ok {
						return "", errors.Errorf("no app named %q in release %q", app.Name, releaseName)
					}

				} else {
					valueSets, err := getValueSetSlice(b, env)
					if err != nil {
						return "", err
					}

					deploySettings := bosun.DeploySettings{
						Environment: ctx.Env,
						ValueSets:   valueSets,
						Apps: map[string]*bosun.App{
							app.Name: app,
						},
					}

					deploy, err := bosun.NewDeploy(ctx, deploySettings)
					if err != nil {
						return "", err
					}

					appDeploy, ok = deploy.AppDeploys[app.Name]
					if !ok {
						return "", errors.Errorf("no app named %q in release %q", app.Name, releaseName)
					}

				}

				values, err := appDeploy.GetResolvedValues(ctx)
				if err != nil {
					return "", errors.Wrap(err, "get release values")
				}

				valueYaml, err := values.Values.YAML()
				if err != nil {
					return "", errors.Wrap(err, "get release values yaml")
				}

				return valueYaml, nil
			}

			env1yaml, err := getValuesForEnv(env1)
			if err != nil {
				return errors.Errorf("error for env1 %q: %s", env1, err)
			}

			env2yaml, err := getValuesForEnv(env2)
			if err != nil {
				return errors.Errorf("error for env2 %q: %s", env2, err)
			}

			env1lines := strings.Split(env1yaml, "\n")
			env2lines := strings.Split(env2yaml, "\n")
			diffs := difflib.Diff(env1lines, env2lines)

			for _, diff := range diffs {
				fmt.Println(renderDiff(diff))
			}

			return nil

		},
	})

func diffStrings(a, b string) []difflib.DiffRecord {
	left := strings.Split(a, "\n")
	right := strings.Split(b, "\n")
	return difflib.Diff(left, right)
}

func renderDiff(diff difflib.DiffRecord) string {
	switch diff.Delta {
	case difflib.Common:
		return fmt.Sprintf("  %s", diff.Payload)
	case difflib.LeftOnly:
		return color.RedString("- %s", diff.Payload)
	case difflib.RightOnly:
		return color.GreenString("+ %s", diff.Payload)
	}
	panic(fmt.Sprintf("invalid delta %v", diff.Delta))
}

func getDeployableApps(b *bosun.Bosun, args []string) ([]*bosun.App, error) {
	fp := getFilterParams(b, args)
	apps, err := fp.GetAppsChain(fp.Chain().Including(filter.MustParse(bosun.LabelDeployable)))
	if err != nil {
		return nil, err
	}
	return apps, nil
}

func getReleaseBySlot(platform *bosun.Platform, slot string) (*bosun.ReleaseManifest, error) {

	switch slot {
	case bosun.SlotStable, bosun.SlotUnstable:
	default:
		return nil, errors.Errorf("invalid slot, wanted %s or %s, got %q", bosun.SlotStable, bosun.SlotUnstable, slot)
	}

	release, err := platform.GetReleaseManifestBySlot(slot)
	if err != nil {
		return nil, err
	}
	return release, nil
}
