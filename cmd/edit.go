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
	"github.com/naveego/bosun/pkg/cli"
	"github.com/spf13/cobra"
)

var editCmd = addCommand(rootCmd, &cobra.Command{
	Use:   "edit [app]",
	Short: "Edits your root config, or the config of an app if provided.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := getBosun()
		if err != nil {
			return err
		}

		var targetPath string

		if len(args) == 1 {
			app, err := b.GetApp(args[0])
			if err != nil {
				return err
			}
			targetPath = app.FromPath
		} else {
			targetPath = b.GetWorkspace().Path
		}

		return cli.Edit(targetPath)
	},
})
