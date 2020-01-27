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
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/templating"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
)

var graylogCmd = &cobra.Command{
	Use:   "graylog",
	Short: "Group of graylog-related commands.",
}

var graylogConfigureCmd = &cobra.Command{
	Use:           "configure {config-file.yaml}",
	Args:          cobra.ExactArgs(1),
	Short:         "Configures graylog using API",
	Aliases:       []string{"config"},
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		vaultClient, err := pkg.NewVaultLowlevelClient("", "")
		if err != nil {
			return err
		}

		values := viper.GetStringSlice(ArgGraylogValues)
		templateArgs, err := templating.NewTemplateValuesFromStrings(values...)
		if err != nil {
			return err
		}

		th := &pkg.TemplateHelper{
			VaultClient:    vaultClient,
			TemplateValues: templateArgs,
		}

		config := &pkg.GraylogConfig{}
		err = th.LoadFromYaml(config, args[0])
		if err != nil {
			return err
		}

		err = config.Apply()

		if err != nil {
			configYaml, _ := yaml.Marshal(config)

			color.Red("Error: \n")
			fmt.Printf("%+v\n", err)

			color.Red("config as rendered:\n")
			fmt.Println(string(configYaml))

		}

		return err
	},
}

const (
	ArgGraylogApiOrigin = "api-origin"
	ArgGraylogUsername  = "username"
	ArgGraylogPassword  = "password"
	ArgGraylogValues    = "set"
)

func init() {

	// graylogConfigureCmd.Flags().String(ArgGraylogApiOrigin, "http://graylog.logging:9000", "The origin of the graylog API")
	// graylogConfigureCmd.MarkFlagRequired(ArgGraylogApiOrigin)
	// graylogConfigureCmd.Flags().String(ArgGraylogUsername, "", "The graylog username")
	// graylogConfigureCmd.MarkFlagRequired(ArgGraylogUsername)
	// graylogConfigureCmd.Flags().String(ArgGraylogPassword, "", "The graylog password")
	//graylogConfigureCmd.MarkFlagRequired(ArgGraylogPassword)
	graylogConfigureCmd.Flags().StringSlice(ArgGraylogValues, []string{}, "Values to pass to the template.")

	graylogCmd.AddCommand(graylogConfigureCmd)

	rootCmd.AddCommand(graylogCmd)
}
