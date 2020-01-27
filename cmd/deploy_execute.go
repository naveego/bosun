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
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/spf13/cobra"
)

func init() {

}

var _ = addCommand(deployCmd, &cobra.Command{
	Use:          "execute {path}",
	Args:         cobra.ExactArgs(1),
	Short:        "Executes a deployment against the current environment.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		req := bosun.ExecuteDeploymentPlanRequest{
			Path: args[0],
		}

		executor := bosun.NewDeploymentPlanExecutor(b, p)

		err = executor.Execute(req)

		return err
	},
}, func(cmd *cobra.Command) {

})
