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
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
)

const (
	ArgIssueParent   = "parents"
	ArgIssueChildren = "children"
)

var issueCmd = addCommand(rootCmd, &cobra.Command{
	Use:   "issue",
	Short: "Commands related to issues.",
})

var issueShowCmd = addCommand(issueCmd, &cobra.Command{
	Use:   "show {ref: org/repo#number}",
	Args:  cobra.ExactArgs(1),
	Short: "Shows info about an issue.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun(bosun.Parameters{ProviderPriority: []string{bosun.WorkspaceProviderName}})

		svc, err := b.GetIssueService()
		if err != nil {
			return err
		}

		ref, err := issues.ParseIssueRef(args[0])

		issue, err := svc.GetIssue(ref)
		if err != nil {
			return err
		}

		headerWriter := color.New(color.FgBlue)
		_, _ = headerWriter.Fprintf(os.Stdout, "Issue:\n")

		err = renderOutput(issue)
		if err != nil {
			return err
		}

		if viper.GetBool(ArgIssueParent) {
			refs, err := svc.GetParentRefs(ref)
			if err != nil {
				return err
			}
			parents, err := issues.GetIssuesFromRefs(svc, refs)
			if err != nil {
				return err
			}
			_, _ = headerWriter.Fprintf(os.Stdout, "Parents:\n")
			err = renderOutput(parents)
			if err != nil {
				return err
			}
		}

		if viper.GetBool(ArgIssueChildren) {
			refs, err := svc.GetChildRefs(ref)
			if err != nil {
				return err
			}
			parents, err := issues.GetIssuesFromRefs(svc, refs)
			if err != nil {
				return err
			}
			_, _ = headerWriter.Fprintf(os.Stdout, "Children:\n")
			err = renderOutput(parents)
			if err != nil {
				return err
			}
		}

		return nil
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(ArgIssueParent, false, "Show parent.")
	cmd.Flags().Bool(ArgIssueChildren, false, "Show children.")
})

var issueListCmd = addCommand(issueCmd, &cobra.Command{
	Use:   "show {ref: org/repo#number}",
	Args:  cobra.ExactArgs(1),
	Short: "Shows info about an issue.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun(bosun.Parameters{ProviderPriority: []string{bosun.WorkspaceProviderName}})

		svc, err := b.GetIssueService()
		if err != nil {
			return err
		}

		ref, err := issues.ParseIssueRef(args[0])

		issue, err := svc.GetIssue(ref)
		if err != nil {
			return err
		}

		headerWriter := color.New(color.FgBlue)
		_, _ = headerWriter.Fprintf(os.Stdout, "Issue:\n")

		err = renderOutput(issue)
		if err != nil {
			return err
		}

		if viper.GetBool(ArgIssueParent) {
			refs, err := svc.GetParentRefs(ref)
			if err != nil {
				return err
			}
			parents, err := issues.GetIssuesFromRefs(svc, refs)
			if err != nil {
				return err
			}
			_, _ = headerWriter.Fprintf(os.Stdout, "Parents:\n")
			err = renderOutput(parents)
			if err != nil {
				return err
			}
		}

		if viper.GetBool(ArgIssueChildren) {
			refs, err := svc.GetChildRefs(ref)
			if err != nil {
				return err
			}
			parents, err := issues.GetIssuesFromRefs(svc, refs)
			if err != nil {
				return err
			}
			_, _ = headerWriter.Fprintf(os.Stdout, "Children:\n")
			err = renderOutput(parents)
			if err != nil {
				return err
			}
		}

		return nil
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(ArgIssueParent, false, "Show parent.")
	cmd.Flags().Bool(ArgIssueChildren, false, "Show children.")
})
