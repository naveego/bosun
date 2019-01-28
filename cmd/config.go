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
	"bytes"
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
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

var configEditCmd = addCommand(configCmd, &cobra.Command{
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
			targetPath = b.GetRootConfig().Path
		}

		editor, ok := os.LookupEnv("EDITOR")
		if !ok {
			return errors.New("EDITOR environment variable is not set")
		}

		currentBytes, err := ioutil.ReadFile(targetPath)
		if err != nil{
			return err
		}

		stat, err := os.Stat(targetPath)
		if err != nil {
			return errors.Wrap(err, "stat target file")
		}

		tmp, err := ioutil.TempFile(os.TempDir(), "bosun-*.yaml")
		if err != nil {
			return errors.Wrap(err, "temp file")
		}

		_, err = io.Copy(tmp, bytes.NewReader(currentBytes))
		if err != nil {
			return errors.Wrap(err, "copy to temp file")
		}
		err = tmp.Close()
		if err != nil {
			return errors.Wrap(err, "close temp file")
		}

		editorCmd := exec.Command("sh", "-c", fmt.Sprintf("%s %s", editor, tmp.Name()))

		editorCmd.Stderr = os.Stderr
		editorCmd.Stdout = os.Stdout
		editorCmd.Stdin = os.Stdin

		err = editorCmd.Run()
		if err != nil {
			return errors.Errorf("editor command %s failed: %s", editor, err)
		}

		updatedBytes, err := ioutil.ReadFile(tmp.Name())
		if err != nil {
			return errors.Wrap(err, "read updated file")
		}

		if bytes.Equal(currentBytes, updatedBytes) {
			pkg.Log.Info("No changes detected.")
			return nil
		}

		pkg.Log.WithField("path", targetPath).Info("Updating file.")

		err = ioutil.WriteFile(targetPath, updatedBytes, stat.Mode())
		if err != nil {
			return errors.Wrap(err, "write updated file")
		}

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
