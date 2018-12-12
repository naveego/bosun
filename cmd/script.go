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
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type Script struct {
	Cluster string
	Domain string
	Steps []ScriptStep
}

type ScriptStep struct {
	Command string
	Args    []string
	Flags   map[string]interface{}
}

var scriptCmd = &cobra.Command{
	Use:   "script {script-file}",
	Args:  cobra.ExactArgs(1),
	Short: "Run a scripted sequence of commands.",
	Long:  `Provide a script file path.`,
	SilenceUsage:true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		scriptFilePath := args[0]
		var script Script
		b, err := ioutil.ReadFile(scriptFilePath)
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(b, &script)
		if err != nil {
			return err
		}

		g := globalParameters{
			domain:script.Domain,
			cluster:script.Cluster,
		}

		err = g.init()
		if err != nil {
			return errors.Wrap(err, "value can be defined in script file or as a parameter")
		}

		script.Cluster = g.cluster
		script.Domain = g.domain

		rootDir := filepath.Dir(scriptFilePath)

		exe, err := os.Executable()
		if err != nil {
			return err
		}
		if !strings.Contains(exe, "bosun") {
			exe = "bosun" // support working under debugger
		}

		exe, err = exec.LookPath(exe)
		if err != nil {
			return err
		}

		if len(scriptStepsSlice) == 0 {
			for i := range script.Steps {
				scriptStepsSlice = append(scriptStepsSlice, i)
			}
		}

		for _, i := range scriptStepsSlice{
			if i >= len(script.Steps){
				return errors.Errorf("invalid step %d (there are %d steps)", i, len(script.Steps))
			}
			step := script.Steps[i]
			log := pkg.Log.WithField("step", i).WithField("command", step.Command)
			if step.Flags == nil {
				step.Flags = make(map[string]interface{})
			}

			var stepArgs []string
			stepArgs = append(stepArgs, strings.Fields(step.Command)...)
			stepArgs = append(stepArgs, "--step", fmt.Sprintf("%d", i))
			if viper.GetBool(ArgGlobalVerbose) {
				stepArgs = append(stepArgs, "--" + ArgGlobalVerbose)
			}
			if viper.GetBool(ArgGlobalDryRun) {
				stepArgs = append(stepArgs, "--" + ArgGlobalDryRun)
			}

			step.Flags[ArgGlobalDomain] = script.Domain
			step.Flags[ArgGlobalCluster] = script.Cluster

			for k, v := range step.Flags {
				switch vt := v.(type) {
				case []interface{}:
					var arr  []string
					for _, i := range vt {
						arr = append(arr, fmt.Sprint(i))
					}
					stepArgs = append(stepArgs, fmt.Sprintf("--%s", k), strings.Join(arr, ","))
				case bool:
					stepArgs = append(stepArgs, fmt.Sprintf("--%s", k))
				default:
					stepArgs = append(stepArgs, fmt.Sprintf("--%s", k), fmt.Sprintf("%v", vt))
				}
			}

			for _, v := range step.Args {
				stepArgs = append(stepArgs, v)
			}

			log.WithField("args", stepArgs).Info("Executing step")

			err = pkg.NewCommand(exe, stepArgs...).WithDir(rootDir).RunE()
			if err != nil {
				log.WithField("flags", step.Flags).WithField("args", step.Args).Error("Step failed.")
				return errors.New("script abended")
			}
		}

		return nil
	},
}

const (
	ArgScriptSteps = "steps"
)

var (
	scriptStepsSlice []int
)

func init() {

	scriptCmd.Flags().IntSliceVar(&scriptStepsSlice, ArgScriptSteps, []int{}, "Steps to run (defaults to all steps)")

	rootCmd.AddCommand(scriptCmd)
}
