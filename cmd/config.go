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
	"gopkg.in/yaml.v2"
	"os"
	"path/filepath"
)

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config {c}",
	Args:  cobra.ExactArgs(1),
	Short: "Root command for configuring bosun.",
}


func init() {
	rootCmd.AddCommand(configCmd)
}

var configShowCmd = addCommand(configCmd, &cobra.Command{
	Use:"show",
	Short:"Shows various config components.",
})

var configShowImportsCmd = addCommand(configShowCmd, &cobra.Command{
	Use:   "imports",
	Short: "Prints the imports config from ~/.bosun.yaml and other files.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := getBosun()
		if err != nil {
			return err
		}

		c := b.GetRootConfig()
		for _, i := range c.Imports {
			fmt.Println(i)
		}

		return nil
	},
})

var configDumpImports = addCommand(configShowCmd, &cobra.Command{
	Use:   "root",
	Short: "Prints the root config from ~/.bosun.yaml.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := getBosun()
		if err != nil {
			return err
		}

		c := b.GetRootConfig()
		data, _ := yaml.Marshal(c)

		fmt.Println(string(data))

		return nil
	},
})

var configDumpCmd = addCommand(configCmd, &cobra.Command{
	Use:   "dump [app]",
	Short: "Prints current merged config, or the config of an app.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := getBosun()
		if err != nil {
			return err
		}

		if len(args) == 1 {
			app, err := b.GetApp(args[0])
			if err != nil {
				return err
			}
			data, _ := yaml.Marshal(app.AppRepoConfig)
			fmt.Println(string(data))
			return nil
		}

		c := b.GetMergedConfig()
		data, _ := yaml.Marshal(c)

		fmt.Println(string(data))

		return nil
	},
})

var configImportCmd = addCommand(configCmd, &cobra.Command{
	Use:     "import [file]",
	Aliases: []string{"include", "add"},
	Args:    cobra.MaximumNArgs(1),
	Short:   "Includes the file in the user's bosun.yaml. If file is not provided, searches for a bosun.yaml file in this or a parent directory.",
	RunE: func(cmd *cobra.Command, args []string) error {

		var filename string
		var err error
		switch len(args) {
		case 0:
			wd, _ := os.Getwd()
			filename, err = findFileInDirOrAncestors(wd, "bosun.yaml")
		case 1:
			filename, err = filepath.Abs(args[0])
		}

		if err != nil {
			return err
		}

		_, err = os.Stat(filename)
		if err != nil {
			return err
		}

		b, err := getBosun()
		if err != nil {
			return err
		}

		if !b.AddImport(filename) {
			fmt.Printf("File %s is already imported in user config.\n", filename)
			return nil
		}

		err = b.Save()

		if err != nil {
			return err
		}

		fmt.Printf("Added %s to imports in user config.\n", filename)

		return err
	},
})
