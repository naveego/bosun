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
	"github.com/kyokomi/emoji"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var toolsCmd = addCommand(rootCmd, &cobra.Command{
	Use:     "tools",
	Aliases: []string{"tool"},
	Short:   "Commands for listing and installing tools.",
})

var toolsListCmd = addCommand(toolsCmd, &cobra.Command{
	Use:   "list",
	Short: "Lists known tools",
	Run: func(cmd *cobra.Command, args []string) {
		b := mustGetBosun()
		tools := b.GetTools()

		byFromPath := map[string][]bosun.ToolDef{}
		for _, tool := range tools {
			byFromPath[tool.FromPath] = append(byFromPath[tool.FromPath], tool)
		}

		for fromPath, tools := range byFromPath {
			fmt.Printf("Defined in %s:\n", fromPath)
			t := tabby.New()
			t.AddHeader("Name", "Installed", "Location", "Description")

			for _, tool := range tools {

				var installInfo string
				var location string
				executable, installErr := tool.GetExecutable()
				if installErr != nil {
					if tool.Installer != nil {
						if _, ok := tool.GetInstaller(); ok {
							installInfo = emoji.Sprint(":cloud:")
							location = "(installable)"
						} else {
							installInfo = emoji.Sprintf(":x:")
						}
					}
				} else {
					installInfo = emoji.Sprintf(":heavy_check_mark:")
					location = executable
				}

				t.AddLine(tool.Name, installInfo, location, tool.Description)
			}
			t.Print()
			fmt.Println()
		}
	},
})

var toolsInstallCmd = addCommand(toolsCmd, &cobra.Command{
	Use:          "install {tool}",
	Short:        "Installs a tool.",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()
		tools := b.GetTools()
		var tool bosun.ToolDef
		var ok bool
		name := args[0]
		for _, tool = range tools {
			if tool.Name == name {
				ok = true
				break
			}
		}
		if !ok {
			return errors.Errorf("no tool found with name %q", name)
		}

		ctx := b.NewContext()

		installer, ok := tool.GetInstaller()
		if !ok {
			return errors.Errorf("could not get installer for %q", name)
		}

		err := installer.Execute(ctx)

		return err
	},
})

func init() {
	rootCmd.AddCommand(metaUpgradeCmd)
}
