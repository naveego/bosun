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
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/vbauerster/mpb/v4"
	"github.com/vbauerster/mpb/v4/decor"
	"log"
	"os"
	"sort"
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
		b := MustGetBosunNoEnvironment()

		t := tablewriter.NewWriter(os.Stdout)
		t.SetCenterSeparator("")
		t.SetColumnSeparator("")
		t.SetHeader([]string{"", "VERSION", "PATH"})
		platform, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		current, err := b.GetStableReleaseManifest()
		if err != nil {
			return err
		}

		for _, release := range platform.GetReleaseMetadataSortedByVersion(true) {
			version := release.Version.String()
			currentMark := ""
			if current != nil && release.Version == current.Version {
				currentMark = "*"
				version = color.GreenString(version)
			}

			t.Append([]string{currentMark, version, release.Description})
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

var releaseShowCmd = addCommand(releaseCmd, &cobra.Command{
	Use:     "show",
	Aliases: []string{"dump"},
	Short:   "Lists the apps in the current release.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosunNoEnvironment()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}
		rm := mustGetRelease(p, bosun.SlotStable)

		err = printOutput(rm)
		return err
	},
})

var releaseDotCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "dot",
	Short: "Prints a dot diagram of the release.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosunNoEnvironment()
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
	b := MustGetBosunNoEnvironment()
	p, err := b.GetCurrentPlatform()
	if err != nil {
		log.Fatal(err)
	}
	return b, p
}

var releaseAddCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "add {app}",
	Args:  cobra.ExactArgs(1),
	Short: "Adds an app to the current release.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosunNoEnvironment()

		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		r, err := p.GetCurrentRelease()
		if err != nil {
			return err
		}

		app, err := b.GetApp(args[0])
		if err != nil {
			return err
		}

		bump := viper.GetString(ArgReleaseAddBump)

		if bump == "" {
			bump = cli.RequestChoice("Choose a version bump for the app", "none", "patch", "minor", "major", "custom")
		}

		if bump == "custom" {
			bump = cli.RequestStringFromUser("Enter the version number to apply to the app")
		}

		branchDescription := viper.GetString(ArgReleaseAddBranch)
		if branchDescription == "" {
			if r.Slot == bosun.SlotUnstable {
				branchDescription = "the develop branch"
			}
		} else {
			branchDescription = "the branch " + branchDescription
		}

		confirmationMessage := fmt.Sprintf("You are adding app %s to the release %s, with a bump of %s and adding from %s. Do you want to continue?",
			app.Name, r, bump, branchDescription)
		confirmed := cli.RequestConfirmFromUser(confirmationMessage)
		if !confirmed {
			return nil
		}

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

		err = p.Save(b.NewContext())
		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().String(ArgReleaseAddBranch, "", "The branch to add the app from (defaults to the branch pattern for the slot).")
	cmd.Flags().String(ArgReleaseAddBump, "", "The version bump to apply to the app.")
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
	Use:           "update [apps...]",
	Short:         "Updates the current release by pulling in the manifests from the app repos.",
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
		release, err := p.GetCurrentRelease()
		if err != nil {
			return err
		}

		err = release.IsMutable()
		if err != nil {
			return err
		}

		apps, err := getKnownApps(b, args)
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

		var appNames []string
		for _, app := range apps {
			appNames = append(appNames, app.Name)
		}
		sort.Strings(appNames)

		fmt.Printf("Refreshing %d apps: %+v\n", len(apps), appNames)

		err = release.RefreshApps(ctx, apps...)
		if err != nil {
			return err
		}

		err = p.Save(ctx)

		return err
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
