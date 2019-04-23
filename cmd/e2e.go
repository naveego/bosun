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
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
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
		err := viper.BindPFlags(cmd.Flags())

		if err != nil {
			return err
		}

		b := mustGetBosun()

		suite, err := b.GetTestSuite(args[0])

		if err != nil {
			return err
		}

		e2eCtx := bosun.E2EContext{
			SkipSetup:    viper.GetBool(ArgE2ERunSkipSetup),
			SkipTeardown: viper.GetBool(ArgE2ERunSkipTeardown),
			Tests:        viper.GetStringSlice(ArgE2ERunTests),
		}
		ctx := bosun.WithE2EContext(b.NewContext(), e2eCtx)
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
			t := tablewriter.NewWriter(os.Stdout)
			t.SetHeader([]string{"Name", "Elapsed", "Pass", "Fail"})
			t.SetColumnColor(
				tablewriter.Color(tablewriter.FgHiWhiteColor),
				tablewriter.Color(tablewriter.FgHiWhiteColor),
				tablewriter.Color(tablewriter.FgGreenColor),
				tablewriter.Color(tablewriter.FgRedColor))
			t.SetAutoWrapText(true)
			t.SetReflowDuringAutoWrap(true)
			t.SetColWidth(100)
			for _, step := range result.Steps {
				name := step.Name
				elapsed := step.Elapsed
				pass := ""
				fail := ""
				if step.Passed {
					pass = "PASS"
				} else {
					fail = step.Error
				}
				t.Append([]string{name, elapsed, pass, fail})
			}

			t.Render()
			fmt.Println()
		}
		return nil
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().StringSlice(ArgE2ERunTests, []string{}, "Specific tests to run.")
	cmd.Flags().Bool(ArgE2ERunSkipSetup, false, "Skip setup scripts.")
	cmd.Flags().Bool(ArgE2ERunSkipTeardown, false, "Skip teardown scripts.")
})

const (
	ArgE2ERunTests        = "tests"
	ArgE2ERunSkipSetup    = "skip-setup"
	ArgE2ERunSkipTeardown = "skip-teardown"
)
