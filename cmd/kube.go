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
	"encoding/base64"
	"fmt"
	"github.com/fatih/color"

	"github.com/naveego/bosun/pkg"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// kubeCmd represents the kube command
var kubeCmd = &cobra.Command{
	Use:   "kube {kube-layout}",
	Args:  cobra.ExactArgs(1),
	Short: "Group of commands wrapping kubectl.",
	Long:  `You must have the cluster set in kubectl.`,
}

var dashboardTokenCmd = &cobra.Command{
	Use:   "dashboard-token",
	Short: "Writes out a dashboard UI access token.",
	Long:  `You must have the cluster set in kubectl.`,
	RunE: func(cmd *cobra.Command, args []string) error {

		secretName, err := pkg.NewCommand("kubectl get serviceaccount kubernetes-dashboard-user -n kube-system -o jsonpath={.secrets[0].name}").RunOut()
		if err != nil {
			return err
		}

		b64, err := pkg.NewCommand(fmt.Sprintf("kubectl get secret %s -n kube-system -o jsonpath={.data.token}", secretName)).RunOut()
		if err != nil {
			return err
		}

		token, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return err
		}

		fmt.Println(string(token))

		return err
	},
}

var pullSecretForce bool

const(
	ArgKubePullSecretUsername = "pull-secret-username"
	ArgKubePullSecretPassword = "pull-secret-password"
	ArgKubePullSecretPasswordLpassPath = "pull-secret-password-path"
)

var pullSecretCmd = &cobra.Command{
	Use:   "pull-secret [username] [password]",
	Args:  cobra.RangeArgs(0, 2),
	Short: "Sets a pull secret in kubernetes for https://docker.n5o.black.",
	Long:  `If username or password parameter is not provided you will be prompted.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error
		viper.BindPFlags(cmd.Flags())

		force := viper.GetBool("force")
		if !force {
			out, err := pkg.NewCommand("kubectl get secret docker-n5o-black").RunOut()
			fmt.Println(out)
			if err == nil {
				color.Yellow("Pull secret already exists (run with --force parameter to overwrite).")
				return nil
			}
		}

		var username string
		var password string

		if len(args) > 0 {
			username = args[0]
		} else if viper.GetString(ArgKubePullSecretUsername) != "" {
			username = viper.GetString(ArgKubePullSecretUsername)
		} else {
			username = pkg.RequestStringFromUser("Please provide username")
		}

		if len(args) == 2 {
			password = args[1]
		} else if viper.GetString(ArgKubePullSecretPassword) != "" {
			password = viper.GetString(ArgKubePullSecretPassword)
		} else if viper.GetString(ArgKubePullSecretPasswordLpassPath) != "" {
			path :=  viper.GetString(ArgKubePullSecretPasswordLpassPath)
			pkg.Log.WithField("path", path ).Info("Trying to get password from LastPass.")
			password, err = pkg.NewCommand("lpass", "show", "--password", path).RunOut()
			if err != nil {
				return err
			}
		} else {
			password = pkg.RequestSecretFromUser("Please provide password for user %s", username)
		}

		err = pkg.NewCommand("kubectl",
				"create", "secret", "docker-registry",
				"docker-n5o-black",
				"--docker-server=https://docker.n5o.black",
				fmt.Sprintf("--docker-username=%s", username),
				fmt.Sprintf("--docker-password=%s", password),
		).RunE()
		if err != nil {
			return err
		}

		err = pkg.NewCommand("kubectl",
				"create", "secret", "docker-registry",
				"--namespace=kube-system",
				"docker-n5o-black",
				"--docker-server=https://docker.n5o.black",
				fmt.Sprintf("--docker-username=%s", username),
				fmt.Sprintf("--docker-password=%s", password),).RunE()

		return err
	},
}

func init() {
	kubeCmd.AddCommand(dashboardTokenCmd)

	pullSecretCmd.Flags().BoolVarP(&pullSecretForce, "force", "f", false, "Force create (overwrite) the secret even if it already exists.")
	pullSecretCmd.Flags().String(ArgKubePullSecretUsername, "", "User for pulling from docker harbor.")
	pullSecretCmd.Flags().String(ArgKubePullSecretPassword, "", "Secret password for pulling from docker harbor.")
	pullSecretCmd.Flags().String(ArgKubePullSecretPasswordLpassPath, "", "Path in LastPass for the password for pulling from docker harbor.")
	kubeCmd.AddCommand(pullSecretCmd)

	rootCmd.AddCommand(kubeCmd)
}
