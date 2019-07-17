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
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/pkg/errors"
	"github.com/schollz/progressbar"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io/ioutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"regexp"
	"strings"
	"sync"
)

const (
	ArgSvcToggleLocalhost = "localhost"
	ArgSvcToggleMinikube  = "minikube"
	ArgFilteringAll       = "all"
	ArgFilteringLabels    = "labels"
	ArgAppListDiff        = "diff"
	ArgAppListSkipActual  = "skip-actual"
	ArgAppValueSet        = "value-sets"
	ArgAppSet             = "set"

	ArgAppDeployDeps    = "deploy-deps"
	ArgAppFromRelease   = "release"
	ArgAppLatest        = "latest"
	ArgAppDeletePurge   = "purge"
	ArgAppCloneDir      = "dir"
	ArgFilteringInclude = "include"
	ArgFilteringExclude = "exclude"
	ArgChangeLogMoreDetails = "details"
)

func init() {
	appCmd.PersistentFlags().BoolP(ArgFilteringAll, "a", false, "Apply to all known microservices.")
	appCmd.PersistentFlags().StringSliceP(ArgFilteringLabels, "i", []string{}, "Apply to microservices with the provided labels.")
	appCmd.PersistentFlags().StringSlice(ArgFilteringInclude, []string{}, `Only include apps which match the provided selectors. --include trumps --exclude.".`)
	appCmd.PersistentFlags().StringSlice(ArgFilteringExclude, []string{}, `Don't include apps which match the provided selectors.".`)

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
	Short:   "App commands",
}

var _ = addCommand(appCmd, configImportCmd)

var appVersionCmd = &cobra.Command{
	Use:     "version [name]",
	Aliases: []string{"v"},
	Args:    cobra.RangeArgs(0, 1),
	Short:   "Outputs the version of an app.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
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
		b := MustGetBosun()
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

		b := MustGetBosun()
		app := mustGetApp(b, args)

		if app.IsFromManifest {
			return errors.New("bump is only available for apps which you have added to bosun")
		}

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
			_, err = g.Exec("tag", app.Version.String())
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
func appBump(b *bosun.Bosun, app *bosun.App, bump string) error {
	ctx := b.NewContext()

	err := app.BumpVersion(ctx, bump)
	if err != nil {
		return err
	}

	err = app.Parent.Save()
	if err == nil {
		pkg.Log.Infof("Updated %q to version %s and saved in %q", app.Name, app.Version, app.Parent.FromPath)
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

		b := MustGetBosun()
		apps := mustGetAppsIncludeCurrent(b, args)
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

		b := MustGetBosun()
		apps := mustGetAppsIncludeCurrent(b, args)
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

		b := MustGetBosun()

		apps, err := getAppsIncludeCurrent(b, args)
		if err != nil {
			return err
		}

		p := progressbar.New(len(apps))
		ctx := b.NewContext()

		deploySettings := bosun.DeploySettings{
			Environment: ctx.Env,
		}
		for _, app := range apps {
			if !app.HasChart() {
				continue
			}

			ctx = ctx.WithApp(app)
			appManifest, err := app.GetManifest(ctx)
			if err != nil {
				return errors.Wrapf(err, "get manifest for %q", app.Name)
			}
			appDeploy, err := bosun.NewAppDeploy(ctx, deploySettings, appManifest)
			if err != nil {
				ctx.Log.WithError(err).Error("Error creating app deploy for current state analysis.")
				continue
			}
			ctx = ctx.WithAppDeploy(appDeploy)

			log := ctx.Log
			log.Debug("Getting actual state...")
			err = appDeploy.LoadActualState(ctx, false)
			p.Add(1)
			if err != nil {
				log.WithError(err).Error("Could not get actual state.")
				return err
			}
			b.SetDesiredState(app.Name, appDeploy.ActualState)
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

		b := MustGetBosun()
		env := b.GetCurrentEnvironment()
		f := getFilterParams(b, args)
		chain := f.Chain().Then().Including(filter.FilterMatchAll())
		apps, err := f.GetAppsChain(chain)
		if err != nil {
			return err
		}

		p := progressbar.New(len(apps))

		diff := viper.GetBool(ArgAppListDiff)
		skipActual := viper.GetBool(ArgAppListSkipActual)

		appReleases, err := getAppDeploysFromApps(b, apps)
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
					defer wg.Done()
					ctx := b.NewContext().WithAppDeploy(appRelease)
					err := appRelease.LoadActualState(ctx, false)
					if err != nil {
						ctx.Log.WithError(err).Fatal()
					}
					p.Add(1)
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
				fmtTableEntry(fmt.Sprintf("%#v", m.AppConfig.Labels)))
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

		b := MustGetBosun()
		c := b.GetCurrentEnvironment()

		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

		if c.Name != "red" {
			return errors.New("Environment must be set to 'red' to toggle services.")
		}

		repos, err := getAppsIncludeCurrent(b, args)
		if err != nil {
			return err
		}
		apps, err := getAppDeploysFromApps(b, repos)
		if err != nil {
			return err
		}

		ctx := b.NewContext()

		appServiceChanged := false

		for _, app := range apps {

			ctx = ctx.WithAppDeploy(app)
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

				appServiceChanged = true
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
				appServiceChanged = true
			}

			b.SetDesiredState(app.Name, app.DesiredState)
		}

		if appServiceChanged {

			client, err := kube.GetKubeClient()
			if err != nil {
				return errors.Wrap(err, "get kube client for tweaking service")
			}

			ctx.Log.Warn("Recycling kube-dns to ensure new services are routed correctly.")

			podClient := client.CoreV1().Pods("kube-system")
			pods, err := podClient.List(metav1.ListOptions{
				LabelSelector: "k8s-app=kube-dns",
			})
			if err != nil {
				return errors.Wrap(err, "find kube-dns")
			}
			if len(pods.Items) == 0 {
				return errors.New("no kube-dns pods found")
			}
			for _, pod := range pods.Items {
				ctx.Log.Warnf("Deleting pod %q...", pod.Name)
				err = podClient.Delete(pod.Name, metav1.NewDeleteOptions(0))
				if err != nil {
					return errors.Wrapf(err, "delete pod %q", pod.Name)
				}
				ctx.Log.Warnf("Pod %q deleted. Kube-hosted services may be unavailable for a short time.", pod.Name)
			}
		}

		err = b.Save()

		return err
	},
}

var appDeployCmd = addCommand(appCmd, &cobra.Command{
	Use:   "deploy [name] [name...]",
	Short: "Deploys the requested app.",
	Long: `If app is not specified, the first app in the nearest bosun.yaml file is used.

If you want to apply specific value sets to this deployment, use the --value-sets (or -v) flag.
You can view the available value-sets using "bosun env value-sets". 

A common use case for using value-sets is if you want to change the tag and pull behavior so that
the pod will use an image you just built using the minikube docker agent. The "latest" and "pullIfNotPresent"
value-sets are available for this. To use them:

bosun app deploy {appName} --value-sets latest,pullIfNotPresent
`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := MustGetBosun()

		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

		ctx := b.NewContext()

		apps := mustGetAppsIncludeCurrent(b, args)

		// fmt.Println("App Configs:")
		// for _, app := range apps {
		// 	fmt.Println(MustYaml(app.AppConfig))
		// }

		valueSets, err := getValueSetSlice(b, b.GetCurrentEnvironment())
		if err != nil {
			return err
		}
		includeDeps := viper.GetBool(ArgAppDeployDeps)
		deploySettings := bosun.DeploySettings{
			Environment:        ctx.Env,
			ValueSets:          valueSets,
			UseLocalContent:    true,
			IgnoreDependencies: !includeDeps,
			Apps:               map[string]*bosun.App{},
		}

		for _, app := range apps {
			ctx.Log.WithField("app", app.Name).Debug("Including in release.")
			deploySettings.Apps[app.Name] = app
			if includeDeps {
				ctx.Log.Debug("Including dependencies of all apps...")
				deps, err := b.GetAppDependencies(app.Name)
				if err != nil {
					return errors.Wrapf(err, "getting deps for %q", app.Name)
				}
				for _, depName := range deps {
					if _, ok := deploySettings.Apps[depName]; !ok {

						depApp, err := b.GetApp(depName)
						if err != nil {
							return errors.Wrapf(err, "getting app for dep %q", app.Name)
						}
						deploySettings.Apps[depApp.Name] = depApp
					}
				}
			}
		}

		fromRelease := viper.GetString(ArgAppFromRelease)

		if fromRelease != "" {
			ctx.Log.Warnf("Deploying app from %q rather than your local clone", fromRelease)
			var p *bosun.Platform
			p, err = b.GetCurrentPlatform()
			if err != nil {
				return err
			}

			deploySettings.Manifest, err = p.GetReleaseManifestByName(fromRelease)
			if err != nil {
				return err
			}
			// reset the default deploy apps so that only the selected apps are deployed
			deploySettings.Manifest.DefaultDeployApps = map[string]bool{}
			for _, app := range apps {
				deploySettings.Manifest.DefaultDeployApps[app.Name] = true
			}
		} else if viper.GetBool(ArgAppLatest) {
			err = pullApps(ctx, apps)
			deploySettings.ValueSets = append(deploySettings.ValueSets, bosun.ValueSet{Static: map[string]interface{}{"tag": "latest"}})
		}

		r, err := bosun.NewDeploy(ctx, deploySettings)
		if err != nil {
			return err
		}

		ctx.Log.Debugf("Created deploy")

		err = r.Deploy(ctx)

		if err != nil {
			return errors.Wrap(err, "deploy failed")
		}

		err = b.Save()

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(ArgAppDeployDeps, false, "Also deploy all dependencies of the requested apps.")
	cmd.Flags().StringP(ArgAppFromRelease, "r", "", "Deploy using the specified release from the platform, rather than your local clone.")
	cmd.Flags().Bool(ArgAppLatest, false, "Force bosun to pull the latest of the app and deploy that.")
	cmd.Flags().StringSliceP(ArgAppValueSet, "v", []string{}, "Additional value sets to include in this deploy.")
	cmd.Flags().StringSliceP(ArgAppSet, "s", []string{}, "Value overrides to set in this deploy, as key=value pairs.")
})

var appRecycleCmd = addCommand(appCmd, &cobra.Command{
	Use:          "recycle [name] [name...]",
	Short:        "Recycles the requested app(s) by deleting their pods.",
	Long:         "If app is not specified, the first app in the nearest bosun.yaml file is used.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		viper.BindPFlags(cmd.Flags())

		b := MustGetBosun()
		ctx := b.NewContext()

		env := b.GetCurrentEnvironment()

		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

		releases := getFilterParams(b, args).IncludeCurrent().MustGetAppDeploys()

		pullLatest := viper.GetBool(ArgAppRecyclePullLatest)

		for _, appRelease := range releases {
			ctx := ctx.WithAppDeploy(appRelease)

			if env.IsLocal && pullLatest {
				ctx.Log.Info("Pulling latest version of image(s) on minikube...")
				for _, image := range appRelease.AppConfig.GetImages() {
					imageName := image.GetFullNameWithTag("latest")
					err := pkg.NewCommand("sh", "-c", fmt.Sprintf("eval $(minikube docker-env); docker pull %s", imageName)).RunE()
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

		b := MustGetBosun()

		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

		appReleases := getFilterParams(b, args).IncludeCurrent().MustGetAppDeploys()

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

		return errors.New("this needs to be refactored to use new deploy paradigm")
		//
		// viper.BindPFlags(cmd.Flags())
		//
		// b := MustGetBosun()
		// c := b.GetCurrentEnvironment()
		//
		// if err := b.ConfirmEnvironment(); err != nil {
		// 	return err
		// }
		//
		// if c.Name != "red" {
		// 	return errors.New("Environment must be set to 'red' to run apps.")
		// }
		//
		// app := mustGetApp(b, args)
		//
		// run, err := app.GetRunCommand()
		// if err != nil {
		// 	return err
		// }
		//
		// ctx := b.NewContext()
		//
		// appRelease, err := bosun.NewAppReleaseFromApp(ctx, app)
		// if err != nil {
		// 	return err
		// }
		//
		// appRelease.DesiredState.Routing = bosun.RoutingLocalhost
		// appRelease.DesiredState.Status = bosun.StatusDeployed
		// b.SetDesiredState(app.Name, appRelease.DesiredState)
		// err = appRelease.Reconcile(ctx)
		// if err != nil {
		// 	return err
		// }
		//
		// err = b.Save()
		//
		// done := make(chan struct{})
		// s := make(chan os.Signal)
		// signal.Notify(s, os.Kill, os.Interrupt)
		// log := pkg.Log.WithField("cmd", run.Args)
		//
		// go func() {
		// 	log.Info("Running child process.")
		// 	err = run.Run()
		// 	close(done)
		// }()
		//
		// select {
		// case <-done:
		// case <-s:
		// 	log.Info("Killing child process.")
		// 	run.Process.Signal(os.Interrupt)
		// }
		// select {
		// case <-done:
		// case <-time.After(3 * time.Second):
		// 	log.Warn("Child process did not exit when signalled.")
		// 	run.Process.Kill()
		// }
		//
		// return err
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

			b := MustGetBosun()
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

			b := MustGetBosun()
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
			b := MustGetBosun()
			app := mustGetApp(b, args)
			ctx := b.NewContext().WithApp(app)
			err := app.BuildImages(ctx)
			return err
		},
	})

var appPullCmd = addCommand(
	appCmd,
	&cobra.Command{
		Use:           "pull [app] [app...]",
		Short:         "Pulls the repo for the app.",
		Long:          "If app is not provided, the current directory is used.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			b := MustGetBosun()
			ctx := b.NewContext()
			apps, err := getAppsIncludeCurrent(b, args)
			if err != nil {
				return err
			}
			return pullApps(ctx, apps)
		},
	})

func pullApps(ctx bosun.BosunContext, apps []*bosun.App) error {
	repos := map[string]*bosun.Repo{}
	for _, app := range apps {
		if app.Repo == nil {
			ctx.Log.Errorf("no repo identified for app %q", app.Name)
		}
		if app.Repo.CheckCloned() != nil {
			ctx.Log.Warn("%q is not cloned", app.Name)
		}
		repos[app.RepoName] = app.Repo
	}

	var lastFailure error
	for _, repo := range repos {
		log := ctx.Log.WithField("repo", repo.Name)
		log.Info("Pulling...")
		err := repo.Pull(ctx)
		if err != nil {
			lastFailure = err
			log.WithError(err).Error("Error pulling.")
		} else {
			log.Info("Pulled.")
		}
	}

	return lastFailure
}

var appScriptCmd = addCommand(appCmd, &cobra.Command{
	Use:          "script [app] {name}",
	Args:         cobra.RangeArgs(1, 2),
	Aliases:      []string{"scripts"},
	Short:        "Run a scripted sequence of commands.",
	Long:         `If app is not provided, the current directory is used.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b := MustGetBosun()

		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

		var app *bosun.App
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

		ctx := b.NewContext()
		values, err := getResolvedValuesFromApp(b, app)
		if err != nil {
			return err
		}
		ctx = ctx.WithPersistableValues(values)

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

		b := MustGetBosun()

		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

		var app *bosun.App
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

		values, err := getResolvedValuesFromApp(b, app)
		if err != nil {
			return err
		}
		ctx = ctx.WithPersistableValues(values)

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
			b := MustGetBosun()

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
				b = MustGetBosun()
			}

			apps, err := getAppsIncludeCurrent(b, args)
			if err != nil {
				return err
			}

			ctx := b.NewContext()
			var lastErr error
			for _, app := range apps {
				log := ctx.Log.WithField("app", app.Name).WithField("repo", app.Repo)

				if app.IsRepoCloned() {
					pkg.Log.Infof("App already cloned to %q", app.FromPath)
					continue
				}
				log.Info("Cloning...")

				err = app.Repo.Clone(ctx, dir)
				if err != nil {
					lastErr = err
					log.WithError(err).Error("Error cloning.")
				} else {
					log.Info("Cloned.")
				}
			}

			return lastErr
		},
	},
	func(cmd *cobra.Command) {
		cmd.Flags().String(ArgAppCloneDir, "", "The directory to clone into.")
	})

var appChangeLog = addCommand(
	appCmd,
	&cobra.Command{
		Use:     "change-log {app} {from} [to]",
		Short:   "Prints a change log in console",
		Aliases: []string{"l"},
		Args:    cobra.RangeArgs(2, 3),
		Long:    "Prints a changelog of changes. 'to' is by default 'master'",
		RunE: func(cmd *cobra.Command, args []string) error {
			viper.BindPFlags(cmd.Flags())
			b := MustGetBosun()
			app := mustGetApp(b, args)
			detailsFlag := viper.GetBool(ArgChangeLogMoreDetails)
			appPath := app.FromPath
			var from string
			var to string

			from = args[1]
			if len(args) == 2 {
				to = "master"
			} else {
				to = args[2]
			}
			gitLogPath := new(strings.Builder)
			g, err := git.NewGitWrapper(appPath)
			if err != nil {
				return err
			}

			fmt.Fprintf(gitLogPath, to)
			fmt.Fprintf(gitLogPath, "..")
			fmt.Fprintf(gitLogPath, from)

			cleanAppPath := strings.Replace(appPath, "bosun.yaml", "", 1)
			svc, err := b.GetIssueService(cleanAppPath)
			if err != nil {
				return err
			}

			changeLogOptions := git.GitChangeLogOptions{
				Description: detailsFlag,
				UnknownType: detailsFlag,
			}

			logs, err := g.ChangeLog(gitLogPath.String(), cleanAppPath, svc, changeLogOptions)
			if err != nil {
				return err //errors.New("Check that the app and branches are correct")
			}

			color.Green(logs.OutputMessage)

			return err
		},
	},
	func(cmd *cobra.Command) {
		cmd.Flags().BoolP(ArgChangeLogMoreDetails, "d", false, "Will output more details")
	})
