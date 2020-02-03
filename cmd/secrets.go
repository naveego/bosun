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
	"github.com/AlecAivazis/survey/v2"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/environment"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"log"
)

var secretsCmd = addCommand(rootCmd, &cobra.Command{
	Use:     "secrets",
	Aliases: []string{"secret"},
	Short:   "Root command for managing secrets commands.",
})

var secretsListCmd = addCommand(secretsCmd, &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "Lists secrets in current environment.",
	RunE: func(cmd *cobra.Command, args []string) error {

		b := MustGetBosun()
		e := b.GetCurrentEnvironment()

		secretGroups, err := e.GetSecretGroupConfigs()
		if err != nil {
			return err
		}
		for _, group := range secretGroups {
			fmt.Printf("%s:\n", group.Name)
			for _, secret := range group.Secrets {
				fmt.Printf("\t%s\n", secret.Name)
			}
		}
		return nil
	},
})

var secretsAddGroupCmd = addCommand(secretsCmd, &cobra.Command{
	Use:   "add-group {name}",
	Args:  cobra.ExactArgs(1),
	Short: "Adds a secret group to the current environment.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return addSecretGroup(args[0])
	},
})

func addSecretGroup(groupName string) error {
	b := MustGetBosun()
	e := b.GetCurrentEnvironment()

	var passwordStrategyIndex int
	prompt := &survey.Select{
		Message: "How do you want to provide the password?",
		Options: []string{
			"Enter password",
			"Use lastpass",
			"Store insecurely in group (only for dev environments)",
		},
	}
	check(survey.AskOne(prompt, &passwordStrategyIndex))

	keyConfig := &environment.SecretKeyConfig{}

	switch passwordStrategyIndex {
	case 0:
		keyConfig.Prompt = true
	case 1:
		var lastpassPath, lastpassField string
		check(survey.AskOne(&survey.Input{
			Message: "What is the path in lastpass where the password is stored?",
		}, &lastpassPath))
		check(survey.AskOne(&survey.Input{
			Message: "What is the field containing the password?",
			Default: "password",
		}, &lastpassField))
		keyConfig.Lastpass = &environment.LastpassKeyConfig{
			Path:  lastpassPath,
			Field: lastpassField,
		}
	case 2:
		if !e.IsLocal {
			return errors.New("Insecure storage of the key only allowed in local environments.")
		}

		var password string
		check(survey.AskOne(&survey.Input{
			Message: "Enter the password to save in the file",
		}, &password))
		keyConfig.UnsafeStoredPassphrase = password
	default:
		panic("invalid response")
	}

	err := e.AddSecretGroup(groupName, keyConfig)

	return err
}

var secretsSetCmd = addCommand(secretsCmd, &cobra.Command{
	Use:   "add {group} {name} [value]",
	Args:  cobra.RangeArgs(2, 3),
	Short: "Sets a secret value. You will be prompted to set the secret if not provided.",
	RunE: func(cmd *cobra.Command, args []string) error {

		b := MustGetBosun()
		e := b.GetCurrentEnvironment()

		var secret string
		if len(args) == 3 {
			secret = args[2]
		} else {
			secret = cli.RequestSecretFromUser("Secret")
		}

		groupName := args[0]
		secretName := args[1]

		_, err := e.GetSecretGroupConfig(groupName)
		if err != nil {
			err = addSecretGroup(groupName)
			if err != nil {
				return err
			}
		}

		err = e.AddOrUpdateSecretValue(groupName, secretName, secret)
		if err != nil {
			return err
		}
		log.Printf("Added or updated secret %s in group %s.", args[1], args[0])

		return nil
	},
})

var secretsShowCmd = addCommand(secretsCmd, &cobra.Command{
	Use:   "show { secretPath | {group} {secret} }",
	Args:  cobra.RangeArgs(1, 2),
	Short: "Shows a secret",
	RunE: func(cmd *cobra.Command, args []string) error {

		b := MustGetBosun()
		e := b.GetCurrentEnvironment()

		switch len(args){
		case 1:
			secret, err := e.ResolveSecretPath(args[0])
			if err != nil {
				return err
			}
			fmt.Println(secret)
		case 2:
			secret, err := e.GetSecretValue(args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Println(secret)
		}

		return nil
	},
})
