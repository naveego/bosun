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
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"os/signal"
	"strings"
	"text/tabwriter"
	"time"
)

// svcCmd represents the svc command
var svcCmd = &cobra.Command{
	Use:   "svc",
	Short: "Service commands",
}

var svcListCmd = &cobra.Command{
	Use:          "list",
	Aliases:      []string{"ls"},
	Short:        "Lists services",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b, err := getBosun()
		if err != nil {
			return err
		}

		sb := new(strings.Builder)
		w := new(tabwriter.Writer)
		w.Init(sb, 0, 8, 2, '\t', 0)
		fmt.Fprintln(w, "SERVICE\tDEPLOYED\tROUTED TO HOST\t")
		for _, m := range b.GetMicroservices() {
			err = m.LoadActualState()
			if err != nil {
				fmt.Fprintf(w, "%s\tError: %s\t\t\n", m.Config.Name, err)
			} else {
				fmt.Fprintf(w, "%s\t%t\t%t\n", m.Config.Name, m.ActualState.Deployed, m.ActualState.RouteToHost)
			}
		}

		w.Flush()

		fmt.Println(sb.String())

		return nil
	},
}

var svcAddCmd = &cobra.Command{
	Use:          "add [path]",
	Short:        "Adds the microservice at the given path, or the current path. A bosun.yaml file must be in the directory at the given path.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		var dir string
		var err error
		if len(args) == 1 {
			dir = args[0]
		} else {
			dir, err = os.Getwd()
			if err != nil {
				return err
			}
		}
		path, err := findFileInDirOrAncestors(dir, "bosun.yaml")
		if err != nil {
			return err
		}

		b, err := getBosun()
		if err != nil {
			return err
		}

		m, err := b.GetOrAddMicroserviceForPath(path)
		if err != nil {
			return err
		}

		err = b.Save()
		if err != nil {
			return err
		}

		pkg.Log.WithField("name", m.Config.Name).Info("Added microservice.")

		return nil
	},
}

var svcToggleCmd = &cobra.Command{
	Use:          "toggle [service] [service...]",
	Short:        "Toggles or sets where a service will be run from. If service is not specified, the service in the current directory is used.",
	SilenceUsage: true,


	RunE: func(cmd *cobra.Command, args []string) error {

		viper.BindPFlags(cmd.Flags())

		b, err := getBosun()
		if err != nil {
			return err
		}
		c, err := b.GetCurrentEnvironment()
		if err != nil {
			return err
		}
		if c.Name != "red" {
			return errors.New("Environment must be set to 'red' to toggle services.")
		}

		var services []*bosun.Microservice
		if viper.GetBool(ArgSvcToggleAll) {
			if !viper.GetBool(ArgSvcToggleMinikube) && !viper.GetBool(ArgSvcToggleLocalhost) {
				return errors.Errorf("--%s or --%s must be set when using the --% flag",
					ArgSvcToggleLocalhost, ArgSvcToggleMinikube, ArgSvcToggleAll)
			}
			services = b.GetMicroservices()
		} else {
			services, err = getMicroservices(b, args)
		}
		if err != nil {
			return err
		}

		for _, ms := range services {
			wantsLocalhost := viper.GetBool(ArgSvcToggleLocalhost)
			wantsMinikube := viper.GetBool(ArgSvcToggleMinikube)
			if wantsLocalhost {
				ms.DesiredState.RouteToHost = true
			} else if wantsMinikube {
				ms.DesiredState.RouteToHost = false
			} else {
				ms.DesiredState.RouteToHost = !ms.DesiredState.RouteToHost
			}

			if ms.DesiredState.RouteToHost {
				pkg.Log.Info("Configuring to be served from localhost...")
			} else {
				pkg.Log.Info("Configuring to be served from minikube...")
			}

			err = ms.Deploy()

			if err != nil {
				return err
			}
		}

		err = b.Save()

		return err
	},
}

var svcRunCmd = &cobra.Command{
	Use:          "run [service]",
	Short:        "Configures a service to have traffic routed to localhost, then runs the service's run command.",
	SilenceUsage: true,
	SilenceErrors:true,
	RunE: func(cmd *cobra.Command, args []string) error {

		viper.BindPFlags(cmd.Flags())

		b, err := getBosun()
		if err != nil {
			return err
		}
		c, err := b.GetCurrentEnvironment()
		if err != nil {
			return err
		}
		if c.Name != "red" {
			return errors.New("Environment must be set to 'red' to run services.")
		}

		var services []*bosun.Microservice

		services, err = getMicroservices(b, args)
		if err != nil {
			return err
		}

		service := services[0]

		run, err := service.GetRunCommand()
		if err != nil {
			return err
		}

		err = service.LoadActualState()
		if err != nil {
			return err
		}

		if !service.ActualState.RouteToHost {
			service.DesiredState.RouteToHost = true
			err = service.Deploy()
			if err != nil {
				return err
			}
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

const (
	ArgSvcToggleLocalhost = "localhost"
	ArgSvcToggleMinikube  = "minikube"
	ArgSvcToggleAll  = "all"
)

func init() {

	svcCmd.AddCommand(svcToggleCmd)
	svcToggleCmd.Flags().BoolP(ArgSvcToggleLocalhost, "l", false, "Run service at localhost.")
	svcToggleCmd.Flags().BoolP(ArgSvcToggleMinikube, "m",  false, "Run service at minikube.")
	svcToggleCmd.Flags().BoolP(ArgSvcToggleAll, "a", false, "Toggle all known microservices.")

	svcCmd.AddCommand(svcListCmd)
	svcCmd.AddCommand(svcAddCmd)

	svcCmd.AddCommand(svcRunCmd)
	rootCmd.AddCommand(svcCmd)
}
