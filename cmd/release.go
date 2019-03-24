package cmd

import (
	"fmt"
	"github.com/cheynewallace/tabby"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/git"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

func init() {

	releaseAddCmd.Flags().BoolP(ArgAppAll, "a", false, "Apply to all known microservices.")
	releaseAddCmd.Flags().StringSliceP(ArgAppLabels, "i", []string{}, "Apply to microservices with the provided labels.")

	releaseCmd.AddCommand(releaseUseCmd)

	releaseCmd.AddCommand(releaseAddCmd)

	releaseCmd.PersistentFlags().StringSlice(ArgInclude, []string{}, `Only include apps which match the provided selectors. --include trumps --exclude.".`)
	releaseCmd.PersistentFlags().StringSlice(ArgExclude, []string{}, `Don't include apps which match the provided selectors.".`)
	rootCmd.AddCommand(releaseCmd)
}

// releaseCmd represents the release command
var releaseCmd = &cobra.Command{
	Use:     "release",
	Aliases: []string{"rel", "r"},
	Short:   "ReleaseConfig commands.",
}

var releaseListCmd = addCommand(releaseCmd, &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "Lists known releases.",
	Run: func(cmd *cobra.Command, args []string) {
		b := mustGetBosun()

		current, _ := b.GetCurrentRelease()
		t := tabby.New()
		t.AddHeader("RELEASE", "VERSION", "PATH")
		releases := b.GetReleaseConfigs()
		for _, release := range releases {
			name := release.Name
			if current != nil && release.Name == current.Name {
				name = fmt.Sprintf("* %s", name)
			}
			t.AddLine(name, release.Version, release.FromPath)
		}
		t.Print()

		if current == nil {
			color.Red("No current release selected (use `bosun release use {name}` to select one).")
		} else {
			color.White("(* indicates currently active release)")
		}
	},
})

var releaseShowCmd = addCommand(releaseCmd, &cobra.Command{
	Use:     "list-apps",
	Aliases: []string{"la"},
	Short:   "Lists the apps in the current release.",
	Run: func(cmd *cobra.Command, args []string) {
		b := mustGetBosun()
		r := mustGetCurrentRelease(b)

		switch viper.GetString(ArgGlobalOutput) {
		case OutputYaml:
			fmt.Println(MustYaml(r))
		default:

			t := tabby.New()
			t.AddHeader("APP", "VERSION", "REPO")
			for _, app := range r.AppReleases.GetAppsSortedByName() {
				t.AddLine(app.Name, app.Version, app.Repo)
			}
			t.Print()

		}
	},
})

var releaseDiffCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "diff",
	Short: "Reports on the changes deploying the release will inflict on the current environment.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()
		r := mustGetCurrentRelease(b)
		if len(viper.GetStringSlice(ArgAppLabels)) == 0 && len(args) == 0 {
			viper.Set(ArgAppAll, true)
		}

		apps, err := getAppRepos(b, args)
		if err != nil {
			return err
		}
		requestedApps := map[string]bool{}
		for _, app := range apps {
			requestedApps[app.Name] = true
		}

		total := len(requestedApps)
		complete := 0

		appReleases := r.AppReleases
		wg := new(sync.WaitGroup)
		wg.Add(len(appReleases))
		for _, appRelease := range appReleases {
			if !requestedApps[appRelease.Name] {
				continue
			}
			go func(appRelease *bosun.AppRelease) {
				defer wg.Done()

				ctx := b.NewContext().WithAppRelease(appRelease)
				values, err := appRelease.GetReleaseValues(ctx)
				if err != nil {
					ctx.Log.WithError(err).Error("Could not create values map for app release.")
					return
				}

				ctx = ctx.WithReleaseValues(values)
				err = appRelease.LoadActualState(ctx, true)
				if err != nil {
					ctx.Log.WithError(err).Error("Could not load actual state.")
					return
				}
				complete += 1
				color.White("Loaded %s (%d/%d)", appRelease.Name, complete, total)
				wg.Done()
			}(appRelease)
		}
		wg.Wait()

		for _, appRelease := range appReleases {
			color.Blue("%s\n", appRelease.Name)
			if appRelease.ActualState.Diff == "" {
				color.White("No diff detected.")
			} else {
				color.Yellow("Diff:\n")
				fmt.Println(appRelease.ActualState.Diff)
				fmt.Println()
			}
		}

		color.Blue("SUMMARY:\n")
		for _, appRelease := range appReleases {
			color.Blue(appRelease.Name)
			if appRelease.ActualState.Diff == "" {
				color.White("No diff detected.\n")
			} else {
				fmt.Println("Has changes (see above).")
			}
		}

		return nil
	},
})

var releaseShowValuesCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "show-values {app}",
	Args:  cobra.ExactArgs(1),
	Short: "Shows the values which will be used for a release.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()
		r := mustGetCurrentRelease(b)

		appRelease := r.AppReleases[args[0]]
		if appRelease == nil {
			return errors.Errorf("app %q not in this release", args[0])
		}

		ctx := b.NewContext()
		values, err := appRelease.GetReleaseValues(ctx)
		if err != nil {
			return err
		}

		yml, err := yaml.Marshal(values)
		if err != nil {
			return err
		}

		fmt.Println(string(yml))

		return nil
	},
})

var releaseUseCmd = &cobra.Command{
	Use:   "use {name}",
	Args:  cobra.ExactArgs(1),
	Short: "Sets the release which release commands will work against.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()
		err := b.UseRelease(args[0])
		if err != nil {
			return err
		}
		if err == nil {
			err = b.Save()
		}
		return err
	},
}

var releaseCreateCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "create {name} {path}",
	Args:  cobra.ExactArgs(2),
	Short: "Creates a new release.",
	Long: `The name will be used to refer to the release.
The release file will be stored at the path.

The --patch flag changes the behavior of "bosun release add {app}".
If the --patch flag is set when the release is created, the add command
will check check if the app was in a previous release with the same 
major.minor version as this release. If such a branch is found,
the release branch will be created from the previous release branch,
rather than being created off of master.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, path := args[0], args[1]

		version := viper.GetString(ArgReleaseCreateVersion)
		if version == "" {
			semverRaw := regexp.MustCompile(`[^\.0-9]`).ReplaceAllString(name, "")
			semverSegs := strings.Split(semverRaw, ".")
			if len(semverSegs) < 3 {
				semverSegs = append(semverSegs, "0")
			}
			version = strings.Join(semverSegs, ".")
		}

		c := bosun.File{
			FromPath: path,
			Releases: []*bosun.ReleaseConfig{
				&bosun.ReleaseConfig{
					Name:    name,
					Version: version,
					IsPatch:viper.GetBool(ArgReleaseCreatePatch),
				},
			},
		}

		err := c.Save()
		if err != nil {
			return err
		}

		b := mustGetBosun()

		err = b.UseRelease(name)

		if err != nil {
			// release path is not already imported
			b.AddImport(path)
			err = b.Save()
			if err != nil {
				return err
			}
			b = mustGetBosun()
			err = b.UseRelease(name)
			if err != nil {
				// this shouldn't happen...
				return err
			}
		}

		err = b.Save()

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(ArgReleaseCreatePatch, false, "Set if this is a patch release.")
	cmd.Flags().String(ArgReleaseCreateVersion, "", "Version of this release (will attempt to derive from name if not provided).")
})

const (
	ArgReleaseCreateVersion = "version"
	ArgReleaseCreatePatch = "patch"
	ArgReleaseCreateParentVersion = "parent-version"
)

var releaseAddCmd = &cobra.Command{
	Use:   "add [names...]",
	Short: "Adds one or more apps to a release.",
	Long:  "Provide app names or use labels.",
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		b := mustGetBosun()
		release := mustGetCurrentRelease(b)

		apps, err := getAppRepos(b, args)
		if err != nil {
			return err
		}

		ctx := b.NewContext().WithRelease(release)

		for _, app := range apps {

			delete(release.Exclude, app.Name)
			_, ok := release.AppReleaseConfigs[app.Name]
			if ok {
				pkg.Log.Warnf("Overwriting existing app %q.", app.Name)
			} else {
				ctx.Log.Infof("Adding app %q", app.Name)
			}

			release.AppReleaseConfigs[app.Name], err = app.GetAppReleaseConfig(ctx)

			if err != nil {
				return errors.Errorf("could not make release for app %q: %s", app.Name, err)
			}
		}

		err = release.IncludeDependencies(ctx)
		if err != nil {
			return err
		}

		err = release.Parent.Save()
		return err
	},
}

var releaseRemoveCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "remove [names...]",
	Short: "Removes one or more apps from a release.",
	Long:  "Provide app names or use labels.",
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		b := mustGetBosun()
		release := mustGetCurrentRelease(b)

		apps, err := getAppRepos(b, args)
		if err != nil {
			return err
		}

		for _, app := range apps {
			delete(release.AppReleaseConfigs, app.Name)
		}

		err = release.Parent.Save()
		return err
	},
})

var releaseExcludeCmd = addCommand(releaseCmd, &cobra.Command{
	Use:   "exclude [names...]",
	Short: "Excludes and removes one or more apps from a release.",
	Long: "Provide app names or use labels. The matched apps will be removed " +
		"from the release and will not be re-added even if apps which depend on " +
		"them are added or synced. If the app is explicitly added it will be " +
		"removed from the exclude list.",
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		b := mustGetBosun()
		release := mustGetCurrentRelease(b)

		apps, err := getAppRepos(b, args)
		if err != nil {
			return err
		}

		for _, app := range apps {
			delete(release.AppReleaseConfigs, app.Name)
			release.Exclude[app.Name] = true
		}

		err = release.Parent.Save()
		return err
	},
})

var releaseValidateCmd = addCommand(releaseCmd, &cobra.Command{
	Use:           "validate",
	Short:         "Validates the release.",
	Long:          "Validation checks that all apps in this release have a published chart and docker image for this release.",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		b := mustGetBosun()
		release := mustGetCurrentRelease(b)

		ctx := b.NewContext()

		return validateRelease(b, ctx, release)
	},
})

func validateRelease(b *bosun.Bosun, ctx bosun.BosunContext, release *bosun.Release) error {
	w := new(strings.Builder)
	hasErrors := false

	apps := release.AppReleases.GetAppsSortedByName()
	p := NewProgressBar(len(apps))

	for _, app := range apps {
		if app.Excluded {
			continue
		}

		p.Add(0, app.Name)

		errs := app.Validate(ctx)

		p.Add(1, app.Name)

		colorHeader.Fprintf(w, "%s ", app.Name)

		if len(errs) == 0 {
			colorOK.Fprintf(w, "OK\n")
		} else {
			fmt.Fprintln(w)
			for _, err := range errs {
				hasErrors = true
				colorError.Fprintf(w, " - %s\n", err)
			}
		}
	}

	fmt.Println()
	fmt.Println(w.String())

	if hasErrors {
		return errors.New("Some apps are invalid.")
	}

	return nil
}

var releaseSyncCmd = addCommand(releaseCmd, &cobra.Command{
	Use:           "sync",
	Short:         "Pulls the latest commits for every app in the release, then updates the values in the release entry.",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		b := mustGetBosun()
		release := mustGetCurrentRelease(b)
		ctx := b.NewContext()

		appReleases := mustGetAppReleases(b, args)

		err := processAppReleases(b, ctx, appReleases, func(appRelease *bosun.AppRelease) error {
			ctx = ctx.WithAppRelease(appRelease)
			if appRelease.AppRepo == nil {
				ctx.Log.Warn("AppRepo not found.")
			}

			repo := appRelease.AppRepo
			if !repo.BranchForRelease {
				return nil
			}

			if err := repo.FetchRepo(ctx); err != nil {
				return errors.Wrap(err, "fetch")
			}

			g, _ := git.NewGitWrapper(repo.FromPath)

			commits, err := g.Log("--oneline", fmt.Sprintf("%s..origin/%s", appRelease.Commit, appRelease.Branch))
			if err != nil {
				return errors.Wrap(err, "check for missed commits")
			}
			if len(commits) == 0 {
				return nil
			}

			ctx.Log.Warn("Release branch has had commits since app was added to release. Will attempt to merge before updating release.")

			currentBranch := appRelease.AppRepo.GetBranch()
			if currentBranch != appRelease.Branch {
				dirtiness, err := g.Exec("status", "--porcelain")
				if err != nil {
					return errors.Wrap(err, "check if branch is dirty")
				}
				if len(dirtiness) > 0 {
					return errors.New("app is on branch %q, not release branch %q, and has dirty files, so we can't switch to the release branch")
				}
				ctx.Log.Warnf("Checking out branch %s")
				_, err = g.Exec("checkout", appRelease.Branch)
				if err != nil {
					return errors.Wrap(err, "check out release branch")
				}

				_, err = g.Exec("merge", fmt.Sprintf("origin/%s", appRelease.Branch))
				if err != nil {
					return errors.Wrap(err, "merge release branch")
				}
			}

			err = release.IncludeApp(ctx, appRelease.AppRepo)
			if err != nil {
				return errors.Wrap(err, "update failed")
			}

			return nil
		})

		if err != nil {
			return err
		}

		err = release.Parent.Save()

		return err
	},
})

func processAppReleases(b *bosun.Bosun, ctx bosun.BosunContext, appReleases []*bosun.AppRelease, fn func(a *bosun.AppRelease) error) error {

	var included []*bosun.AppRelease
	for _, ar := range appReleases {
		if !ar.Excluded {
			included = append(included, ar)
		}
	}
	p := NewProgressBar(len(included))

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

		appReleases := mustGetAppReleases(b, args)

		for _, appRelease := range appReleases {

			ctx = ctx.WithAppRelease(appRelease)
			for _, action := range appRelease.Actions {
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
})

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

		if viper.GetBool(ArgReleaseSkipValidate) {
			ctx.Log.Warn("Validation disabled.")
		} else {
			ctx.Log.Info("Validating...")
			err := validateRelease(b, ctx, release)
			if err != nil {
				return err
			}
		}

		err := release.Deploy(ctx)

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(ArgReleaseSkipValidate, false, "Skips running validation before deploying the release.")
})

var releaseMergeCmd = addCommand(releaseCmd, &cobra.Command{
	Use:           "merge [apps...]",
	Short:         "Merges the release branch back to master for each app in the release (or the listed apps)",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		b := mustGetBosun()
		ctx := b.NewContext()

		release, err := b.GetCurrentRelease()
		if err != nil {
			return err
		}
		appReleases := mustGetAppReleases(b, args)

		releaseBranch := fmt.Sprintf("release/%s", release.Name)

		for _, appRelease := range appReleases {

			ctx = ctx.WithAppRelease(appRelease)
			appRepo := appRelease.AppRepo
			if !appRepo.IsRepoCloned() {
				ctx.Log.Error("Repo is not cloned, cannot merge.")
				continue
			}

			localRepoPath, _ := appRepo.GetLocalRepoPath()
			ctx.Log.Info("Creating pull request.")
			prNumber, err := GitPullRequestCommand{
				LocalRepoPath: localRepoPath,
				Base: "master",
				FromBranch: releaseBranch,
			}.Execute()
			if err != nil {
				ctx.Log.WithError(err).Error("Could not create pull request.")
				continue
			}

			ctx.Log.Info("Accepting pull request.")
			err = GitAcceptPRCommand{
				PRNumber:prNumber,
				RepoDirectory: filepath.Dir(appRepo.FromPath),
				DoNotMergeBaseIntoBranch: true,
			}.Execute()

			if err != nil {
				ctx.Log.WithError(err).Error("Could not accept pull request.")
				continue
			}

			ctx.Log.Info("Merged back to master.")
		}

		return nil
	},
})

const ArgReleaseSkipValidate = "skip-validation"
