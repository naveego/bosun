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
	"github.com/aryann/difflib"
	"github.com/cheynewallace/tabby"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/git"
	"github.com/pkg/errors"
	"github.com/schollz/progressbar"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io/ioutil"
	"os"
	"os/signal"
	"regexp"
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
	ArgInclude            = "include"
	ArgExclude            = "exclude"
)

func init() {
	appCmd.PersistentFlags().BoolP(ArgAppAll, "a", false, "Apply to all known microservices.")
	appCmd.PersistentFlags().StringSliceP(ArgAppLabels, "i", []string{}, "Apply to microservices with the provided labels.")
	appCmd.PersistentFlags().StringSlice(ArgInclude, []string{}, `Only include apps which match the provided selectors. --include trumps --exclude.".`)
	appCmd.PersistentFlags().StringSlice(ArgExclude, []string{}, `Don't include apps which match the provided selectors.".`)

	appCmd.AddCommand(appToggleCmd)
	appToggleCmd.Flags().Bool(ArgSvcToggleLocalhost, false, "Run service at localhost.")
	appToggleCmd.Flags().Bool(ArgSvcToggleMinikube, false, "Run service at minikube.")

	appStatusCmd.Flags().Bool(ArgAppListDiff, false, "Run diff on deployed charts.")
	appStatusCmd.Flags().BoolP(ArgAppListSkipActual, "s", false, "Skip collection of actual state.")
	appCmd.AddCommand(appStatusCmd)

	appCmd.AddCommand(appAcceptActualCmd)

	appDeleteCmd.Flags().Bool(ArgAppDeletePurge, false, "Purge the release from the cluster.")
	appCmd.AddCommand(appDeleteCmd)

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

var _ = addCommand(appCmd, configImportCmd)

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

var appRepoPathCmd = addCommand(appCmd, &cobra.Command{
	Use:   "repo-path [name]",
	Args:  cobra.RangeArgs(0, 1),
	Short: "Outputs the path where the app is cloned on the local system.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()
		app := mustGetApp(b, args)
		if !app.IsRepoCloned() {
			return errors.New("repo is not cloned")
		}
		path, err := git.GetRepoPath(app.FromPath)
		fmt.Println(path)
		return err
	},
})

var appBumpCmd = addCommand(appCmd, &cobra.Command{
	Use:   "bump {name} {major|minor|patch|major.minor.patch}",
	Args:  cobra.ExactArgs(2),
	Short: "Updates the version of an app.",
	RunE: func(cmd *cobra.Command, args []string) error {

		b := mustGetBosun()
		app := mustGetApp(b, args)

		g, err := git.NewGitWrapper(app.FromPath)
		if err != nil {
			return err
		}

		wantsTag := viper.GetBool(ArgAppBumpTag)
		if wantsTag {
			if g.IsDirty() {
				return errors.New("cannot bump version and tag when workspace is dirty")
			}
		}

		err = appBump(b, app, args[1])
		if err != nil {
			return err
		}

		if wantsTag {
			_, err = g.Exec("tag", app.Version)
			if err != nil {
				return err
			}
		}

		return nil
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(ArgAppBumpTag, false, "Create and push a git tag for the version.")
})

const (
	ArgAppBumpTag = "tag"
)

// appBump is the implementation of appBumpCmd
func appBump(b *bosun.Bosun, app *bosun.AppRepo, bump string) error {
	ctx := b.NewContext()

	err := app.BumpVersion(ctx, bump)
	if err != nil {
		return err
	}

	err = app.Fragment.Save()
	if err == nil {
		pkg.Log.Infof("Updated %q to version %s and saved in %q", app.Name, app.Version, app.Fragment.FromPath)
	}
	return err
}

var appAddHostsCmd = addCommand(appCmd, &cobra.Command{
	Use:   "add-hosts [name...]",
	Short: "Writes out what the hosts file apps to the hosts file would look like if the requested apps were bound to the minikube IP.",
	Long: `Writes out what the hosts file apps to the hosts file would look like if the requested apps were bound to the minikube IP.

The current domain and the minikube IP are used to populate the output. To update the hosts file, pipe to sudo tee /etc/hosts.`,
	Example: "bosun apps add-hosts --all | sudo tee /etc/hosts",
	RunE: func(cmd *cobra.Command, args []string) error {

		b := mustGetBosun()
		apps := mustGetAppRepos(b, args)
		env := b.GetCurrentEnvironment()
		ip := pkg.NewCommand("minikube", "ip").MustOut()

		toAdd := map[string]hostLine{}
		for _, app := range apps {
			host := fmt.Sprintf("%s.%s", app.Name, env.Domain)
			toAdd[host] = hostLine{
				IP:      ip,
				Host:    host,
				Comment: fmt.Sprintf("bosun"),
			}
		}

		hosts, err := ioutil.ReadFile("/etc/hosts")
		if err != nil {
			return err
		}

		var lines []hostLine
		for _, line := range strings.Split(string(hosts), "\n") {
			segs := hostLineRE.FindStringSubmatch(line)
			hostLine := hostLine{}
			if len(segs) == 0 {
				hostLine.Comment = strings.TrimPrefix(line, "#")
			}
			if len(segs) >= 3 {
				hostLine.IP = segs[1]
				hostLine.Host = segs[2]
			}
			if len(segs) >= 4 {
				hostLine.Comment = segs[3]
			}

			delete(toAdd, hostLine.Host)

			lines = append(lines, hostLine)
		}

		for _, line := range toAdd {
			lines = append(lines, line)
		}

		for _, h := range lines {
			if h.IP != "" && h.Host != "" {
				fmt.Fprintf(os.Stdout, "%s\t%s    ", h.IP, h.Host)
			}
			if h.Comment != "" {
				fmt.Fprintf(os.Stdout, "# %s", strings.TrimSpace(h.Comment))
				if h.IP == "" && h.Host == "" {
					fmt.Fprint(os.Stdout, "\t\t")
				}
			}
			fmt.Fprintln(os.Stdout)
		}

		return err
	},
})

var appRemoveHostsCmd = addCommand(appCmd, &cobra.Command{
	Use:   "remove-hosts [name...]",
	Short: "Removes apps with the current domain from the hosts file.",
	RunE: func(cmd *cobra.Command, args []string) error {

		b := mustGetBosun()
		apps := mustGetAppRepos(b, args)
		env := b.GetCurrentEnvironment()

		toRemove := map[string]bool{}
		for _, app := range apps {
			host := fmt.Sprintf("%s.%s", app.Name, env.Domain)
			toRemove[host] = true
		}

		hosts, err := ioutil.ReadFile("/etc/hosts")
		if err != nil {
			return err
		}

		var lines []hostLine
		for _, line := range strings.Split(string(hosts), "\n") {
			segs := hostLineRE.FindStringSubmatch(line)
			hostLine := hostLine{}
			if len(segs) == 0 {
				hostLine.Comment = strings.TrimPrefix(line, "#")
			}
			if len(segs) >= 3 {
				hostLine.IP = segs[1]
				hostLine.Host = segs[2]
			}
			if len(segs) >= 4 {
				hostLine.Comment = segs[3]
			}
			lines = append(lines, hostLine)
		}

		out, err := os.OpenFile("/etc/hosts", os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer out.Close()
		for _, line := range lines {
			if !toRemove[line.Host] {
				_, err = fmt.Fprintf(out, "%s\n", line.String())
				if err != nil {
					return err
				}
			}
		}

		return err
	},
})

var hostLineRE = regexp.MustCompile(`^\s*([A-Fa-f\d\.:]+)\s+(\S+) *#? *(.*)`)

type hostLine struct {
	IP      string
	Host    string
	Comment string
}

func (h hostLine) String() string {
	w := new(strings.Builder)
	if h.IP != "" && h.Host != "" {
		fmt.Fprintf(w, "%s\t%s\t", h.IP, h.Host)
	}
	if h.Comment != "" {
		fmt.Fprintf(w, "# %s", strings.TrimSpace(h.Comment))
	}
	return w.String()
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
			if !app.HasChart() {
				continue
			}
			ctx := b.NewContext().WithAppRepo(app)
			appRelease, err := bosun.NewAppReleaseFromRepo(ctx, app)
			if err != nil {
				ctx.Log.WithError(err).Error("Error creating app release for current state analysis.")
				continue
			}
			ctx = ctx.WithAppRelease(appRelease)

			log := ctx.Log
			log.Debug("Getting actual state...")
			err = appRelease.LoadActualState(ctx, false)
			p.Add(1)
			if err != nil {
				log.WithError(err).Error("Could not get actual state.")
				return err
			}
			b.SetDesiredState(app.Name, appRelease.ActualState)
			log.Debug("Updated.")
		}

		err = b.Save()

		return err
	},
}

var appStatusCmd = &cobra.Command{
	Use:          "status [name...]",
	Short:        "Lists apps",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := mustGetBosun()

		env := b.GetCurrentEnvironment()

		apps, err := getAppReposOpt(b, args, getAppReposOptions{ifNoMatchGetAll: true})
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
				fmtTableEntry(fmt.Sprintf("%#v", m.AppRepo.AppLabels)))
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

		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

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

			ctx = ctx.WithAppRelease(app)
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

			if app.DesiredState.Routing == bosun.RoutingLocalhost {

				err = app.RouteToLocalhost(ctx)
				if err != nil {
					return err
				}
			} else {
				// force upgrade the app to restore it to its normal state.
				ctx.Log.Info("Deleting app.")
				app.DesiredState.Status = bosun.StatusNotFound
				err = app.Reconcile(ctx)
				if err != nil {
					return err
				}

				ctx.Log.Info("Re-deploying app.")
				app.DesiredState.Status = bosun.StatusDeployed

				err = app.Reconcile(ctx)

				if err != nil {
					return err
				}
			}

			b.SetDesiredState(app.Name, app.DesiredState)
		}

		err = b.Save()

		return err
	},
}

var appDeployCmd = addCommand(appCmd, &cobra.Command{
	Use:          "deploy [name] [name...]",
	Short:        "Deploys the requested app.",
	Long:         "If app is not specified, the first app in the nearest bosun.yaml file is used.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		valueOverrides := map[string]string{}
		for _, set := range viper.GetStringSlice(ArgAppDeploySet) {

			segs := strings.Split(set, "=")
			if len(segs) != 2 {
				return errors.Errorf("invalid set (should be key=value): %q", set)
			}
			valueOverrides[segs[0]] = segs[1]
		}

		ipp := strings.ToUpper(viper.GetString(AppDeployPullPolicy))
		if strings.HasPrefix(ipp, "A") {
			ipp = "Always"
		} else if strings.HasPrefix(ipp, "I") {
			ipp = "IfNotPresent"
		}
		if ipp != "" {
			valueOverrides["imagePullPolicy"] = ipp
		}

		valueOverrides["tag"] = viper.GetString(AppDeployTag)

		b := mustGetBosun(bosun.Parameters{
			ValueOverrides: valueOverrides,
		})

		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

		ctx := b.NewContext()

		apps, err := getAppReposOpt(b, args, getAppReposOptions{})
		if err != nil {
			return err
		}

		ctx.Log.Debugf("AppReleaseConfigs: \n%s\n", MustYaml(apps))

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

		ctx.Log.Debugf("Created transient release to define deploy: \n%s\n", r.Name)

		err = r.Deploy(ctx)

		if err != nil {
			return errors.Wrap(err, "deploy failed")
		}

		err = b.Save()

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().StringP(AppDeployPullPolicy, "p", "", "Set the imagePullPolicy in the chart. (A = Always, I = IfNotPresent)")
	cmd.Flags().StringP(AppDeployTag, "t", "latest", "Set the tag used in the chart.")
	cmd.Flags().Bool(ArgAppDeployDeps, false, "Also deploy all dependencies of the requested apps.")
	cmd.Flags().StringSlice(ArgAppDeploySet, []string{}, "Additional values to pass to helm for this deploy.")
})

const (
	AppDeployPullPolicy = "pull-policy"
	AppDeployTag        = "tag"
)

var appRecycleCmd = addCommand(appCmd, &cobra.Command{
	Use:          "recycle [name] [name...]",
	Short:        "Recycles the requested app(s) by deleting their pods.",
	Long:         "If app is not specified, the first app in the nearest bosun.yaml file is used.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		viper.BindPFlags(cmd.Flags())

		b := mustGetBosun()
		ctx := b.NewContext()

		env := b.GetCurrentEnvironment()

		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

		releases := mustGetAppReleases(b, args)

		pullLatest := viper.GetBool(ArgAppRecyclePullLatest)

		for _, appRelease := range releases {
			ctx := ctx.WithAppRelease(appRelease)

			if env.IsLocal && pullLatest {
				ctx.Log.Info("Pulling latest version of image(s) on minikube...")
				for _, imageName := range appRelease.ImageNames {
					image := fmt.Sprintf("%s:latest", imageName)
					err := pkg.NewCommand("sh", "-c", fmt.Sprintf("eval $(minikube docker-env); docker pull %s", image)).RunE()
					if err != nil {
						return err
					}
				}
			}

			ctx.Log.Info("Recycling app...")
			err := appRelease.Recycle(ctx)
			if err != nil {
				return err
			}
		}

		return nil
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(ArgAppRecyclePullLatest, false, "Pull the latest image before recycling (only works in minikube).")
})

const ArgAppRecyclePullLatest = "pull-latest"

var appDeleteCmd = &cobra.Command{
	Use:          "delete [name] [name...]",
	Short:        "Deletes the specified apps.",
	Long:         "If app is not specified, the first app in the nearest bosun.yaml file is used.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := mustGetBosun()

		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

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

		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

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
		Use:     "publish-image [app]",
		Aliases: []string{"publish-images"},
		Args:    cobra.MaximumNArgs(1),
		Short:   "Publishes the image for an app.",
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
			err := app.PublishImages(ctx)
			return err
		},
	})

var appBuildImageCmd = addCommand(
	appCmd,
	&cobra.Command{
		Use:           "build-image [app]",
		Aliases:       []string{"build-images"},
		Args:          cobra.MaximumNArgs(1),
		Short:         "Builds the image(s) for an app.",
		Long:          `If app is not provided, the current directory is used. The image(s) will be built with the "latest" tag.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			b := mustGetBosun()
			app := mustGetApp(b, args)
			ctx := b.NewContext().WithAppRepo(app)
			err := app.BuildImages(ctx)
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

var appScriptCmd = addCommand(appCmd, &cobra.Command{
	Use:          "script [app] {name}",
	Args:         cobra.RangeArgs(1, 2),
	Aliases:      []string{"scripts"},
	Short:        "Run a scripted sequence of commands.",
	Long:         `If app is not provided, the current directory is used.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := mustGetBosun()

		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

		var app *bosun.AppRepo
		var scriptName string
		switch len(args) {
		case 1:
			app = mustGetApp(b, []string{})
			scriptName = args[0]
		case 2:
			app = mustGetApp(b, args[:1])
			scriptName = args[1]
		}

		if app == nil {
			panic("will not happen because of mustGetApp")
		}

		var script *bosun.Script
		var scriptNames []string
		for _, s := range app.Scripts {
			scriptNames = append(scriptNames, s.Name)
			if strings.EqualFold(s.Name, scriptName) {
				script = s
			}
		}
		if script == nil {
			return errors.Errorf("no script named %q in app %q\navailable scripts:\n-%s", scriptName, app.Name, strings.Join(scriptNames, "\n-"))
		}

		ctx := b.NewContext().WithDir(app.FromPath)

		appRelease, err := bosun.NewAppReleaseFromRepo(ctx, app)
		if err != nil {
			return err
		}

		values, err := appRelease.GetReleaseValues(ctx)
		if err != nil {
			return err
		}
		ctx = ctx.WithReleaseValues(values)

		err = script.Execute(ctx, scriptStepsSlice...)

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().IntSliceVar(&scriptStepsSlice, ArgScriptSteps, []int{}, "Steps to run (defaults to all steps)")
})

var appActionCmd = addCommand(appCmd, &cobra.Command{
	Use:          "action [app] {name}",
	Args:         cobra.RangeArgs(1, 2),
	Aliases:      []string{"actions", "run"},
	Short:        "Run an action associated with an app.",
	Long:         `If app is not provided, the current directory is used.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := mustGetBosun()

		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

		var app *bosun.AppRepo
		var actionName string
		switch len(args) {
		case 1:
			app = mustGetApp(b, []string{})
			actionName = args[0]
		case 2:
			app = mustGetApp(b, args[:1])
			actionName = args[1]
		}

		if app == nil {
			panic("will not happen because of mustGetApp")
		}

		var action *bosun.AppAction
		var actionNames []string
		for _, a := range app.Actions {
			actionNames = append(actionNames, a.Name)
			if strings.EqualFold(a.Name, actionName) {
				action = a
				break
			}
		}
		if action == nil {
			return errors.Errorf("no action named %q in app %q\navailable actions:\n-%s", actionName, app.Name, strings.Join(actionNames, "\n-"))
		}

		ctx := b.NewContext()

		appRelease, err := bosun.NewAppReleaseFromRepo(ctx, app)
		if err != nil {
			return err
		}

		values, err := appRelease.GetReleaseValues(ctx)
		if err != nil {
			return err
		}
		ctx = ctx.WithReleaseValues(values)

		err = action.Execute(ctx)

		return err
	},
})

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
			var err error
			if dir == "" {
				if len(roots) == 0 {
					p := promptui.Prompt{
						Label: "Provide git root (apps will be cloned to ./org/repo in the dir you specify)",
					}
					dir, err = p.Run()
					if err != nil {
						return err
					}
				} else {
					dir = roots[0]
				}
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

			ctx := b.NewContext()
			for _, app := range apps {
				log := ctx.Log.WithField("app", app.Name).WithField("repo", app.Repo)

				if app.IsRepoCloned() {
					pkg.Log.Infof("AppRepo already cloned to %q", app.FromPath)
					continue
				}
				log.Info("Cloning...")

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

var appDiffCmd = addCommand(
	appCmd,
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

			getValuesForEnv := func(scenario string) (string, error) {

				segs := strings.Split(scenario, "/")
				var releaseName, envName string
				var appRelease *bosun.AppRelease
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

				if releaseName != "" {
					releaseConfig, err := b.GetReleaseConfig(releaseName)
					release, err := bosun.NewRelease(ctx, releaseConfig)
					if err != nil {
						return "", err
					}
					appReleaseConfig, ok := release.AppReleaseConfigs[app.Name]
					if !ok {
						return "", errors.Errorf("no app named %q in release %q", app.Name, releaseName)
					}
					ctx = ctx.WithRelease(release)
					appRelease, err = bosun.NewAppRelease(ctx, appReleaseConfig)
					if err != nil {
						return "", err
					}
				} else {
					rc := &bosun.ReleaseConfig{
						Name: time.Now().Format(time.RFC3339),
					}
					r, err := bosun.NewRelease(ctx, rc)
					if err != nil {
						return "", err
					}
					r.Transient = true
					ctx = ctx.WithRelease(r)
					config, err := app.GetAppReleaseConfig(ctx)
					if err != nil {
						return "", errors.Wrap(err, "make app release config")
					}

					appRelease, err = bosun.NewAppRelease(ctx, config)
					if err != nil {
						return "", errors.Wrap(err, "make app release")
					}
				}

				values, err := appRelease.GetReleaseValues(ctx)
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
