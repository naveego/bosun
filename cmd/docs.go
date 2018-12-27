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
	"github.com/spf13/cobra/doc"
	"os"
)

func init() {
	rootCmd.AddCommand(docsCmd)
}

var docsCmd = &cobra.Command{
	Use:   "docs",
	ArgAliases:   []string{"doc"},
	Short: "Completion and documentation generators.",
}

var _ = addCommand(docsCmd, &cobra.Command{
	Use:   "markdown [dir]",
	Short: "Output documentation in markdown. Output dir defaults to ./docs",
	RunE: func(cmd *cobra.Command, args []string) error{
		dir := "./docs"
		if len(args) > 0 {
			dir = args[0]
		}
		err := doc.GenMarkdownTree(rootCmd, dir)
		if err != nil {
			fmt.Printf("Output to %q.\n", dir)
		}
		return err
	},
})

var _ = addCommand(docsCmd, &cobra.Command{
	Use:   "bash",
	Short: "Completion generator for bash.",
	Run: func(cmd *cobra.Command, args []string) {
		rootCmd.GenBashCompletion(os.Stdout)
	},
})

var _ = addCommand(docsCmd, &cobra.Command{
	Use:   "bash",
	Short: "Completion generator for bash.",
	Run: func(cmd *cobra.Command, args []string) {
		rootCmd.GenBashCompletion(os.Stdout)
	},
})
