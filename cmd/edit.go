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
	"io"
	"io/ioutil"
	"os"
	"os/exec"
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
