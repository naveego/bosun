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
	"github.com/hashicorp/vault/api"
	"github.com/naveego/bosun/pkg"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
	"strings"
)

// helmsmanCmd represents the helmsman command
var helmsmanCmd = &cobra.Command{
	Use:   "helmsman {cluster} {helmsman-file} [additional-helmsman-files...}",
	Args:  cobra.MinimumNArgs(2),
	Short: "Deploys a helmsman to a cluster. Supports --dry-run flag.",
	Long: `This command has environmental pre-reqs:
- You must be authenticated to vault (with VAULT_ADDR set and either VAULT_TOKEN set or a ~/.vault-token file created by logging in to vault).
- You must have kubectl installed and have a context defined for the cluster you want to deploy to.
- You must have helm installed.
- You must have helmsman installed. (https://pkg.com/Praqma/helmsman)

The {domain} argument is the domain the services in the helmsman will be available under (e.g. n5o.green or n5.blue).
The {helmsman-files} argument is the path of the helmsman files to pass to helmsman.

You must set the --apply flag to actually run the helmsman (this is to prevent accidents).
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		var err error
		noVault := viper.GetBool(ArgHelmsmanNoVault)


		checkExecutableDependency("helm")

		var vaultClient *api.Client
		if !noVault {
			vaultClient, err = pkg.NewVaultLowlevelClient("", "")
			if err != nil {
				return err
			}
		}

		helmsmanFile, err := filepath.Abs(args[1])
		if err != nil {
			return err
		}

		_, err = os.Stat(helmsmanFile)
		if err != nil {
			return err
		}

		r := pkg.HelmsmanCommand{
			VaultClient:      vaultClient,
			Cluster:          args[0],
			HelmsmanFilePaths: args[1:],
			Apply:            viper.GetBool(ArgHelmsmanApply),
			NoConfirm:        viper.GetBool(ArgHelmsmanNoConfirm),
			DryRun:           viper.GetBool(ArgGlobalDryRun),
			NoVault:          viper.GetBool(ArgHelmsmanNoVault),
			KeepRenderedFile: viper.GetBool(ArgHelmsmanKeepTempFiles),
			Apps:             viper.GetStringSlice(ArgHelmsmanApps),
			Values:           map[string]interface{}{},
			Verbose:          viper.GetBool(ArgGlobalVerbose),
		}

		r.Domain = viper.GetString(ArgHelmsmanDomain)
		if r.Domain == "" {
			r.Domain = fmt.Sprintf("n5o.%s", r.Cluster)
		}

		r.MarketingRelease, err = getMarketingRelease()
		if err != nil {
			return err
		}


		values := viper.GetStringSlice(ArgHelmsmanSet)
		for _, v := range values {
			segs := strings.Split(v, "=")
			if len(segs) == 2 {
				r.Values[segs[0]] = segs[1]
			}
		}

		err = r.Execute()

		return err
	},
}

const (
	ArgHelmsmanDomain           = "domain"
	ArgHelmsmanMarketingRelease = "marketing-release"
	ArgHelmsmanApply            = "apply"
	ArgHelmsmanSet              = "set"
	ArgHelmsmanNoConfirm        = "no-confirm"
	ArgHelmsmanNoVault          = "no-vault"
	ArgHelmsmanKeepTempFiles    = "keep-temp-files"
	ArgHelmsmanApps             = "apps"
)

func init() {
	rootCmd.AddCommand(helmsmanCmd)
	helmsmanCmd.Flags().String(ArgHelmsmanDomain, "", "The value of {{ .Domain }} in the template. If not set, will default to '.n5o.{{.Cluster}}'.")
	helmsmanCmd.Flags().String(ArgHelmsmanMarketingRelease, "", "The value of {{ .MarketingRelease }} in the template. If not set, will default to the current branch.")
	helmsmanCmd.Flags().Bool(ArgHelmsmanApply, false, "Actually run the deploy (if not set, will render and print the template, then exit).")
	helmsmanCmd.Flags().Bool(ArgHelmsmanNoConfirm, false, "Suppress all confirmation messages.")
	helmsmanCmd.Flags().Bool(ArgHelmsmanKeepTempFiles, false, "Don't delete temp files when done.")
	helmsmanCmd.Flags().Bool(ArgHelmsmanNoVault, false, "Disable vault integration (secrets will be populated with the string 'disabled').")
	helmsmanCmd.Flags().StringSlice(ArgHelmsmanSet, []string{}, "Comma-delimited list of value=key pairs to set in the helmsman chart. Available as {{ .Values.value }} in template.")
	helmsmanCmd.Flags().StringSlice(ArgHelmsmanApps, []string{}, "Comma-delimited list of apps to include (if not set, all apps will be included).")
}
