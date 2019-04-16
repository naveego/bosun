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
	"github.com/spf13/cobra"
)

var e2eCmd = addCommand(rootCmd, &cobra.Command{
	Use:   "e2e",
	Short: "Contains sub-commands for running E2E tests.",
})

var e2eListCmd = addCommand(e2eCmd, &cobra.Command{
	Use:   "list",
	Short: "Lists E2E test suites.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()

		suites := b.GetTestSuiteConfigs()

		return printOutput(suites, "name", "description", "fromPath")
	},
})

var e2eRunCmd = addCommand(e2eCmd, &cobra.Command{
	Use:   "run {suite}",
	Short: "Runs an E2E test suite.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()

		suite, err := b.GetTestSuite(args[0])

		if err != nil {
			return err
		}

		ctx := b.NewContext()
		results, err := suite.Run(ctx)

		if err != nil {
			return err
		}

		for _, result := range results {

			colorHeader.Printf("Test: %s  ", result.Name)
			if result.Passed {
				colorOK.Println("PASS")
			} else {
				colorError.Println("FAIL")
			}
			fmt.Printf("Started at: %s\n", result.StartedAt)
			fmt.Printf("Ended at: %s\n", result.EndedAt)
			fmt.Printf("Elapsed time: %s\n", result.Elapsed)
			colorHeader.Printf("Steps:\n")
			t := tabby.New()
			t.AddHeader("Name", "Elapsed", "Result")
			for _, step := range result.Steps {
				name := step.Name
				elapsed := step.Elapsed
				var passed string
				if step.Passed {
					passed = color.GreenString("PASS")
				} else {
					passed = color.RedString("FAIL: %s", step.Error)
				}
				t.AddLine(name, elapsed, passed)
			}

			t.Print()
			fmt.Println()
		}
		return nil
	},
})
