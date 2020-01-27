package cmd

import (
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/command"
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
	Use:   "foreach {[app apps...] | --include {filter}} -- {command} [args]",
	Short: "Runs a command for each matched app.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b := MustGetBosun()


		appNames := args[0: cmd.ArgsLenAtDash()]
		commandArgs := args[cmd.ArgsLenAtDash():]


		apps := mustGetKnownApps(b, appNames)

		if len(args) < 1 {
			return errors.New("a command is required")
		}
		dryRun := viper.GetBool(ArgGlobalDryRun)


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
			script := strings.Join(commandArgs, " ")
			c := command.Command{
				Script:script,
			}
			ctx = ctx.WithDir(dir)
			_, err := c.Execute(ctx, command.CommandOpts{StreamOutput: true})
			if err != nil {
				color.Red("Command failed:\n")
				color.Yellow("dir:     %s\n", dir)
				color.Yellow("command: %s\n", script)
				color.Yellow("error:   %s\n", err.Error())
				continue
			}
		}

		return nil
	},
})
