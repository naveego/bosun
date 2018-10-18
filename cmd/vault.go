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
	"github.com/naveegoinc/devops/bosun/internal"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"os"
	"strings"
)

// vaultCmd represents the vault command
var vaultCmd = &cobra.Command{
	Use:   "vault {vault-layout}",
	Args: cobra.ExactArgs(1),
	Short: "Updates VaultClient using layout files. Supports --dry-run flag.",
	Long: `This command has environmental pre-reqs:
- You must be authenticated to vault (with VAULT_ADDR set and either VAULT_TOKEN set or a ~/.vault-token file created by logging in to vault).

The {vault-layout} argument is the path to the vault layout.

The vault layout yaml file can use go template syntax for formatting.

The .Domain and .Cluster values are populated from the flags to this command, or inferred from VAULT_ADDR.
Any values provided using --values will be in {{ .Values.xxx }}
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		vaultClient, err := internal.NewVaultLowlevelClient("", "")
		if err != nil {
			return err
		}

		templateArgs := internal.VaultLayoutTemplateArgs{
			Cluster:viper.GetString(ArgVaultCluster),
			Domain:viper.GetString(ArgVaultDomain),
			Values:map[string]interface{}{
				"cluster":viper.GetString(ArgVaultCluster),
				"domain":viper.GetString(ArgVaultDomain),
			},
		}

		for _, kv := range viper.GetStringSlice(ArgVaultValues){
			segs := strings.Split(kv, "=")
			if len(segs) != 2 {
				return errors.Errorf("invalid values flag value: %q (should be key=value)", kv)
			}
			templateArgs.Values[segs[0]] = segs[1]
		}

		vaultLayout, err := internal.LoadVaultLayoutFromFile(args[0], templateArgs, vaultClient)
		if err != nil {
			return err
		}

		if viper.GetBool(ArgGlobalDryRun) {
			out, err := yaml.Marshal(vaultLayout)
			if err != nil {
				return err
			}
			color.Yellow("Dry run. This is the rendered template that would be applied:")
			fmt.Println(string(out))
			return nil
		}

		err = vaultLayout.Apply(vaultClient)

		return err
	},
}

const (
	ArgVaultCluster ="cluster"
	ArgVaultDomain ="domain"
	ArgVaultValues = "values"
)

func init() {

	defaultCluster := ""
	defaultDomain := ""
	vaultAddr, ok := os.LookupEnv("VAULT_ADDR")
	if ok {
		segs := strings.Split(vaultAddr, ".")
		tld := segs[len(segs)-1]
		defaultCluster = tld
		defaultDomain = "n5o." + tld
	}

	vaultCmd.Flags().String(ArgVaultCluster, defaultCluster, "The cluster to use when getting kube config data.")
	vaultCmd.Flags().String(ArgVaultDomain, defaultDomain, "The domain to use when applying data to vault (will be defaulted from VAULT_ADDR if unset).")
	vaultCmd.Flags().StringSlice(ArgVaultValues, []string{}, "Any number of key=value values which will be available under the .Values token in the layout template.")

	rootCmd.AddCommand(vaultCmd)
}
