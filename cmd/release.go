package cmd

import (
	"fmt"
	"github.com/cheynewallace/tabby"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"strings"
)

func init() {

	releaseCmd.AddCommand(releaseCreateCmd)

	releaseAddCmd.Flags().BoolP(ArgAppAll, "a", false, "Apply to all known microservices.")
	releaseAddCmd.Flags().StringSliceP(ArgAppLabels, "i", []string{}, "Apply to microservices with the provided labels.")

	releaseCmd.AddCommand(releaseUseCmd)

	releaseCmd.AddCommand(releaseAddCmd)

	rootCmd.AddCommand(releaseCmd)
}

// releaseCmd represents the release command
var releaseCmd = &cobra.Command{
	Use:     "release",
	Aliases: []string{"rel", "r"},
	Short:   "ReleaseConfig commands.",
}

func addCommand(parent *cobra.Command, child *cobra.Command, flags ...func(cmd *cobra.Command)) *cobra.Command {
	for _, fn := range flags {
		fn(child)
	}
	parent.AddCommand(child)

	return child
}

var releaseListCmd = addCommand(releaseCmd, &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "Lists known releases.",
	Run: func(cmd *cobra.Command, args []string) {
		b := mustGetBosun()

		current, _ := b.GetCurrentRelease()
		t := tabby.New()
		t.AddHeader("RELEASE", "PATH")
		releases := b.GetReleaseConfigs()
		for _, release := range releases {
			name := release.Name
			if release.Name == current.Name {
				name = fmt.Sprintf("* %s", name)
			}
			t.AddLine(name, release.FromPath)
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
	Use:     "show",
	Aliases: []string{"ls"},
	Short:   "Lists known releases.",
	Run: func(cmd *cobra.Command, args []string) {
		b := mustGetBosun()
		r := mustGetCurrentRelease(b)

		switch viper.GetString(ArgGlobalOutput) {
		case OutputYaml:
			fmt.Println(MustYaml(r))
		default:

			t := tabby.New()
			t.AddHeader("APP", "VERSION", "REPO", "BRANCH")
			for _, app := range r.AppReleases.GetAppsSortedByName() {
				t.AddLine(app.Name, app.Version, app.Repo, app.Branch)
			}
			t.Print()

		}
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


		//app.ReleaseValues = appRelease.Values
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

var releaseCreateCmd = &cobra.Command{
	Use:   "create {name} {path}",
	Args:  cobra.ExactArgs(2),
	Short: "Creates a new release.",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, path := args[0], args[1]
		c := bosun.ConfigFragment{
			FromPath: path,
			Releases: []*bosun.ReleaseConfig{
				&bosun.ReleaseConfig{
					Name: name,
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
}

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
	Short: "Removes one or more apps to a release.",
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

var releaseValidateCmd = addCommand(releaseCmd, &cobra.Command{
	Use:           "validate",
	Short:         "Validates the release.",
	Long:          "Validation checks that all apps in this release have a published chart and docker image for this release.",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()
		release := mustGetCurrentRelease(b)

		ctx := b.NewContext()

		w := new(strings.Builder)
		hasErrors := false

		apps := release.AppReleases.GetAppsSortedByName()
		p := NewProgressBar(len(apps))

		for _, app := range apps {

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
	},
})

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

		for _, appRelease := range appReleases {
			ctx = ctx.WithAppRelease(appRelease)
			if appRelease.AppRepo == nil {
				ctx.Log.Warn("AppRepo not found.")
			}
			ctx.Log.Info("Pulling latest...")
			err := appRelease.AppRepo.PullRepo(ctx)
			if err != nil {
				ctx.Log.WithError(err).Error("Pull failed.")
				continue
			}
			err = release.IncludeApp(ctx, appRelease.AppRepo)
			if err != nil {
				ctx.Log.WithError(err).Error("Update release failed.")
			}
		}

		err := release.Parent.Save()

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().StringSlice(ArgReleaseIncludeApps, []string{}, "Whitelist of apps to sync. If not provided, all apps are synced.")
	cmd.Flags().StringSlice(ArgReleaseExcludeApps, []string{}, "Blacklist of apps to exclude from the sync.")
})

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
}, func(cmd *cobra.Command) {
	cmd.Flags().StringSlice(ArgReleaseIncludeApps, []string{}, "Whitelist of apps to test. If not provided, all apps are tested.")
	cmd.Flags().StringSlice(ArgReleaseExcludeApps, []string{}, "Blacklist of apps to exclude from testing.")
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

		err := release.Deploy(ctx)

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().StringSlice(ArgReleaseIncludeApps, []string{}, "Whitelist of apps to include in release. If not provided, all apps in the release are released.")
	cmd.Flags().StringSlice(ArgReleaseExcludeApps, []string{}, "Blacklist of apps to exclude from the release.")
})

const ArgReleaseIncludeApps = "include"
const ArgReleaseExcludeApps = "exclude"
