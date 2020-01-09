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
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/environment"
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
	ArgEnvCluster = "cluster"
)

// envCmd represents the env command
var envCmd = addCommand(rootCmd, &cobra.Command{
	Use:        "env [environment]",
	Args:       cobra.ExactArgs(1),
	Short:      "Sets the environment, and outputs a script which will set environment variables in the environment. Should be called using $() so that the shell will apply the script.",
	Long:       "The special environment name `current` will emit the script for the current environment without changing anything.",
	Deprecated: "This command is deprecated in favor of `bosun env use {environment}`.",
	Example:    "$(bosun env {env})",
	RunE: func(cmd *cobra.Command, args []string) error {

		return useEnvironment(args...)
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(ArgEnvCurrent, false, "Write script for setting current environment.")
	cmd.Flags().StringP(ArgEnvCluster, "c", "", "Set the cluster.")
})

// envCmd represents the env command
var envUseCmd = addCommand(envCmd, &cobra.Command{
	Use:     "use [environment]",
	Args:    cobra.ExactArgs(1),
	Short:   "Sets the environment, and outputs a script which will set environment variables in the environment. Should be called using $() so that the shell will apply the script.",
	Long:    "The special environment name `current` will emit the script for the current environment without changing anything.",
	Example: "$(bosun env {env})",
	RunE: func(cmd *cobra.Command, args []string) error {
		return useEnvironment(args...)
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(ArgEnvCurrent, false, "Write script for setting current environment.")
	cmd.Flags().StringP(ArgEnvCluster, "c", "", "Set the cluster.")
})

func useEnvironment(args ...string) error {

	b, err := getBosun(cli.Parameters{NoEnvironment: true})
	if err != nil {
		return err
	}

	envName := args[0]
	if envName != "current" {
		err = b.UseEnvironment(envName, viper.GetString(ArgEnvCluster))
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
}

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
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := getBosun(cli.Parameters{
			NoEnvironment: true,
		})
		if err != nil {
			return err
		}
		envs, err := b.GetEnvironments()
		if err != nil {
			return err
		}
		for _, e := range envs {
			fmt.Println(e.Name)
		}
		return nil
	},
})

var envGetOrCreateCert = addCommand(envCmd, &cobra.Command{
	Use:   "get-cert {name} {part=cert|key} {hosts...}",
	Args:  cobra.MinimumNArgs(3),
	Short: "Creates or reads a certificate for the specified hosts.",
	Long:  "Requires mkcert to be installed.",
	RunE: func(cmd *cobra.Command, args []string) error {

		b := MustGetBosun()

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

		out, err := pkg.NewShellExe("mkcert", mkcertArgs...).RunOut()
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
		b := MustGetBosun()
		var env *environment.Config
		var err error
		if len(args) == 1 {
			env, err = b.GetEnvironment(args[0])
			if err != nil {
				return err
			}
		} else {
			e := b.GetCurrentEnvironment()
			env = &e.Config
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
		b := MustGetBosun()
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
