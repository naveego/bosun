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
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
)

func init() {

}

var _ = addCommand(deployCmd, &cobra.Command{
	Use:          "validate {path | release}",
	Args:         cobra.ExactArgs(1),
	Short:        "Validates a deployment plan.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		deploymentPlanPath := args[0]
		if deploymentPlanPath == "release" {
			release, err := p.GetCurrentRelease()
			if err != nil {
				return err
			}
			deploymentPlanPath = filepath.Join(p.GetDeploymentsDir(), fmt.Sprintf("%s/plan.yaml", release.Version.String()))
		}

		plan, err := bosun.LoadDeploymentPlanFromFile(deploymentPlanPath)

		if err != nil {
			return err
		}

		executor := bosun.NewDeploymentPlanExecutor(b, p)
		req := bosun.ExecuteDeploymentPlanRequest{
			Plan:plan,
			Validate:true,
			ValidateOnly:true,
			UseSudo:viper.GetBool(ArgGlobalSudo),
		}

		result, err := executor.Execute(req)

		if err != nil {
			return err
		}

		errCount := 0
		t := tablewriter.NewWriter(os.Stdout)
		t.SetHeader([]string{"app", "result"})
		for _, app := range plan.Apps {
			validationError := result.ValidationErrors[app.Name]

			resultText := ""
			if validationError == "" {
				resultText = color.GreenString("OK")
			} else {
				resultText = color.RedString(validationError)
				errCount++
			}
			t.Append([]string{app.Name, resultText})
		}

		t.Render()

		if errCount > 0 {
			return errors.New("one or more apps were invalid")
		}

		return nil
	},
}, func(cmd *cobra.Command) {
	})

