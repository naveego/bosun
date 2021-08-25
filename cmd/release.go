package cmd

import (
	"fmt"
	"github.com/aryann/difflib"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/util/stringsn"
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
		rm := mustGetRelease(p, bosun.SlotStable)

		previousRelease := mustGetRelease(p, bosun.SlotStable, bosun.SlotStable)
		for _, app := range rm.AppMetadata {
			if previousApp, ok := previousRelease.AppMetadata[app.Name]; ok {
				app.PreviousVersion = &previousApp.Version
			}
		}

		err = printOutput(rm)
		return err
	},
})

var releaseShowPreviousCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "show-previous",
	Short: "Shows information about the previous release.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}
		r, err := p.GetPreviousRelease()
		if err != nil {
			return err
		}

		fmt.Printf("Name: %s\n", r.Name)
		fmt.Printf("Version: %s\n", r.Version)

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
	Use:   "add {stable|unstable} [apps...]",
	Args:  cobra.MinimumNArgs(1),
	Short: "Adds an app to a release.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()

		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		r := mustGetRelease(p, args[0], bosun.SlotUnstable, bosun.SlotStable)

		apps := mustGetKnownApps(b, args[1:])
		bump := viper.GetString(ArgReleaseAddBump)
		for _, app := range apps {

			ctx := b.NewContext().WithApp(app)

			ctx.Log().Info("Adding app to release...")

			branch := viper.GetString(ArgReleaseAddBranch)

			appManifest, prepareErr := r.PrepareAppForRelease(ctx, app, semver.Bump(bump), branch)
			if prepareErr != nil {
				return prepareErr
			}

			addErr := r.AddOrReplaceApp(appManifest, true)
			if addErr != nil {
				return addErr
			}
		}

		err = p.Save(b.NewContext())
		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().String(ArgReleaseAddBranch, "", "The branch to add the app from (defaults to the branch pattern for the slot).")
	cmd.Flags().String(ArgReleaseAddBump, "none", "The version bump to apply to the app.")
}, withFilteringFlags)

var releaseReloadCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "reload {stable|unstable} [apps...]",
	Args:  cobra.MinimumNArgs(1),
	Short: "Reloads an app (or all apps) into a release from the location declared in the manifest.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()

		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		r := mustGetRelease(p, args[0], bosun.SlotUnstable, bosun.SlotStable)

		requestedApps := args[1:]

		ctx := b.NewContext()

		for appName, appMetadata := range r.AppMetadata {

			if len(requestedApps) > 0 && !stringsn.Contains(appName, requestedApps) {
				ctx.Log().Info("Skipping app %s because it wasn't requested.", appName)
				continue
			}
			ctx.Log().Info("Reloading app %s.", appName)

			app, appErr := b.ProvideApp(bosun.AppProviderRequest{
				Name:   appName,
				Branch: appMetadata.Branch,
			})

			if appErr != nil {
				ctx.Log().WithError(err).Warnf("Couldn't provide app %s from branch %s", appName, appMetadata.Branch)
			}

			manifest, appErr := app.GetManifest(ctx)
			if appErr != nil {
				ctx.Log().WithError(err).Warnf("Couldn't get manifest from app %s from branch %s", appName, appMetadata.Branch)
			}

			appErr = r.AddOrReplaceApp(manifest, false)
			if appErr != nil {
				ctx.Log().WithError(err).Warnf("Couldn't add app to release for app %s from branch %s", appName, appMetadata.Branch)
			}
		}

		err = p.Save(b.NewContext())
		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().String(ArgReleaseAddBranch, "", "The branch to add the app from (defaults to the branch pattern for the slot).")
	cmd.Flags().String(ArgReleaseAddBump, "none", "The version bump to apply to the app.")
}, withFilteringFlags)

const (
	ArgReleaseAddBranch = "branch"
	ArgReleaseAddBump   = "bump"
)

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
			SharedDeploySettings: bosun.SharedDeploySettings{
				Environment: ctx.Environment(),
			},
			ValueSets: valueSets,
			Manifest:  release,
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

	err := command.NewShellExe("helm", "repo", "update").RunE()
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
			validateErr := app.Validate(ctx)
			if validateErr != nil {
				errmu.Lock()
				defer errmu.Unlock()
				errs[app.Name] = validateErr
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
			for _, appErr := range appErrs {
				state = append(state, colorError.Sprint(appErr))
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
			SharedDeploySettings: bosun.SharedDeploySettings{
				Environment: ctx.Environment(),
				Recycle:     viper.GetBool(ArgReleaseRecycle),
			},
			ValueSets:       valueSets,
			Manifest:        release,
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
		for _, app := range deploy.AppDeploys {
			fmt.Printf("- %s: %s (tag %s) => namespace:%s \n", app.Name, app.AppConfig.Version, deploySettings.GetImageTag(app.AppManifest.AppMetadata), app.Namespace)
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

var releaseUpdateCmd = addCommand(releaseCmd, &cobra.Command{
	Use:           "update {stable|unstable} [apps...]",
	Short:         "Updates the release with the correct values from the apps in it.",
	Args:          cobra.MinimumNArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := MustGetBosun(cli.Parameters{NoEnvironment: true})
		ctx := b.NewContext()

		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}
		release, err := p.GetReleaseManifestBySlot(args[0])
		if err != nil {
			return err
		}

		err = release.IsMutable()
		if err != nil {
			return err
		}

		apps, err := getKnownApps(b, args[1:])
		if err != nil {
			return err
		}

		confirmMsg := "OK to refresh all apps"
		if len(apps) > 0 {
			var appNames []string
			for _, app := range apps {
				appNames = append(appNames, app.Name)
			}
			confirmMsg = fmt.Sprintf("OK to refresh release %s for these apps: %s", release, strings.Join(appNames, ", "))
		} else {
			return errors.Errorf("no apps matched %+v", args[1:])
		}
		if len(apps) > 1 {
			confirm(confirmMsg)
		}

		fmt.Printf("Refreshing %d apps: %+v\n", len(apps), args[1:])

		fromBranch := viper.GetString(argReleaseUpdateBranch)

		err = release.RefreshApps(ctx, fromBranch, apps...)
		if err != nil {
			return err
		}

		err = p.Save(ctx)

		return err
	},
}, withFilteringFlags,

	func(cmd *cobra.Command) {
		cmd.Flags().String(argReleaseUpdateBranch, "", "The branch to pull the app from (defaults to using the release branch for the app).")
	})

const (
	argReleaseUpdateBranch = "branch"
)

var releaseChangelogCmd = addCommand(releaseCmd, &cobra.Command{
	Use:           "change-log",
	Short:         "Outputs the changelog for the release.",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {

		return errors.New("not implemented")
		/*		viper.BindPFlags(cmd.Flags())

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

				return nil*/
	},
}, withFilteringFlags)

const ArgReleaseSkipValidate = "skip-validation"
const ArgReleaseRecycle = "recycle"

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
