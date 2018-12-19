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
	"github.com/naveego/bosun/pkg"
	"github.com/spf13/cobra"
)


func init() {

	lpassCmd.AddCommand(lpassPasswordCmd)

	rootCmd.AddCommand(lpassCmd)
}

// lpassCmd represents the lpass command
var lpassCmd = &cobra.Command{
	Use:   "lpass",
	Aliases:[]string{"lastpass"},
	Args:  cobra.ExactArgs(1),
	Short: "Root command for LastPass commands.",
}


var lpassPasswordCmd = &cobra.Command{
	Use:   "password {folder/name} {username} {url}",
	Short: "Gets (or generates if not found) a password in LastPass.",
	Args:cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {

		name := args[0]
		username:=args[1]
		url:=args[2]

		password, err := pkg.NewCommand("lpass", "show", "--sync=now", "-p", "--basic-regexp", name).RunOut()
		if err == nil {
			fmt.Println(password)
			return nil
		}

		pkg.Log.Debug("Password %q does not yet exist; it will be generated.", name)

		password, err = pkg.NewCommand("lpass", "generate", "--sync=now", "--no-symbols", "--username", username, "--url", url, name, "30").RunOut()
		if err == nil {
			fmt.Println(password)
		}

		return err
	},
}
