package cmd

import (
	"fmt"
	"github.com/aryann/difflib"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/git"
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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {

		releaseOverride := viper.GetString(ArgReleaseName)
		if releaseOverride != "" {
			b := mustGetBosun()
			currentReleaseMetadata, err := b.GetCurrentReleaseMetadata()
			if err == nil {
				originalCurrentRelease = &currentReleaseMetadata.Name
			}
			err = b.UseRelease(releaseOverride)
			if err != nil {
				return errors.Wrap(err, "setting release override")
			}
			err = b.Save()
			if err != nil {
				return errors.Wrap(err, "saving release override")
			}
			b.NewContext().Log.Infof("Using release %q for this command (original release was %q).", releaseOverride, currentReleaseMetadata.Name)
		}
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		if originalCurrentRelease != nil {
			b := mustGetBosun()
			err := b.UseRelease(*originalCurrentRelease)
			if err != nil {
				return errors.Wrap(err, "resetting current release")
			}
			err = b.Save()
			if err != nil {
				return errors.Wrap(err, "saving release reset")
			}
			b.NewContext().Log.Infof("Reset release to %q.", *originalCurrentRelease)
		}
		return nil
	},
}, func(cmd *cobra.Command) {
	cmd.PersistentFlags().StringP(ArgReleaseName, "r", "", "The release to use for this command (overrides current release set with `release use {name}`).")
})

var originalCurrentRelease *string

const (
	ArgReleaseName = "release"
)

var releaseListCmd = addCommand(releaseCmd, &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "Lists known releases.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()

		t := tablewriter.NewWriter(os.Stdout)
		t.SetCenterSeparator("")
		t.SetColumnSeparator("")
		t.SetHeader([]string{"", "RELEASE", "VERSION", "PATH"})
		platform, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		current, err := b.GetCurrentReleaseMetadata()

		for _, release := range platform.GetReleaseMetadataSortedByVersion(true, true) {
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
	Use:   "replan",
	Short: "Returns the release to the planning stage.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b, p := getReleaseCmdDeps()
		rm, err := b.GetCurrentReleaseMetadata()
		if err != nil {
			return err
		}
		ctx := b.NewContext()
		_, err = p.RePlanRelease(ctx, rm)
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
		b := mustGetBosun()
		rm, err := b.GetCurrentReleaseManifest(false)
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
		b := mustGetBosun()
		rm, err := b.GetCurrentReleaseManifest(true)
		if err != nil {
			return err
		}

		out := rm.ExportDiagram()
		fmt.Println(out)
		return nil
	},
})

func getReleaseCmdDeps() (*bosun.Bosun, *bosun.Platform) {
	b := mustGetBosun()
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
		// 			ctx.Log.WithError(err).Error("Could not create values map for app release.")
		// 			return
		// 		}
		//
		// 		ctx = ctx.WithPersistableValues(values)
		// 		err = appRelease.LoadActualState(ctx, true)
		// 		if err != nil {
		// 			ctx.Log.WithError(err).Error("Could not load actual state.")
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
		releaseManifest := mustGetCurrentRelease(b)

		appManifest := releaseManifest.AppManifests[args[0]]
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

var releaseUseCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "use {name}",
	Args:  cobra.ExactArgs(1),
	Short: "Sets the release which release commands will work against.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()

		err := b.UseRelease(args[0])
		if err != nil {
			return err
		}

		err = b.Save()
		return err
	},
})

var releaseDeleteCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "delete [name]",
	Args:  cobra.ExactArgs(1),
	Short: "Deletes a release.",

	RunE: func(cmd *cobra.Command, args []string) error {
		b, p := getReleaseCmdDeps()
		ctx := b.NewContext()
		return p.DeleteRelease(ctx, args[0])
	},
})

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
// 		b := mustGetBosun()
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
		b := mustGetBosun()
		release := mustGetCurrentRelease(b)

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
	withFilteringFlags)

func validateDeploy(b *bosun.Bosun, ctx bosun.BosunContext, release *bosun.Deploy) error {

	hasErrors := false

	apps := release.AppDeploys

	var wg sync.WaitGroup
	// pass &wg (optional), so p will wait for it eventually
	p := mpb.New(mpb.WithWaitGroup(&wg))

	errmu := new(sync.Mutex)

	errs := map[string][]error{}
	start := time.Now()
	for i := range apps {
		app := apps[i]
		if app.Excluded {
			continue
		}
		wg.Add(1)
		bar := p.AddBar(100, mpb.PrependDecorators(decor.Name(app.Name)),
			mpb.AppendDecorators(decor.OnComplete(decor.EwmaETA(decor.ET_STYLE_GO, 60), "done")))

		go func() {
			err := app.Validate(ctx)
			if err != nil {
				errmu.Lock()
				defer errmu.Unlock()
				errs[app.Name] = err
			}
			bar.IncrBy(100, time.Since(start))
			wg.Done()
		}()

	}
	p.Wait()

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

//
// var releaseSyncCmd = addCommand(releaseCmd, &cobra.Command{
// 	Use:           "sync",
// 	Short:         "Pulls the latest commits for every app in the release, then updates the values in the release entry.",
// 	SilenceErrors: true,
// 	SilenceUsage:  true,
// 	RunE: func(cmd *cobra.Command, args []string) error {
// 		viper.BindPFlags(cmd.Flags())
// 		b := mustGetBosun()
// 		release := mustGetCurrentRelease(b)
// 		ctx := b.NewContext()
//
// 		appReleases := getFilterParams(b, args).MustGetAppDeploys()
//
// 		err := processAppReleases(b, ctx, appReleases, func(appRelease *bosun.AppDeploy) error {
// 			ctx = ctx.WithAppDeploy(appRelease)
// 			if appRelease.App == nil {
// 				ctx.Log.Warn("App not found.")
// 			}
//
// 			repo := appRelease.App
// 			if !repo.BranchForRelease {
// 				return nil
// 			}
//
// 			if err := repo.Repo.Fetch(ctx); err != nil {
// 				return errors.Wrap(err, "fetch")
// 			}
//
// 			g, _ := git.NewGitWrapper(repo.FromPath)
//
// 			commits, err := g.Log("--oneline", fmt.Sprintf("%s..origin/%s", appRelease.Commit, appRelease.Branch))
// 			if err != nil {
// 				return errors.Wrap(err, "check for missed commits")
// 			}
// 			if len(commits) == 0 {
// 				return nil
// 			}
//
// 			ctx.Log.Warn("Deploy branch has had commits since app was added to release. Will attempt to merge before updating release.")
//
// 			currentBranch := appRelease.App.GetBranchName()
// 			if currentBranch != appRelease.Branch {
// 				dirtiness, err := g.Exec("status", "--porcelain")
// 				if err != nil {
// 					return errors.Wrap(err, "check if branch is dirty")
// 				}
// 				if len(dirtiness) > 0 {
// 					return errors.New("app is on branch %q, not release branch %q, and has dirty files, so we can't switch to the release branch")
// 				}
// 				ctx.Log.Warnf("Checking out branch %s")
// 				_, err = g.Exec("checkout", appRelease.Branch.String())
// 				if err != nil {
// 					return errors.Wrap(err, "check out release branch")
// 				}
//
// 				_, err = g.Exec("merge", fmt.Sprintf("origin/%s", appRelease.Branch))
// 				if err != nil {
// 					return errors.Wrap(err, "merge release branch")
// 				}
// 			}
//
// 			err = release.MakeAppAvailable(ctx, appRelease.App)
// 			if err != nil {
// 				return errors.Wrap(err, "update failed")
// 			}
//
// 			return nil
// 		})
//
// 		if err != nil {
// 			return err
// 		}
//
// 		err = release.Parent.Save()
//
// 		return err
// 	},
// })

func processAppReleases(b *bosun.Bosun, ctx bosun.BosunContext, appReleases []*bosun.AppDeploy, fn func(a *bosun.AppDeploy) error) error {

	var included []*bosun.AppDeploy
	for _, ar := range appReleases {
		if !ar.Excluded {
			included = append(included, ar)
		}
	}
	p := util.NewProgressBar(len(included))

	for _, appRelease := range included {
		p.Add(1, appRelease.Name)
		err := fn(appRelease)
		if err != nil {
			return errors.Errorf("%s failed: %s", appRelease.Name, err)
		}
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
		b := mustGetBosun()
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
						ctx.Log.WithError(err).Error("Test failed.")
					}
				}
			}
		}

		return nil
	},
}, withFilteringFlags)

var releaseDeployCmd = addCommand(releaseCmd, &cobra.Command{
	Use:           "deploy",
	Short:         "Deploys the release.",
	Long:          "Deploys the current release to the current environment.",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := mustGetBosun()
		release := mustGetCurrentRelease(b)
		ctx := b.NewContext()
		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

		valueSets, err := getValueSetSlice(b, b.GetCurrentEnvironment())
		if err != nil {
			return err
		}

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

		color.Yellow("About to deploy the following apps:")
		for _, name := range util.SortedKeys(deploy.AppDeploys) {
			app := deploy.AppDeploys[name]
			fmt.Printf("- %s: %s (tag %s)\n", name, app.Version, app.GetImageTag())
		}

		if !confirm("Is this what you expected") {
			return errors.New("Deploy cancelled.")
		}

		if viper.GetBool(ArgReleaseSkipValidate) {
			ctx.Log.Warn("Validation disabled.")
		} else {
			ctx.Log.Info("Validating...")
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
},
	withFilteringFlags,
	withValueSetFlags)

var releaseMergeCmd = addCommand(releaseCmd, &cobra.Command{
	Use:           "merge [apps...]",
	Short:         "Merges the release branch back to master for each app in the release (or the listed apps)",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := mustGetBosun()
		ctx := b.NewContext()

		force := viper.GetBool(ArgGlobalForce)

		release, err := b.GetCurrentReleaseManifest(false)
		if err != nil {
			return err
		}

		repoToManifestMap := make(map[*bosun.Repo]*bosun.AppManifest)

		releaseBranch := fmt.Sprintf("release/%s", release.Version)

		appsNames := map[string]bool{}
		for _, appName := range args {
			appsNames[appName] = true
		}

		for _, appDeploy := range release.AppManifests {

			if len(args) > 0 && !appsNames[appDeploy.Name] {
				// not listed as an arg
				continue
			}

			app, err := b.GetApp(appDeploy.Name)

			if err != nil {
				ctx.Log.WithError(err).Errorf("App repo %s (%s) not available.", appDeploy.Name, appDeploy.Repo)
				continue
			}

			manifest, err := app.GetManifest(ctx)
			if err != nil {
				ctx.Log.WithError(err).Errorf("App manifest %s (%s) not available.", appDeploy.Name, appDeploy.Repo)
				continue
			}

			if !app.BranchForRelease {
				ctx.Log.Warnf("App repo (%s) for app %s is not branched for release.", app.RepoName, app.Name)
				continue
			}

			if appDeploy.Branch != releaseBranch {
				ctx.Log.Warnf("App repo (%s) does not have a release branch for release %s (%s), nothing to merge.", app.RepoName, release.Name, release.Version)
				continue
			}

			if !app.IsRepoCloned() {
				ctx.Log.Errorf("App repo (%s) for app %s is not cloned, cannot merge.", app.RepoName, app.Name)
				continue
			}

			repoToManifestMap[app.Repo] = manifest
		}

		if len(repoToManifestMap) == 0 {
			return errors.New("no apps found")
		}

		fmt.Println("About to merge back to master:")
		for _, appDeploy := range repoToManifestMap {
			fmt.Printf("- %s: %s (tag %s)\n", appDeploy.Name, appDeploy.Version, appDeploy.Branch)
		}

		if !confirm("Is this what you expected") {
			return errors.New("Merge cancelled.")
		}

		for repo, appDeploy := range repoToManifestMap {

			log := ctx.Log.WithField("repo", repo.Name)

			localRepo := repo.LocalRepo

			if localRepo.IsDirty() {
				log.Errorf("Repo at %s is dirty, cannot merge.", localRepo.Path)
				continue
			}

			repoDir := localRepo.Path

			g, _ := git.NewGitWrapper(repoDir)

			err := g.FetchAll()
			if err != nil {
				return err
			}

			releaseBranch := appDeploy.Branch

			log.Info("Checking out release branch...")

			_, err = g.Exec("checkout", releaseBranch)
			if err != nil {
				return errors.Errorf("checkout %s: %s", repoDir, releaseBranch)
			}

			log.Info("Pulling release branch...")
			err = g.Pull()
			if err != nil {
				return err
			}

			log.Info("Tagging release branch...")
			tagArgs := []string{"tag", fmt.Sprintf("%s-%s", appDeploy.Version, release.Version)}
			if force {
				tagArgs = append(tagArgs, "--force")
			}

			_, err = g.Exec(tagArgs...)
			if err != nil {
				log.WithError(err).Warn("Could not tag repo, skipping merge. Set --force flag to force tag.")
			} else {
				log.Info("Pushing tags...")

				pushArgs := []string{"push", "--tags"}
				if force {
					pushArgs = append(pushArgs, "--force")
				}
				_, err = g.Exec(pushArgs...)
				if err != nil {
					return errors.Errorf("push tags: %s", err)
				}
			}

			log.Info("Checking for changes...")

			diff, err := g.Exec("log", "origin/master..origin/"+releaseBranch, "--oneline")
			if err != nil {
				return errors.Errorf("find diffs: %s", err)
			}

			if len(diff) == 0 {
				log.Info("No diffs found between release branch and master...")
			} else {

				log.Info("Deploy branch has diverged from master, will merge back...")

				log.Info("Creating pull request.")
				_, prNumber, err := GitPullRequestCommand{
					LocalRepoPath: repoDir,
					Base:          "master",
					FromBranch:    releaseBranch,
				}.Execute()
				if err != nil {
					ctx.Log.WithError(err).Error("Could not create pull request.")
					continue
				}

				log.Info("Accepting pull request.")
				err = GitAcceptPRCommand{
					PRNumber:                 prNumber,
					RepoDirectory:            repoDir,
					DoNotMergeBaseIntoBranch: true,
				}.Execute()

				if err != nil {
					ctx.Log.WithError(err).Error("Could not accept pull request.")
					continue
				}

				log.Info("Merged back to master.")
			}

		}

		return nil
	},
}, withFilteringFlags)

const ArgReleaseSkipValidate = "skip-validation"

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
			b := mustGetBosun()
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
					releaseManifest, err := p.GetReleaseManifestByName(releaseName, true)

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
