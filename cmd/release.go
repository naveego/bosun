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
	Short:   "Release commands.",
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
		releases := b.GetReleases()
		for _, release := range releases {
			name := release.Name
			if release == current {
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

		t := tabby.New()
		t.AddHeader("APP", "VERSION", "REPO", "BRANCH")
		for _, app := range r.Apps {
			t.AddLine(app.Name, app.Version, app.Repo, app.Branch)
		}
		t.Print()

	},
})

var releaseUseCmd = &cobra.Command{
	Use:   "use {name}",
	Args:  cobra.ExactArgs(1),
	Short: "Sets the release which release commands will work against.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()
		err := b.UseRelease(args[0])
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
			Releases: []*bosun.Release{
				&bosun.Release{
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

		apps, err := getApps(b, args)
		if err != nil {
			return err
		}

		ctx := b.NewContext("")

		for _, app := range apps {

			_, ok := release.Apps[app.Name]
			if ok {
				pkg.Log.Warnf("Overwriting existing app %q.", app.Name)
			} else {
				ctx.Log.Infof("Adding app %q", app.Name)
			}

			release.Apps[app.Name], err = app.MakeAppRelease(release)

			if err != nil {
				return errors.Errorf("could not make release for app %q: %s", app.Name, err)
			}
		}

		err = release.IncludeDependencies(ctx)
		if err != nil {
			return err
		}

		err = release.Fragment.Save()
		return err
	},
}

var releaseValidateCmd = addCommand(releaseCmd, &cobra.Command{
	Use:           "validate",
	Short:         "Validates the release.",
	Long:          "Validation checks that all apps in this release have a published chart and docker image for this release.",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()
		release := mustGetCurrentRelease(b)

		ctx := b.NewContext("")

		w := new(strings.Builder)
		hasErrors := false

		for _, app := range release.Apps {

			colorHeader.Fprintf(w, "%s\n", app.Name)
			errs := app.Validate(ctx)
			if len(errs) == 0 {
				colorOK.Fprintf(w, "OK\n")
			} else {
				for _, err := range errs {
					hasErrors = true
					colorError.Fprintf(w, "- %s\n", err)
				}
			}

			fmt.Fprintln(w)
		}

		fmt.Println(w.String())

		if hasErrors {
			return errors.New("Some apps are invalid.")
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
		b := mustGetBosun()
		release := mustGetCurrentRelease(b)

		ctx := b.NewContext("")

		err := release.Deploy(ctx)

		return err
	},
})
