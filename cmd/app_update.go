package cmd

import (
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"strings"
)

var appMutateCmd = addCommand(appCmd, &cobra.Command{
	Use:          "mutate",
	Short:        "Collects operations for updating app config files.",
	SilenceUsage: true,
})

var appMutateLabel = addCommand(appMutateCmd, &cobra.Command{
	Use:   "label {app} [apps...] {label=value} [label=value...]",
	Short: "Adds labels to one or more apps.",

	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b := MustGetBosun()

		var appNames []string
		labels := map[string]string{}
		for _, arg := range args {
			if strings.Contains(arg, "=") {
				segs := strings.Split(arg, "=")
				if len(segs) != 2 {
					return errors.Errorf("invalid label pair %q (want label=value)", arg)
				}
				labels[segs[0]] = segs[1]
			} else {
				appNames = append(appNames, arg)
			}
		}

		apps := getFilterParams(b, appNames).GetApps()

		if len(apps) == 0 {
			return errors.New("no apps")
		}
		if len(labels) == 0 {
			return errors.New("no labels")
		}

		for _, app := range apps {
			ctx := b.NewContext()
			if !app.IsRepoCloned() {
				ctx.Log().Warnf("App %q is not cloned, cannot be labelled (obtained app from provider %q)", app.Name, app.Provider)
				continue
			}

			if app.Labels == nil {
				app.Labels = filter.Labels{}
			}
			for k, v := range labels {
				app.Labels[k] = filter.LabelString(v)
			}
			err := app.FileSaver.Save()
			if err != nil {
				return errors.Wrapf(err, "saving app %s", app.Name)
			}
		}

		return nil
	},
})

var appForEachLabel = addCommand(appMutateCmd, &cobra.Command{
	Use:   "foreach --include {filter} -- {command} [args]",
	Short: "Runs a command for each matched app.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b := MustGetBosun()

		apps := mustGetKnownApps(b, nil)

		if len(args) < 1 {
			return errors.New("a command is required")
		}
		dryRun := viper.GetBool(ArgGlobalDryRun)


		commandExe := args[0]
		commandArgs := args[1:]

		for _, app := range apps {
			ctx := b.NewContext().WithApp(app)
			if !app.IsRepoCloned() {
				ctx.Log().Warn("App is not cloned, skipping...")
				continue
			}

			dir := app.Repo.LocalRepo.Path

			if dryRun {
				ctx.Log().Infof("DRYRUN - would have run command in %s", dir)
				continue
			}



			ctx.Log().Infof("Running command in %s", dir)
			_, err := pkg.NewShellExe(commandExe, commandArgs...).WithDir(dir).RunOutLog()
			if err != nil {
				color.Red("Command failed:\n")
				color.Yellow("dir:     %s\n", dir)
				color.Yellow("command: %s %s\n", commandExe, strings.Join(commandArgs, " "))
				color.Yellow("error:   %s\n", err.Error())
				continue
			}
		}

		return nil
	},
})

const (
	argAppMutateForeachCommand = "command"
)