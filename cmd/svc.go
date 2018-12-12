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
)

// svcCmd represents the svc command
var svcCmd = &cobra.Command{
	Use:   "svc",
	Short: "Service commands",
}

var svcListCmd = &cobra.Command{
	Use:   "list",
	Aliases:[]string{"ls"},
	Short: "Lists services",
	SilenceUsage:true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b, err := getBosun()
		if err != nil {
			return err
		}

		for _, m := range b.GetMicroservices() {
			fmt.Printf("%s  	%t\n", m.Config.Name, m.DesiredState.RouteToHost)
		}
		return nil
	},
}

var svcAddCmd = &cobra.Command{
	Use:   "add [path]",
	Short: "Adds the microservice at the given path, or the current path. A bosun.yaml file must be in the directory at the given path.",
	SilenceUsage:true,
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
	Use:   "toggle [service]",
	Args:  cobra.MaximumNArgs(1),
	Short: "Toggles or sets where the service will be run from.",
	SilenceUsage:true,
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
		var ms *bosun.Microservice
		if len(args) == 1 {
			ms, err = b.GetMicroservice(args[0])
		} else {
			wd, _ := os.Getwd()
			bosunFile, err := findFileInDirOrAncestors(wd, "bosun.yaml")
			if err != nil {
				return err
			}

			ms, err = b.GetOrAddMicroserviceForPath(bosunFile)
		}

		if err != nil {
			return err
		}

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

		err = b.Save()

		return err
	},
}

const (
	ArgSvcToggleLocalhost = "localhost"
	ArgSvcToggleMinikube = "minikube"
)

func init() {

	svcCmd.AddCommand(svcToggleCmd)
	svcToggleCmd.Flags().Bool(ArgSvcToggleLocalhost, false, "Run service at localhost.")
	svcToggleCmd.Flags().Bool(ArgSvcToggleMinikube, false, "Run service at minikube.")

	svcCmd.AddCommand(svcListCmd)
	svcCmd.AddCommand(svcAddCmd)

	rootCmd.AddCommand(svcCmd)
}
