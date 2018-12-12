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
	"github.com/spf13/cobra"
)

// envCmd represents the env command
var envCmd = &cobra.Command{
	Use:   "env {environment}",
	Args:  cobra.ExactArgs(1),
	Short: "Outputs a script which will configure the environment. Should be called using $() so that the shell will apply the script.",
	Example: "$(bosun env {env})",
	RunE: func(cmd *cobra.Command, args []string) error {

		b, err := getBosun()
		if err != nil {
			return err
		}

		envName := args[0]

		err = b.SetCurrentEnvironment(envName)
		if err != nil {
			return err
		}


		env, err := b.GetCurrentEnvironment()
		if err != nil {
			return err
		}

		err = env.Execute()
		if err != nil {
			return err
		}

		script, err := env.Render()
		if err != nil {
			return err
		}

		err = b.Save()
		if err != nil {
			return err
		}

		fmt.Print(script)

		return nil
	},
}


func init() {

	rootCmd.AddCommand(envCmd)
}
