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
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

var scriptCmd = &cobra.Command{
	Use:          "script {script-file}",
	Args:         cobra.ExactArgs(1),
	Aliases:[]string{"scripts"},
	Short:        "Run a scripted sequence of commands.",
	Long:         `Provide a script file path.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b, err := getBosun()
		if err != nil {
			return err
		}

		script, err := b.GetScript(args[0])
		if err != nil {
			scriptFilePath := args[0]
			var script bosun.Script
			data, err := ioutil.ReadFile(scriptFilePath)
			if err != nil {
				return err
			}

			err = yaml.Unmarshal(data, &script)
			if err != nil {
				return err
			}
		}

		err = b.Execute(script, scriptStepsSlice...)

		return err
	},
}

var scriptListCmd = &cobra.Command{
	Use:          "list",
	Aliases:[]string{"ls"},
	Short:        "List scripts from current environment.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {


		b, err := getBosun()
		if err != nil {
			return err
		}

		scripts := b.GetScripts()


		if len(scripts) == 0 {
			fmt.Println("No scripts in current environment.")
			return nil
		}

		fmt.Printf("Found %d scripts.\n\n", len(scripts))

		for _, script := range scripts {
			color.New(color.Bold).Println(script.Name)
			color.Blue("FROM: %s\n", script.FromPath)
			color.White("%s\n", script.Description)
			fmt.Println()
		}

		return err
	},
}

const (
	ArgScriptSteps = "steps"
)

var (
	scriptStepsSlice []int
)

func init() {

	scriptCmd.Flags().IntSliceVar(&scriptStepsSlice, ArgScriptSteps, []int{}, "Steps to run (defaults to all steps)")

	scriptCmd.AddCommand(scriptListCmd)

	rootCmd.AddCommand(scriptCmd)
}
