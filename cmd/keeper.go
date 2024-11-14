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
	jsoniter "github.com/json-iterator/go"
	"github.com/naveego/bosun/pkg/command"
	"github.com/spf13/cobra"
)

// keeperCmd represents the keeper command
var keeperCmd = addCommand(rootCmd, &cobra.Command{
	Use:     "keeper",
	Aliases: []string{"keeper"},
	Args:    cobra.ExactArgs(1),
	Short:   "Root command for Keeper commands.",
})

var keeperPasswordCmd = addCommand(keeperCmd, &cobra.Command{
	Use:   "password {folder/name}",
	Short: "Gets a password in Keeper.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {

		name := args[0]

		password, err := command.NewShellExe("keeper", "find-password", name).RunOut()
		if err == nil {
			fmt.Println(password)
			return nil
		}

		return err
	},
})

var keeperExecCred = addCommand(keeperCmd, &cobra.Command{
	Use:   "execcred {path}",
	Short: "Gets a password from Keeper and returns it in kubeconfig ExecCredential format.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {

		name := args[0]
		password, err := command.NewShellExe("keeper", "find-password", name).RunOut()
		if err != nil {
			return err
		}

		execCred := map[string]interface{}{
			"apiVersion": "client.authentication.k8s.io/v1beta1",
			"kind":       "ExecCredential",
			"status": map[string]interface{}{
				"token": password,
			},
		}

		json, _ := jsoniter.Marshal(execCred)

		fmt.Println(string(json))

		return nil
	},
})
