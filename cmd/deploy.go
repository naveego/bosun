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
	"github.com/AlecAivazis/survey/v2"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/spf13/cobra"
)

func init() {

}

var deployCmd = addCommand(rootCmd, &cobra.Command{
	Use:   "deploy",
	Short: "Contains commands for planning and executing a deploy.",
})

func userChooseApps(message string, apps []string) []string {
	const invertKey = "Invert (select apps to exclude)"
	options := append([]string{invertKey}, apps...)
	var selected []string
	prompt := &survey.MultiSelect{
		Message: message,
		Options: options,
	}

	check(survey.AskOne(prompt, &selected))

	if len(selected) == 0 {
		return []string{}
	}

	selections := map[string]bool{}
	selectedMeansInclude := true
	for _, s := range selected {
		if s == invertKey {
			selectedMeansInclude = false
			continue
		}
		selections[s] = true
	}
	var out []string
	for _, o := range apps {
		if selectedMeansInclude && selections[o] {
			out = append(out, o)
		} else if !selectedMeansInclude && !selections[o] {
			out = append(out, o)
		}
	}

	return out
}

func userChooseStringWithDefault(message string, value string) string {
	prompt := &survey.Input{
		Message: message,
		Default: value,
	}
	check(survey.AskOne(prompt, &value))
	return value
}

func userChooseProvider(provider string) string {
	if provider != "" {
		return provider
	}
	prompt := &survey.Select{
		Message: "Choose a provider",
		Options: []string{bosun.SlotStable, bosun.SlotUnstable, bosun.SlotCurrent, bosun.WorkspaceProviderName},
	}
	check(survey.AskOne(prompt, &provider))
	return provider
}
