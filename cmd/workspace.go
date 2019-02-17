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
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"os"
	"path/filepath"
	"strings"
)

func init() {

}

var workspaceCmd = addCommand(rootCmd, &cobra.Command{
	Use:     "workspace",
	Aliases: []string{"ws", "config"},
	Short:   "Workspace commands configure and manipulate the bindings between app repos and your local machine.",
	Long: `A workspace contains the core configuration that is used when bosun is run.
It stores the current environment, the current release (if any), a listing of imported bosun files,
the apps discovered in them, and the current state of those apps in the workspace.
The app state includes the location of the app on the file system (for apps which have been cloned)
and the minikube deploy status of the app.

A workspace is based on a workspace config file. The default location is $HOME/.bosun/bosun.yaml,
but it can be overridden by setting the BOSUN_CONFIG environment variable or passing the --config-file flag.`,
})

var configShowCmd = addCommand(workspaceCmd, &cobra.Command{
	Use:   "show",
	Short: "Shows various config components.",
})

var configShowImportsCmd = addCommand(configShowCmd, &cobra.Command{
	Use:   "imports",
	Short: "Prints the imports.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := getBosun()
		if err != nil {
			return err
		}

		c := b.Getworkspace()
		visited := map[string]bool{}

		var visit func(path string, depth int, last bool)
		visit = func(path string, depth int, last bool) {
			if file, ok := c.ImportedBosunFiles[path]; ok {
				symbol := "├─"
				if last {
					symbol = "└─"
				}
				fmt.Printf("%s%s%s\n", strings.Repeat(" ", depth), symbol, path)

				if visited[path] {
					return
				}

				visited[path] = true

				for i, importPath := range file.Imports {
					if !filepath.IsAbs(importPath) {
						importPath = filepath.Join(filepath.Dir(path), importPath)
					}
					visit(importPath, depth+1, i + 1 >= len(file.Imports))
				}
			} else {

			}
		}

		fmt.Println(c.Path)
		for i, path := range c.Imports {
			visit(path, 0, i + 1 == len(c.Imports))
		}

		return nil
	},
})

var configDumpImports = addCommand(configShowCmd, &cobra.Command{
	Use:     "workspace",
	Aliases: []string{"ws"},
	Short:   "Prints the workspace config.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := getBosun()
		if err != nil {
			return err
		}

		c := b.Getworkspace()
		data, _ := yaml.Marshal(c)

		fmt.Println(string(data))

		return nil
	},
})

var configDumpCmd = addCommand(workspaceCmd, &cobra.Command{
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

var configImportCmd = addCommand(workspaceCmd, &cobra.Command{
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

var wsTidyPathsCmd = addCommand(workspaceCmd, &cobra.Command{
	Use:   "tidy",
	Short: "Cleans up workspace.",
	Long: `Cleans up workspace by:
- Removing redundant imports.
- Finding apps which have been cloned into registered git roots.
- Other things as we think of them...
`,
	RunE: func(cmd *cobra.Command, args []string) error {

		b := mustGetBosun()
		b.TidyWorkspace()

		if viper.GetBool(ArgGlobalDryRun) {
			b.NewContext().Log.Warn("Detected dry run flag, no changes will be saved.")
			return nil
		}

		return b.Save()
	},
})
