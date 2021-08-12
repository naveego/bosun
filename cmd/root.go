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
	"github.com/naveego/bosun/pkg/core"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"runtime/pprof"
	"strings"

	"os"

	"github.com/spf13/viper"

	"github.com/spf13/cobra"
)

var cfgFile string

var step int

// rootCmd represents the base command when called without any subcommands
var rootCmd = TraverseRunHooks(&cobra.Command{
	Use:           "bosun",
	Short:         "Devops tool.",
	SilenceErrors: true,
	Version: fmt.Sprintf(`
Version: %s
Timestamp: %s
GetCurrentCommit: %s
`, core.Version, core.Timestamp, core.Commit),
	Long: `This is our tool for for devops. If you have some scripts for
building, deploying, or monitoring apps you may want to add them to this tool.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {

		viper.RegisterAlias("debug", "verbose")
		viper.BindPFlags(cmd.Flags())
		viper.BindPFlags(cmd.PersistentFlags())

		logrus.SetFormatter(&logrus.TextFormatter{
			DisableTimestamp: true,
			ForceColors:      true,
		})

		core.Log = logrus.NewEntry(logrus.StandardLogger())

		verbose := viper.GetBool(ArgGlobalVerbose)
		if viper.GetBool(ArgGlobalTrace) {
			logrus.SetLevel(logrus.TraceLevel)
			core.Log.Debug("Logging at trace level.")
		} else if verbose {
			logrus.SetLevel(logrus.DebugLevel)
			core.Log.Debug("Logging at debug level.")
		} else {
			logrus.SetLevel(logrus.InfoLevel)
		}

		if step >= 0 {
			core.Log = core.Log.WithField("@step", step).WithField("@command", cmd.Name())
			cmd.SilenceUsage = true
		}

		if viper.GetBool(ArgGlobalProfile) {
			profilePath := "./bosun.prof"
			f, err := os.Create(profilePath)
			if err != nil {
				return errors.Wrap(err, "creating profiling file")
			}
			err = pprof.StartCPUProfile(f)
			if err != nil {
				return errors.Wrap(err, "starting profiling")
			}
		}

		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if viper.GetBool(ArgGlobalProfile) {
			pprof.StopCPUProfile()
		}
	},
})

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		switch e := err.(type) {
		case handledError:
			fmt.Println(e.Error())
		default:
			if viper.GetBool(ArgGlobalVerbose) || viper.GetBool(ArgGlobalVerboseErrors) {
				_, _ = colorError.Fprintf(os.Stderr, "%+v\n", err)

			} else {
				_, _ = colorError.Fprintln(os.Stderr, err)
			}
		}

		os.Exit(1)
	}
}

const (
	ArgGlobalSudo          = "sudo"
	ArgGlobalVerbose       = "verbose"
	ArgGlobalTrace         = "trace"
	ArgGlobalVerboseErrors = "verbose-errors"
	ArgGlobalDryRun        = "dry-run"
	ArgGlobalDomain        = "domain"
	ArgGlobalValues        = "values"
	ArgBosunConfigFile     = "config-file"
	ArgGlobalConfirmedEnv  = "confirm-env"
	ArgGlobalNoEnv         = "no-env"
	ArgGlobalNoCluster     = "no-cluster"
	ArgGlobalForce         = "force"
	ArgGlobalNoReport      = "no-report"
	ArgGlobalOutput        = "output"
	ArgGlobalProfile       = "profile"
	ArgGlobalCluster       = "cluster"
)

func init() {
	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", configPath, "The config file for bosun.")
	rootCmd.PersistentFlags().IntVar(&step, "step", -1, "The step we are on.")
	_ = rootCmd.PersistentFlags().MarkHidden("step")

	bosunConfigFile := os.Getenv("BOSUN_CONFIG")
	if bosunConfigFile == "" {
		bosunConfigFile = os.ExpandEnv("$HOME/.bosun/bosun.yaml")
	}

	rootCmd.PersistentFlags().String(ArgBosunConfigFile, bosunConfigFile, "Config file for Bosun. You can also set BOSUN_CONFIG.")
	rootCmd.PersistentFlags().StringP(ArgGlobalOutput, "o", "", "Output format. Options are `table`, `json`, or `yaml`. Only respected by a some commands.")
	rootCmd.PersistentFlags().Bool(ArgGlobalVerbose, false, "Enable verbose logging.")
	rootCmd.PersistentFlags().Bool(ArgGlobalTrace, false, "Enable trace logging.")
	_ = rootCmd.PersistentFlags().MarkHidden(ArgGlobalTrace)
	rootCmd.PersistentFlags().BoolP(ArgGlobalVerboseErrors, "V", false, "Enable verbose errors with stack traces.")
	rootCmd.PersistentFlags().Bool(ArgGlobalDryRun, false, "Display rendered plans, but do not actually execute (not supported by all commands).")
	rootCmd.PersistentFlags().Bool(ArgGlobalForce, false, "Force the requested command to be executed even if heuristics indicate it should not be.")
	rootCmd.PersistentFlags().Bool(ArgGlobalNoReport, false, "Disable reporting of deploys to github.")
	rootCmd.PersistentFlags().Bool(ArgGlobalNoEnv, false, "Disables loading of the environment.")
	rootCmd.PersistentFlags().Bool(ArgGlobalSudo, false, "Use sudo when running commands like docker.")
	rootCmd.PersistentFlags().String(ArgGlobalConfirmedEnv, "", "Set to confirm that the environment is correct when targeting a protected environment.")
	rootCmd.PersistentFlags().String(ArgGlobalCluster, "", "Set to target a specific cluster.")
	rootCmd.PersistentFlags().Bool(ArgGlobalProfile, false, "Dump profiling info.")
	_ = rootCmd.PersistentFlags().MarkHidden(ArgGlobalProfile)

	defaultDomain := ""
	vaultAddr, ok := os.LookupEnv("VAULT_ADDR")
	if ok {
		segs := strings.Split(vaultAddr, ".")
		tld := segs[len(segs)-1]
		defaultDomain = "n5o." + tld
	}

	rootCmd.PersistentFlags().String(ArgGlobalDomain, defaultDomain, "The domain to use when connecting, and as the .Domain value in templates.")
	rootCmd.PersistentFlags().MarkHidden(ArgGlobalDomain)
	rootCmd.PersistentFlags().StringSlice(ArgGlobalValues, []string{}, "Any number of key=value values which will be available under the .Values token in templates.")
	rootCmd.PersistentFlags().MarkHidden(ArgGlobalValues)

	viper.RegisterAlias("debug", "verbose")
	viper.BindPFlags(rootCmd.PersistentFlags())

	viper.AutomaticEnv()

	viper.BindEnv(ArgBosunConfigFile, "BOSUN_CONFIG")

	cobra.OnInitialize(initConfig)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	// viper.SetConfigFile(cfgFile)
	//
	// viper.AutomaticEnv() // read in environment variables that match
	//
	// // If a config file is found, read it in.
	// if err := viper.ReadInConfig(); err != nil {
	// 	log.Fatalf("config file not found (config path was %q)", cfgFile)
	// }
}
