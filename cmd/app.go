// Copyright © 2018 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"github.com/cheynewallace/tabby"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/pkg/errors"
	"github.com/schollz/progressbar"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
)

const (
	ArgSvcToggleLocalhost = "localhost"
	ArgSvcToggleMinikube  = "minikube"
	ArgAppAll             = "all"
	ArgAppLabels          = "labels"
	ArgAppListDiff        = "diff"
	ArgAppListSkipActual  = "skip-actual"
	ArgAppDeploySet       = "set"
	ArgAppDeployDeps      = "deploy-deps"
	ArgAppDeletePurge     = "purge"
	ArgAppCloneDir        = "dir"
)

func init() {
	appCmd.PersistentFlags().BoolP(ArgAppAll, "a", false, "Apply to all known microservices.")
	appCmd.PersistentFlags().StringSliceP(ArgAppLabels, "i", []string{}, "Apply to microservices with the provided labels.")

	appCmd.AddCommand(appToggleCmd)
	appToggleCmd.Flags().Bool(ArgSvcToggleLocalhost, false, "Run service at localhost.")
	appToggleCmd.Flags().Bool(ArgSvcToggleMinikube, false, "Run service at minikube.")

	appStatusCmd.Flags().Bool(ArgAppListDiff, false, "Run diff on deployed charts.")
	appStatusCmd.Flags().BoolP(ArgAppListSkipActual, "s", false, "Skip collection of actual state.")
	appCmd.AddCommand(appStatusCmd)

	appCmd.AddCommand(appAcceptActualCmd)

	appDeployCmd.Flags().Bool(ArgAppDeployDeps, false, "Also deploy all dependencies of the requested apps.")
	appDeployCmd.Flags().StringSlice(ArgAppDeploySet, []string{}, "Additional values to pass to helm for this deploy.")
	appCmd.AddCommand(appDeployCmd)

	appDeleteCmd.Flags().Bool(ArgAppDeletePurge, false, "Purge the release from the cluster.")
	appCmd.AddCommand(appDeleteCmd)

	appCmd.AddCommand(appListCmd)

	appCmd.AddCommand(appBumpCmd)

	appCmd.AddCommand(appRunCmd)
	appCmd.AddCommand(appVersionCmd)
	rootCmd.AddCommand(appCmd)
}

// appCmd represents the app command
var appCmd = &cobra.Command{
	Use:     "app",
	Aliases: []string{"apps", "a"},
	Short:   "AppRepo commands",
}

var appVersionCmd = &cobra.Command{
	Use:     "version [name]",
	Aliases: []string{"v"},
	Args:    cobra.RangeArgs(0, 1),
	Short:   "Outputs the version of an app.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()
		app := mustGetApp(b, args)
		fmt.Println(app.Version)
		return nil
	},
}

var appBumpCmd = &cobra.Command{
	Use:   "bump {name} {major|minor|patch|major.minor.patch}",
	Args:  cobra.ExactArgs(2),
	Short: "Updates the version of an app.",
	RunE: func(cmd *cobra.Command, args []string) error {

		b := mustGetBosun()
		app := mustGetApp(b, args)
		ctx := b.NewContext()

		err := app.BumpVersion(ctx, args[1])
		if err != nil {
			return err
		}

		err = app.Fragment.Save()
		if err == nil {
			fmt.Printf("Updated %q to version %s and saved in %q", app.Name, app.Version, app.Fragment.FromPath)
		}
		return err

	},
}

var appAcceptActualCmd = &cobra.Command{
	Use:          "accept-actual [name...]",
	Short:        "Updates the desired state to match the actual state of the apps. ",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := mustGetBosun()

		apps, err := getAppRepos(b, args)
		if err != nil {
			return err
		}

		p := progressbar.New(len(apps))

		for _, app := range apps {
			ctx := b.NewContext()
			appRelease, err := bosun.NewAppReleaseFromRepo(ctx, app)
			ctx = ctx.WithAppRelease(appRelease)

			log := pkg.Log.WithField("name", app)
			log.Debug("Getting actual state...")
			err = appRelease.LoadActualState(ctx, false)
			p.Add(1)
			if err != nil {
				log.WithError(err).Error("Could not get actual state.")
				return err
			}
			b.SetDesiredState(app.Name, appRelease.ActualState)
			log.Debug("Updated.")
			return nil
		}

		err = b.Save()

		return err
	},
}


var appListCmd = &cobra.Command{
	Use:          "list",
	Aliases:[]string{"ls"},
	Short:        "Lists the static config of all known apps.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		viper.SetDefault(ArgAppAll, true)

		b := mustGetBosun()
		apps, err := getAppRepos(b, args)
		if err != nil {
			return err
		}

		t := tabby.New()
		t.AddHeader("APP", "AVAILABLE", "VERSION", "PATH or REPO", "BRANCH")
		for _, app := range apps {
			var check, pathrepo, branch, version string

			if app.IsRepoCloned() {
				check = "✔"
				pathrepo = app.FromPath
				branch = app.GetBranch()
				version = app.Version
			} else {
				check = ""
				pathrepo = app.Repo
				branch = ""
				version = app.Version

			}
			t.AddLine(app.Name, check, version, pathrepo, branch)
		}

		t.Print()

		return nil
	},
}

var appStatusCmd = &cobra.Command{
	Use:          "status [name...]",
	Short:        "Lists apps",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := mustGetBosun()

		if len(viper.GetStringSlice(ArgAppLabels)) == 0 && len(args) == 0 {
			viper.Set(ArgAppAll, true)
		}

		env := b.GetCurrentEnvironment()

		apps, err := getAppRepos(b, args)
		if err != nil {
			return err
		}

		p := progressbar.New(len(apps))

		diff := viper.GetBool(ArgAppListDiff)
		skipActual := viper.GetBool(ArgAppListSkipActual)

		appReleases, err := getAppReleasesFromApps(b, apps)
		if err != nil {
			return err
		}

		// check first to avoid concurrency issues
		_ = b.IsClusterAvailable()

		if !skipActual {
			wg := new(sync.WaitGroup)
			wg.Add(len(appReleases))
			for i := range appReleases {
				appRelease := appReleases[i]
				go func() {
					ctx := b.NewContext().WithAppRelease(appRelease)
					err := appRelease.LoadActualState(ctx, false)
					if err != nil {
						ctx.Log.WithError(err).Fatal()
					}
					p.Add(1)

					wg.Done()
				}()
			}
			wg.Wait()
		}

		fmt.Println()
		fmt.Println()

		t := tabby.New()
		t.AddHeader(fmtTableEntry("APP"),
				fmtTableEntry("STATUS"),
				fmtTableEntry("ROUTE"),
				fmtTableEntry("DIFF"),
				fmtTableEntry("LABELS"))
		for _, m := range appReleases {

			actual, desired := m.ActualState, m.DesiredState
			var diffStatus string
			if diff && actual.Diff != "" {
				diffStatus = "detected"
			}

			if desired.Status == "" {
				desired.Status = bosun.StatusNotFound
			}

			if desired.Routing == "" {
				desired.Routing = bosun.RoutingNA
			}

			routing := "n/a"
			if env.Cluster == "minikube" {
				routing = fmtDesiredActual(desired.Routing, actual.Routing)
			}

			t.AddLine(
				fmtTableEntry(m.Name),
				fmtDesiredActual(desired.Status, actual.Status),
				routing,
				fmtTableEntry(diffStatus),
				fmtTableEntry(strings.Join(m.AppRepo.Labels, ", ")))
		}

		t.Print()

		return nil
	},
}

func fmtDesiredActual(desired, actual interface{}) string {

	if desired == actual {
		return fmt.Sprintf("✔   ️%v", actual)
	}

	return fmt.Sprintf("❌   %v [want %v]", actual, desired)

}

func fmtTableEntry(v interface{}) string {
	return fmt.Sprintf("%v", v)
}

var appToggleCmd = &cobra.Command{
	Use:          "toggle [name] [name...]",
	Short:        "Toggles or sets where traffic for an app will be routed to.",
	Long:         "If app is not specified, the first app in the nearest bosun.yaml file is used.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		viper.BindPFlags(cmd.Flags())

		b := mustGetBosun()
		c := b.GetCurrentEnvironment()

		if c.Name != "red" {
			return errors.New("Environment must be set to 'red' to toggle services.")
		}

		repos, err := getAppRepos(b, args)
		if err != nil {
			return err
		}
		apps, err := getAppReleasesFromApps(b, repos)
		if err != nil {
			return err
		}

		ctx := b.NewContext()

		for _, app := range apps {
			wantsLocalhost := viper.GetBool(ArgSvcToggleLocalhost)
			wantsMinikube := viper.GetBool(ArgSvcToggleMinikube)
			if wantsLocalhost {
				app.DesiredState.Routing = bosun.RoutingLocalhost
			} else if wantsMinikube {
				app.DesiredState.Routing = bosun.RoutingCluster
			} else {
				switch app.DesiredState.Routing {
				case bosun.RoutingCluster:
					app.DesiredState.Routing = bosun.RoutingLocalhost
				case bosun.RoutingLocalhost:
					app.DesiredState.Routing = bosun.RoutingCluster
				default:
					app.DesiredState.Routing = bosun.RoutingCluster
				}

			}

			app.DesiredState.Status = bosun.StatusDeployed

			err = app.Reconcile(ctx)

			if err != nil {
				return err
			}
		}

		err = b.Save()

		return err
	},
}

var appDeployCmd = &cobra.Command{
	Use:          "deploy [name] [name...]",
	Short:        "Deploys the requested app.",
	Long:         "If app is not specified, the first app in the nearest bosun.yaml file is used.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		viper.BindPFlags(cmd.Flags())

		b := mustGetBosun()
		ctx := b.NewContext()

		apps, err := getAppRepos(b, args)
		if err != nil {
			return err
		}

		ctx.Log.Debugf("AppReleaseConfigs: \n%s\n", MustYaml(apps))

		sets := map[string]string{}
		for _, set := range viper.GetStringSlice(ArgAppDeploySet) {

			segs := strings.Split(set, "=")
			if len(segs) != 2 {
				return errors.Errorf("invalid set (should be key=value): %q", set)
			}
			sets[segs[0]] = segs[1]
		}

		ctx.Log.Debug("Creating transient release...")
		rc := &bosun.ReleaseConfig{
			Name: time.Now().Format(time.RFC3339),
		}
		r, err := bosun.NewRelease(ctx, rc)
		if err != nil {
			return err
		}

		r.Transient = true

		for _, app := range apps {
			ctx.Log.WithField("app", app.Name).Debug("Including in release.")
			err = r.IncludeApp(ctx, app)
			if err != nil {
				return errors.Errorf("error including app %q in release: %s", app.Name, err)
			}
		}

		if viper.GetBool(ArgAppDeployDeps) {
			ctx.Log.Debug("Including dependencies of all apps...")
			err = r.IncludeDependencies(ctx)
			if err != nil {
				return errors.Wrap(err, "include dependencies")
			}
		}

		ctx.Log.Debugf("Created transient release to define deploy: \n%s\n", MustYaml(r))

		err = r.Deploy(ctx)

		if err != nil {
			return errors.Wrap(err, "deploy failed")
		}

		err = b.Save()

		return err
	},
}

var appDeleteCmd = &cobra.Command{
	Use:          "delete [name] [name...]",
	Short:        "Deletes the specified apps.",
	Long:         "If app is not specified, the first app in the nearest bosun.yaml file is used.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := mustGetBosun()

		appReleases := mustGetAppReleases(b, args)

		ctx := b.NewContext()

		for _, app := range appReleases {
			if viper.GetBool(ArgAppDeletePurge) {
				app.DesiredState.Status = bosun.StatusNotFound
			} else {
				app.DesiredState.Status = bosun.StatusDeleted
			}

			b.SetDesiredState(app.Name, app.DesiredState)

			app.DesiredState.Routing = bosun.RoutingNA
			err := app.Reconcile(ctx)
			if err != nil {
				return errors.Errorf("error deleting %q: %s", app.Name, err)
			}
		}

		err := b.Save()

		return err
	},
}

var appRunCmd = &cobra.Command{
	Use:           "run [app]",
	Short:         "Configures an app to have traffic routed to localhost, then runs the apps's run command.",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		viper.BindPFlags(cmd.Flags())

		b := mustGetBosun()
		c := b.GetCurrentEnvironment()

		if c.Name != "red" {
			return errors.New("Environment must be set to 'red' to run apps.")
		}

		app := mustGetApp(b, args)

		run, err := app.GetRunCommand()
		if err != nil {
			return err
		}

		ctx := b.NewContext()

		appRelease, err := bosun.NewAppReleaseFromRepo(ctx, app)
		if err != nil {
			return err
		}

		appRelease.DesiredState.Routing = bosun.RoutingLocalhost
		appRelease.DesiredState.Status = bosun.StatusDeployed
		b.SetDesiredState(app.Name, appRelease.DesiredState)
		err = appRelease.Reconcile(ctx)
		if err != nil {
			return err
		}

		err = b.Save()

		done := make(chan struct{})
		s := make(chan os.Signal)
		signal.Notify(s, os.Kill, os.Interrupt)
		log := pkg.Log.WithField("cmd", run.Args)

		go func() {
			log.Info("Running child process.")
			err = run.Run()
			close(done)
		}()

		select {
		case <-done:
		case <-s:
			log.Info("Killing child process.")
			run.Process.Signal(os.Interrupt)
		}
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			log.Warn("Child process did not exit when signalled.")
			run.Process.Kill()
		}

		return err
	},
}

var appPublishChartCmd = addCommand(
	appCmd,
	&cobra.Command{
		Use:           "publish-chart [app]",
		Args:          cobra.MaximumNArgs(1),
		Short:         "Publishes the chart for an app.",
		Long:          "If app is not provided, the current directory is used.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {

			b := mustGetBosun()
			app := mustGetApp(b, args)
			ctx := b.NewContext()
			err := app.PublishChart(ctx, viper.GetBool(ArgGlobalForce))
			return err
		},
	},
	func(cmd *cobra.Command) {
		cmd.Flags().Bool(ArgGlobalForce, false, "Force publish even if version exists.")
	})

var appPublishImageCmd = addCommand(
	appCmd,
	&cobra.Command{
		Use:   "publish-image [app]",
		Args:  cobra.MaximumNArgs(1),
		Short: "Publishes the image for an app.",
		Long: `If app is not provided, the current directory is used.
The image will be published with the "latest" tag and with a tag for the current version.
If the current branch is a release branch, the image will also be published with a tag formatted
as "version-release".
`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {

			b := mustGetBosun()
			app := mustGetApp(b, args)
			ctx := b.NewContext()
			err := app.PublishImage(ctx)
			return err
		},
	})

var appPullCmd = addCommand(
	appCmd,
	&cobra.Command{
		Use:           "pull [app]",
		Short:         "Pulls the repo for the app.",
		Long:          "If app is not provided, the current directory is used.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			b := mustGetBosun()
			ctx := b.NewContext()
			apps, err := getAppRepos(b, args)
			if err != nil {
				return err
			}
			repos := map[string]*bosun.AppRepo{}
			for _, app := range apps {
				repos[app.Repo] = app
			}

			for _, app := range repos {
				log := ctx.Log.WithField("repo", app.Repo)
				log.Info("Pulling...")
				err := app.PullRepo(ctx)
				if err != nil {
					log.WithError(err).Error("Error pulling.")
				} else {
					log.Info("Pulled.")
				}
			}
			return err
		},
	})

// var appScriptCmd = addCommand(appCmd, &cobra.Command{
// 	Use:          "script [app] {name}",
// 	Args:         cobra.RangeArgs(1, 2),
// 	Aliases:[]string{"scripts"},
// 	Short:        "Run a scripted sequence of commands.",
// 	Long:         `If app is not provided, the current directory is used.`,
// 	SilenceUsage: true,
// 	RunE: func(cmd *cobra.Command, args []string) error {
// 		viper.BindPFlags(cmd.Flags())
//
// 		b := mustGetBosun()
//
//
// 		app := mustGetApp(b, args)
//
// 		script, err := b.GetScript(args[0])
// 		if err != nil {
// 			scriptFilePath := args[0]
// 			var script bosun.Script
// 			data, err := ioutil.ReadFile(scriptFilePath)
// 			if err != nil {
// 				return err
// 			}
//
// 			err = yaml.Unmarshal(data, &script)
// 			if err != nil {
// 				return err
// 			}
// 		}
//
// 		err = b.Execute(script, scriptStepsSlice...)
//
// 		return err
// 	},
// })

var appCloneCmd = addCommand(
	appCmd,
	&cobra.Command{
		Use:   "clone [name] [name...]",
		Short: "Clones the repo for the named app(s).",
		Long:  "Uses the first directory in `gitRoots` from the root config.",
		RunE: func(cmd *cobra.Command, args []string) error {
			viper.BindPFlags(cmd.Flags())
			b := mustGetBosun()

			dir := viper.GetString(ArgAppCloneDir)
			roots := b.GetGitRoots()
			if dir == "" {
				if len(roots) == 0 {
					return errors.Errorf("gitRoots element is empty in config and --%s flag was not set", ArgAppCloneDir)
				}
				dir = roots[0]
			}
			rootExists := false
			for _, root := range roots {
				if root == dir {
					rootExists = true
					break
				}
			}
			if !rootExists {
				b.AddGitRoot(dir)
				err := b.Save()
				if err != nil {
					return err
				}
				b = mustGetBosun()
			}

			apps, err := getAppRepos(b, args)
			if err != nil {
				return err
			}

			repos := map[string]*bosun.AppRepo{}
			for _, app := range apps {
				repos[app.Repo] = app
			}

			ctx := b.NewContext()
			for _, app := range repos {
				log := ctx.Log.WithField("app", app.Name).WithField("repo", app.Repo)
				log.Info("Cloning...")
				if app.IsRepoCloned() {
					pkg.Log.Infof("AppRepo already cloned to %q", app.FromPath)
					continue
				}

				err := app.CloneRepo(ctx, dir)
				if err != nil {
					log.WithError(err).Error("Error cloning.")
				} else {
					log.Info("Cloned.")
				}
			}

			return err
		},
	},
	func(cmd *cobra.Command) {
		cmd.Flags().String(ArgAppCloneDir, "", "The directory to clone into.")
	})

func getStandardObjects(args []string) (*bosun.Bosun, *bosun.EnvironmentConfig, []*bosun.AppRepo, bosun.BosunContext) {
	b := mustGetBosun()
	env := b.GetCurrentEnvironment()
	ctx := b.NewContext()

	apps, err := getAppRepos(b, args)
	if err != nil {
		panic(err)
	}
	return b, env, apps, ctx
}
