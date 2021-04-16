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
	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/tenant"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"strings"
)

var tenantCmd = addCommand(rootCmd, &cobra.Command{
	Use:   "tenant",
	Short: "Commands for interacting with tenants.",
})

var tenantCloneCmd = addCommand(tenantCmd, &cobra.Command{
	Use:   "clone {planning-directory} [edit|validate|export|import]",
	Args:  cobra.RangeArgs(1, 2),
	Short: "Clones a tenant",
	RunE: func(cmd *cobra.Command, args []string) error {

		log := core.Log.WithField("cmd", "tenant clone")
		planner, err := tenant.NewPlanner(args[0], log)
		if err != nil {
			return err
		}

		var action string
		var interactive bool
		switch len(args) {
		case 1:
			interactive = true
		case 2:
			interactive = false
			action = strings.ToLower(args[1])
		}

		for {
			if interactive {
				prompt := &survey.Select{
					Message: "Choose a command:",
					Options: []string{"Edit", "Validate", "Export", "Import", "Quit"},
				}
				err = survey.AskOne(prompt, &action)
				if err != nil {
					return err
				}
			}

			switch strings.ToLower(action) {
			case "quit":
				return nil
			case "edit":
				err = planner.EditPlan()
			case "validate":
				err = planner.ValidatePlan()
			case "export":
				err = planner.PerformExport()
			case "import":
				var confirmed bool
				_ = survey.AskOne(&survey.Confirm{
					Message: fmt.Sprintf("This will delete all jobs, connections, schemas, and shapes from the %q tenant. Are you sure you want to procede?", planner.Plan.ToTenant),
				}, &confirmed)
				if !confirmed {
					return errors.New("cancelled")
				}

				err = planner.PerformImport()
			default:
				err = errors.Errorf("invalid command %q", action)
			}

			if interactive {
				if err != nil {
					color.Red("%s\n", err)
				}
			} else {
				return err
			}
		}

	},
})
