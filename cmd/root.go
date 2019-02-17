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
	"github.com/naveego/bosun/pkg"
	"github.com/sirupsen/logrus"
	"strings"

	"os"

	"github.com/spf13/viper"

	"github.com/spf13/cobra"
)

var cfgFile string

var step int

var version string
var timestamp string
var commit string


// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "bosun",
	Short: "Devops tool.",
	SilenceErrors:true,
	Version: fmt.Sprintf(`Version: %s
Timestamp: %s
Commit: %s
`, version, timestamp, commit),
	Long: `This is our tool for for devops. If you have some scripts for
building, deploying, or monitoring apps you may want to add them to this tool.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {

		viper.RegisterAlias("debug", "verbose")
		viper.BindPFlags(cmd.Flags())
		viper.BindPFlags(cmd.PersistentFlags())

		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
			ForceColors:   true,
		})

		pkg.Log = logrus.NewEntry(logrus.StandardLogger())

		verbose := viper.GetBool("verbose")
		if verbose {
			logrus.SetLevel(logrus.DebugLevel)
			pkg.Log.Debug("Logging at debug level.")
		} else {
			logrus.SetLevel(logrus.InfoLevel)
		}

		if step >= 0 {
			pkg.Log = pkg.Log.WithField("@step", step).WithField("@command", cmd.Name())
			cmd.SilenceUsage = true
		}

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		switch e := err.(type){
		case handledError:
			fmt.Println(e.Error())
		default:
			colorError.Fprintln(os.Stderr, err)
		}

		os.Exit(1)
	}
}

const (
	ArgGlobalVerbose   = "verbose"
	ArgGlobalDryRun    = "dry-run"
	ArgGlobalCluster   ="cluster"
	ArgGlobalDomain    ="domain"
	ArgGlobalValues    = "values"
	ArgBosunConfigFile = "config-file"
	ArgGlobalForce        = "force"
	ArgGlobalNoReport        = "no-report"
	ArgGlobalOutput      = "output"
)



func init() {
	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", configPath, "The config file for bosun.")
	rootCmd.PersistentFlags().IntVar(&step, "step", -1, "The step we are on.")
	rootCmd.PersistentFlags().MarkHidden("step")

	rootCmd.PersistentFlags().String(ArgBosunConfigFile, "$HOME/.bosun/bosun.yaml", "Config file for Bosun. You can also set BOSUN_CONFIG.")
	rootCmd.PersistentFlags().StringP(ArgGlobalOutput, "o","table", "Output format. Options are `table` or `yaml`. Only respected by a some commands.")
	rootCmd.PersistentFlags().Bool(ArgGlobalVerbose, false, "Enable verbose logging.")
	rootCmd.PersistentFlags().Bool(ArgGlobalDryRun, false, "Display rendered plans, but do not actually execute (not supported by all commands).")
	rootCmd.PersistentFlags().Bool(	ArgGlobalForce, false, "Force the requested command to be executed even if heuristics indicate it should not be.")
	rootCmd.PersistentFlags().Bool(	ArgGlobalNoReport, false, "Disable reporting of deploys to github.")

	defaultCluster := ""
	defaultDomain := ""
	vaultAddr, ok := os.LookupEnv("VAULT_ADDR")
	if ok {
		segs := strings.Split(vaultAddr, ".")
		tld := segs[len(segs)-1]
		defaultCluster = tld
		defaultDomain = "n5o." + tld
	}

	rootCmd.PersistentFlags().String(ArgGlobalCluster, defaultCluster, "The cluster to use when getting kube config data, and as the .Cluster value in templates.")
	rootCmd.PersistentFlags().MarkHidden(ArgGlobalCluster)
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
