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
	"text/tabwriter"
	"time"
)

const (
	ArgSvcToggleLocalhost = "localhost"
	ArgSvcToggleMinikube  = "minikube"
	ArgAppAll             = "all"
	ArgAppLabels          = "labels"
	ArgAppListDiff        = "diff"
	ArgAppListSkipActual  = "skip-actual"
	ArgAppDeployForce     = "force"
	ArgAppDeploySet       = "set"
	ArgAppDeployDeps      = "deploy-deps"
	ArgAppDeletePurge     = "purge"
)

func init() {
	appCmd.PersistentFlags().BoolP(ArgAppAll, "a", false, "Apply to all known microservices.")
	appCmd.PersistentFlags().StringSliceP(ArgAppLabels, "i", []string{}, "Apply to microservices with the provided labels.")

	appCmd.AddCommand(appToggleCmd)
	appToggleCmd.Flags().Bool(ArgSvcToggleLocalhost, false, "Run service at localhost.")
	appToggleCmd.Flags().Bool(ArgSvcToggleMinikube, false, "Run service at minikube.")

	appListCmd.Flags().Bool(ArgAppListDiff, false, "Run diff on deployed charts.")
	appListCmd.Flags().BoolP(ArgAppListSkipActual, "s", false, "Skip collection of actual state.")
	appCmd.AddCommand(appListCmd)

	appCmd.AddCommand(appAcceptActualCmd)

	appDeployCmd.Flags().Bool(ArgAppDeployForce, false, "Force deploy even if nothing has changed.")
	appDeployCmd.Flags().Bool(ArgAppDeployDeps, false, "Also deploy all dependencies of the requested apps.")
	appDeployCmd.Flags().StringSlice(ArgAppDeploySet, []string{}, "Additional values to pass to helm for this deploy.")
	appCmd.AddCommand(appDeployCmd)

	appDeleteCmd.Flags().Bool(ArgAppDeletePurge, false, "Purge the release from the cluster.")
	appCmd.AddCommand(appDeleteCmd)

	appCmd.AddCommand(appRunCmd)
	appCmd.AddCommand(appVersionCmd)
	rootCmd.AddCommand(appCmd)
}

// appCmd represents the app command
var appCmd = &cobra.Command{
	Use:     "app",
	Aliases: []string{"apps"},
	Short:   "Service commands",
}

var appVersionCmd = &cobra.Command{
	Use:     "version [name]",
	Aliases: []string{"v"},
	Args:    cobra.RangeArgs(0, 1),
	Short:   "Outputs the version of an app.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()
		app := getApp(b, args)
		fmt.Println(app.Version)
		return nil
	},
}

var appAcceptActualCmd = &cobra.Command{
	Use:          "accept-actual [name...]",
	Short:        "Updates the desired state to match the actual state of the apps. ",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b, err := getBosun()
		if err != nil {
			return err
		}

		apps, err := getApps(b, args)
		if err != nil {
			return err
		}

		p := progressbar.New(len(apps))

		foreachAppConcurrent(apps, func(app *bosun.App) error {
			err := app.LoadActualState(false)
			p.Add(1)
			return err
		})

		for _, app := range apps {
			log := pkg.Log.WithField("name", app)
			log.Debug("Getting actual state...")
			err = app.LoadActualState(true)
			if err != nil {
				log.WithError(err).Error("Could not get actual state.")
				continue
			}

			app.DesiredState = app.ActualState
			log.Debug("Updated.")
			p.Add(1)
		}

		err = b.Save()

		return err
	},
}

func foreachAppConcurrent(apps []*bosun.App, action func(app *bosun.App) error) error {

	wg := new(sync.WaitGroup)
	wg.Add(len(apps))

	hadErr := false

	for i := range apps {
		app := apps[i]
		go func() {
			err := action(app)
			if err != nil {
				hadErr = true
				pkg.Log.WithField("name", app.Name).WithError(err).Error("App action failed.")
			}
			wg.Done()
		}()
	}

	wg.Wait()

	if hadErr {
		return errors.New("Had an error.")
	}

	return nil
}

var appListCmd = &cobra.Command{
	Use:          "list [name...]",
	Aliases:      []string{"ls"},
	Short:        "Lists apps",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b, err := getBosun()
		if err != nil {
			return err
		}

		if len(viper.GetStringSlice(ArgAppLabels)) == 0 && len(args) == 0 {
			viper.Set(ArgAppAll, true)
		}

		env := b.GetCurrentEnvironment()

		apps, err := getApps(b, args)
		if err != nil {
			return err
		}

		p := progressbar.New(len(apps))

		diff := viper.GetBool(ArgAppListDiff)
		skipActual := viper.GetBool(ArgAppListSkipActual)

		if !skipActual {
			err = foreachAppConcurrent(apps, func(app *bosun.App) error {
				err := app.LoadActualState(false)
				p.Add(1)
				return err

			})
			if err != nil {
				return err
			}
		}

		sb := new(strings.Builder)
		w := new(tabwriter.Writer)
		w.Init(sb, 0, 8, 3, '\t', 0)
		fmt.Fprintln(w,
			fmtTableEntry("APP")+"\t"+
				fmtTableEntry("STATUS")+"\t"+
				fmtTableEntry("ROUTE")+"\t"+
				fmtTableEntry("DIFF")+"\t"+
				fmtTableEntry("LABELS")+"\t")
		for _, m := range apps {

			if !m.HasChart() {
				fmt.Fprintf(w, "%s - no chart \t\n", m.Name)
				continue
			}

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

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				fmtTableEntry(m.Name),
				fmtDesiredActual(desired.Status, actual.Status),
				routing,
				fmtTableEntry(diffStatus),
				fmtTableEntry(strings.Join(m.Labels, ", ")))

		}

		fmt.Println()
		w.Flush()
		fmt.Println(sb.String())

		return nil
	},
}

func fmtDesiredActual(desired, actual interface{}) string {

	if desired == actual {
		return fmt.Sprintf("✔️%v", actual)
	}

	return fmt.Sprintf("❌ %v [want %v]", actual, desired)

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

		b, err := getBosun()
		if err != nil {
			return err
		}
		c := b.GetCurrentEnvironment()

		if c.Name != "red" {
			return errors.New("Environment must be set to 'red' to toggle services.")
		}

		services, err := getApps(b, args)
		if err != nil {
			return err
		}

		for _, ms := range services {
			wantsLocalhost := viper.GetBool(ArgSvcToggleLocalhost)
			wantsMinikube := viper.GetBool(ArgSvcToggleMinikube)
			if wantsLocalhost {
				ms.DesiredState.Routing = bosun.RoutingLocalhost
			} else if wantsMinikube {
				ms.DesiredState.Routing = bosun.RoutingCluster
			} else {
				switch ms.DesiredState.Routing {
				case bosun.RoutingCluster:
					ms.DesiredState.Routing = bosun.RoutingLocalhost
				case bosun.RoutingLocalhost:
					ms.DesiredState.Routing = bosun.RoutingCluster
				default:
					ms.DesiredState.Routing = bosun.RoutingCluster
				}
			}

			err = b.Reconcile(ms)

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
		env := b.GetCurrentEnvironment()

		apps, err := getApps(b, args)
		if err != nil {
			return err
		}

		sets := map[string]string{}
		for _, set := range viper.GetStringSlice(ArgAppDeploySet) {

			segs := strings.Split(set, "=")
			if len(segs) != 2 {
				return errors.Errorf("invalid set (should be key=value): %q", set)
			}
			sets[segs[0]] = segs[1]
		}

		var requestedAppNames []string
		requestedAppNameSet := map[string]bool{}
		for _, app := range apps {
			requestedAppNameSet[app.Name] = true
		}
		for appName := range requestedAppNameSet {
			requestedAppNames = append(requestedAppNames, appName)
		}

		allApps := b.GetApps()

		topology, err := bosun.GetDependenciesInTopologicalOrder(allApps, requestedAppNames...)

		if err != nil {
			return errors.Errorf("apps could not be sorted in dependency order: %s", err)
		}

		deployDeps := viper.GetBool(ArgAppDeployDeps)

		var toDeploy []*bosun.App

		for _, dep := range topology {
			app, ok := allApps[dep.Name]
			if !ok {
				if requestedAppNameSet[dep.Name] {
					return errors.Errorf("a requested app %q could not be found in the list of apps", dep.Name)
				} else {
					if deployDeps {
						return errors.Errorf("an app specifies a dependency that could not be found: %q from repo %q", dep.Name, dep.Repo)
					} else {
						pkg.Log.Debugf("Skipping missing dependency %q because it was not requested.", dep.Name)
					}
				}
			} else {
				if requestedAppNameSet[dep.Name] || deployDeps {
					toDeploy = append(toDeploy, app)
				} else {
					pkg.Log.Debugf("Skipping dependency %q because it was not requested.", dep.Name)
				}
			}
		}

		for _, app := range toDeploy {
			requested := requestedAppNameSet[app.Name]
			if requested {
				pkg.Log.Infof("App %q will be deployed because it was requested.", app.Name)
			} else {
				pkg.Log.Infof("App %q will be deployed because it was a dependency of a requested app.", app.Name)
			}
		}

		for _, app := range toDeploy {

			envValues := app.GetValuesForEnvironment(env.Name)
			if envValues.Set == nil {
				envValues.Set = make(map[string]*bosun.DynamicValue)
			}
			for k, v := range sets {
				envValues.Set[k] = &bosun.DynamicValue{Value: v}
			}

			app.DesiredState.Status = bosun.StatusDeployed
			if app.DesiredState.Routing == "" {
				app.DesiredState.Routing = bosun.RoutingCluster
			}

			app.DesiredState.Force = viper.GetBool(ArgAppDeployForce)

			err = b.Reconcile(app)

			if err != nil {
				return err
			}
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

		b, err := getBosun()
		if err != nil {
			return err
		}

		services, err := getApps(b, args)
		if err != nil {
			return err
		}

		for _, ms := range services {
			if viper.GetBool(ArgAppDeletePurge) {
				ms.DesiredState.Status = bosun.StatusNotFound
			} else {
				ms.DesiredState.Status = bosun.StatusDeleted
			}

			ms.DesiredState.Routing = bosun.RoutingNA
			err = b.Reconcile(ms)
			if err != nil {
				return err
			}
		}

		err = b.Save()

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

		b, err := getBosun()
		if err != nil {
			return err
		}
		c := b.GetCurrentEnvironment()

		if c.Name != "red" {
			return errors.New("Environment must be set to 'red' to run services.")
		}

		var services []*bosun.App

		services, err = getApps(b, args)
		if err != nil {
			return err
		}

		service := services[0]

		run, err := service.GetRunCommand()
		if err != nil {
			return err
		}

		service.DesiredState.Routing = bosun.RoutingLocalhost

		err = b.Reconcile(service)
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
