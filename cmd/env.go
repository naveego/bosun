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
	"errors"
	"fmt"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {

}

const (
	ArgEnvCurrent = "current"
)

// envCmd represents the env command
var envCmd = addCommand(rootCmd, &cobra.Command{
	Use:     "env [environment]",
	Args:    cobra.ExactArgs(1),
	Short:   "Sets the environment, and outputs a script which will set environment variables in the environment. Should be called using $() so that the shell will apply the script.",
	Long:    "The special environment name `current` will emit the script for the current environment without changing anything.",
	Example: "$(bosun env {env})",
	RunE: func(cmd *cobra.Command, args []string) error {

		b, err := getBosun(bosun.Parameters{NoCurrentEnv: true})
		if err != nil {
			return err
		}

		envName := args[0]
		if envName != "current" {
			err = b.UseEnvironment(envName)
			if err != nil {
				return err
			}
		}

		env := b.GetCurrentEnvironment()

		ctx := b.NewContext()

		err = env.ForceEnsure(ctx)
		if err != nil {
			return err
		}

		err = env.Execute(ctx)
		if err != nil {
			return err
		}

		script, err := env.Render(ctx)
		if err != nil {
			return err
		}

		err = b.Save()
		if err != nil {
			return err
		}

		fmt.Print(script)

		return nil
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(ArgEnvCurrent, false, "Write script for setting current environment.")
})

var envNameCmd = addCommand(envCmd, &cobra.Command{
	Use:   "name",
	Short: "Prints the name of the current environment.",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := bosun.LoadWorkspaceNoImports(viper.GetString(ArgBosunConfigFile))
		if err != nil {
			return err
		}
		if config.CurrentEnvironment == "" {
			return errors.New("no current environment set")
		}
		fmt.Println(config.CurrentEnvironment)
		return nil
	},
})

var envListCmd = addCommand(envCmd, &cobra.Command{
	Use:   "list",
	Short: "Lists environments.",
	Run: func(cmd *cobra.Command, args []string) {
		b := mustGetBosun()
		for _, e := range b.GetEnvironments() {
			fmt.Println(e.Name)
		}
	},
})

var envGetOrCreateCert = addCommand(envCmd, &cobra.Command{
	Use:   "get-cert {name} {part=cert|key} {hosts...}",
	Args:  cobra.MinimumNArgs(3),
	Short: "Creates or reads a certificate for the specified hosts.",
	Long:  "Requires mkcert to be installed.",
	RunE: func(cmd *cobra.Command, args []string) error {

		b := mustGetBosun()

		err := b.EnsureTool("mkcert")
		if err != nil {
			return err
		}

		name := args[0]
		part := args[1]
		hosts := args[2:]

		certName := regexp.MustCompile(`(\W|_)+`).ReplaceAllString(fmt.Sprintf("%n_%n", name, strings.Join(hosts, "_")), "_")

		dir := filepath.Join(os.TempDir(), name)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			os.MkdirAll(dir, 0770)
		}

		certPath := fmt.Sprintf("%s/%s.pem", dir, certName)
		keyPath := fmt.Sprintf("%s/%s.key.pem", dir, certName)

		wantsKey := part == "key"

		cert, err := ioutil.ReadFile(certPath)
		if err == nil && !wantsKey {
			fmt.Println(string(cert))
			return nil
		}
		key, err := ioutil.ReadFile(keyPath)
		if err == nil && wantsKey {
			fmt.Println(string(key))
			return nil
		}

		mkcertArgs := append([]string{"-cert-file", certPath, "-key-file", keyPath}, hosts...)

		out, err := pkg.NewCommand("mkcert", mkcertArgs...).RunOut()
		fmt.Fprintf(os.Stderr, "mkcert output:\n%s\n---- end output\n", out)

		if err != nil {
			return err
		}

		if wantsKey {
			key, err = ioutil.ReadFile(keyPath)
			if err != nil {
				return err
			}
			fmt.Println(string(key))
		} else {
			cert, err = ioutil.ReadFile(certPath)
			if err != nil {
				return err
			}
			fmt.Println(string(cert))
		}

		return nil
	},
})

var _ = addCommand(envCmd, &cobra.Command{
	Use:     "show [name]",
	Args:    cobra.MaximumNArgs(1),
	Aliases: []string{"dump"},
	Short:   "Shows the current environment with its valueSets.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()
		var env *bosun.EnvironmentConfig
		var err error
		if len(args) == 1 {
			env, err = b.GetEnvironment(args[0])
			if err != nil {
				return err
			}
		} else {
			env = b.GetCurrentEnvironment()
		}

		y, err := yaml.Marshal(env)
		if err != nil {
			return err
		}

		valueSets, err := b.GetValueSetsForEnv(env)
		if err != nil {
			return err
		}

		fmt.Println(string(y))
		for _, vs := range valueSets {
			y, err = yaml.Marshal(vs)
			if err != nil {
				return err
			}
			fmt.Println("---")
			fmt.Println(string(y))
		}

		return nil
	},
})

var _ = addCommand(envCmd, &cobra.Command{
	Use:   "value-sets",
	Short: "Lists known value-sets.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()
		for _, vs := range b.GetValueSets() {
			color.Blue("%s ", vs.Name)
			color.White("(from %s):\n", vs.FromPath)
			y, err := yaml.Marshal(vs)
			if err != nil {
				return err
			}
			fmt.Println(string(y))
		}
		return nil
	},
})
