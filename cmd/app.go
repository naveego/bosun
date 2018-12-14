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

// appCmd represents the app command
var appCmd = &cobra.Command{
	Use:   "app",
	Short: "Service commands",
}

var appListCmd = &cobra.Command{
	Use:          "list",
	Aliases:      []string{"ls"},
	Short:        "Lists apps",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b, err := getBosun()
		if err != nil {
			return err
		}

		sb := new(strings.Builder)
		w := new(tabwriter.Writer)
		w.Init(sb, 0, 8, 2, '\t', 0)
		fmt.Fprintln(w, "APP\tDEPLOYED\tROUTED TO HOST\tLABELS\t")
		for _, m := range b.GetMicroservices() {
			err = m.LoadActualState()
			if err != nil {
				fmt.Fprintf(w, "%s\tError: %s\n", m.Config.Name, err)
			} else {
				fmt.Fprintf(w, "%s\t%t\t%t\t%s\n", m.Config.Name, m.ActualState.Deployed, m.ActualState.RouteToHost, strings.Join(m.Config.Labels, ", "))
			}
		}

		w.Flush()

		fmt.Println(sb.String())

		return nil
	},
}


var appToggleCmd = &cobra.Command{
	Use:          "toggle [name] [name...]",
	Short:        "Toggles or sets where traffic for an app will be routed to.",
	Long:"If app is not specified, the first app in the nearest bosun.yaml file is used.",
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


			services, err := getMicroservices(b, args)
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

var appRunCmd = &cobra.Command{
	Use:          "run [app]",
	Short:        "Configures an app to have traffic routed to localhost, then runs the apps's run command.",
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

		var services []*bosun.App

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
	ArgSvcAll             = "all"
	ArgSvcLabels             = "labels"
)

func init() {
	appCmd.PersistentFlags().BoolP(ArgSvcAll, "a", false, "Apply to all known microservices.")
	appCmd.PersistentFlags().StringSliceP(ArgSvcLabels, "L", []string{}, "Apply to microservices with the provided labels.")

	appCmd.AddCommand(appToggleCmd)
	appToggleCmd.Flags().Bool(ArgSvcToggleLocalhost, false, "Run service at localhost.")
	appToggleCmd.Flags().Bool(ArgSvcToggleMinikube,  false, "Run service at minikube.")

	appCmd.AddCommand(appListCmd)

	appCmd.AddCommand(appRunCmd)
	rootCmd.AddCommand(appCmd)
}
