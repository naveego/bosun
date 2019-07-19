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
	"encoding/json"
	"fmt"
	"github.com/naveego/bosun/pkg/util"
	"github.com/oliveagle/jsonpath"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"os"
	"path/filepath"
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

		c := b.GetWorkspace()
		visited := map[string]bool{}

		for path := range c.ImportedBosunFiles {
			visited[path] = true
		}

		paths := util.SortedKeys(visited)

		fmt.Println(c.Path)
		for _, path := range paths {
			fmt.Println(path)
		}

		return nil
	},
})

var configGetCmd = addCommand(workspaceCmd, &cobra.Command{
	Use:   "get {JSONPath}",
	Args:  cobra.ExactArgs(1),
	Short: "Gets a value in the workspace config. Use a dotted path to reference the value.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		ws := b.GetWorkspace()

		//spew.Dump(ws)

		j, err := json.Marshal(ws)
		if err != nil {
			return errors.Wrap(err, "could not marshal workspace")
		}
		var jdata interface{}
		json.Unmarshal(j, &jdata)

		result, err := jsonpath.JsonPathLookup(jdata, args[0])
		if err != nil {
			return err
		}
		err = printOutput(result)
		return err
	},
})

var configSetImports = addCommand(workspaceCmd, &cobra.Command{
	Use:   "set {path} {value}",
	Args:  cobra.ExactArgs(2),
	Short: "Sets a value in the workspace config. Use a dotted path to reference the value.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		err := b.SetInWorkspace(args[0], args[1])
		return err
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
			data, _ := yaml.Marshal(app.AppConfig)
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

var configClearCmd = addCommand(workspaceCmd, &cobra.Command{
	Use:   "clear-imports",
	Short: "Removes all imports.",
	RunE: func(cmd *cobra.Command, args []string) error {

		b, err := getBosun()
		if err != nil {
			return err
		}

		b.ClearImports()

		err = b.Save()

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

		b := MustGetBosun()
		b.TidyWorkspace()

		if viper.GetBool(ArgGlobalDryRun) {
			b.NewContext().Log.Warn("Detected dry run flag, no changes will be saved.")
			return nil
		}

		return b.Save()
	},
})
