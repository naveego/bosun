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
	"encoding/base64"
	"fmt"
	"github.com/fatih/color"
	"github.com/hashicorp/vault/api"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"strings"
)

// vaultCmd represents the vault command
var vaultCmd = &cobra.Command{
	Use:   "vault {vault-layouts...}",
	Args: cobra.MinimumNArgs(1),
	Short: "Updates VaultClient using layout files. Supports --dry-run flag.",
	Long: `This command has environmental pre-reqs:
- You must be authenticated to vault (with VAULT_ADDR set and either VAULT_TOKEN set or a ~/.vault-token file created by logging in to vault).

The {vault-layouts...} argument is one or more paths to a vault layout yaml, or a glob which will locate a set of files.

The vault layout yaml file can use go template syntax for formatting.

The .Domain and .Cluster values are populated from the flags to this command, or inferred from VAULT_ADDR.
Any values provided using --values will be in {{ .Values.xxx }}
`,
	Example:"vault green-auth.yaml green-kube.yaml green-default.yaml",
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		vaultClient, err := pkg.NewVaultLowlevelClient("", "")
		if err != nil {
			return err
		}

		templateArgs := pkg.TemplateValues{
			Cluster:viper.GetString(ArgGlobalCluster),
			Domain:viper.GetString(ArgGlobalDomain),
			Values:map[string]interface{}{
				"cluster":viper.GetString(ArgGlobalCluster),
				"domain":viper.GetString(ArgGlobalDomain),
			},
		}

		for _, kv := range viper.GetStringSlice(ArgGlobalValues){
			segs := strings.Split(kv, "=")
			if len(segs) != 2 {
				return errors.Errorf("invalid values flag value: %q (should be key=value)", kv)
			}
			templateArgs.Values[segs[0]] = segs[1]
		}

		vaultLayout, err := pkg.LoadVaultLayoutFromFiles(args, templateArgs, vaultClient)
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

var vaultInitCmd = &cobra.Command{
	Use:   "bootstrap-dev",
	Short: "Sets up a Vault instance suitable for non-production environment.",

	Long: `This command should only be run against the dev vault instances.

If Vault has not been initialized, this will initialize it and store the keys in Kubernetes secrets.
If Vault is initialized, but sealed, this will unseal it using the keys stored in Kubernetes.
Otherwise, this will do nothing.
`,
	Example:"vault init-dev",
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		vaultClient, err := pkg.NewVaultLowlevelClient("", "")
		if err != nil {
			return err
		}

		initialized, err := vaultClient.Sys().InitStatus()
		if err != nil {
			return err
		}

		if !initialized {
			_, _, err = initialize(vaultClient)
			if err != nil {
				return err
			}
		} else {
			fmt.Printf("Vault at %q is already initialized.\n", vaultClient.Address())
		}

		sealStatus, err := vaultClient.Sys().SealStatus()
		if err != nil {
			return err
		}
		if sealStatus.Sealed {
			err = unseal(vaultClient)
			if err != nil {
				return err
			}
		} else {
			fmt.Printf("Vault at %q is already unsealed.\n", vaultClient.Address())
		}

		err = installPlugin(vaultClient)



		return err
	},
}

func installPlugin(vaultClient *api.Client)error {
	joseSHA, err := pkg.NewCommand("kubectl exec vault-dev-0 cat /vault/plugins/jose-plugin.sha").RunOut()
	if err != nil {
		return err
	}

	err = vaultClient.Sys().RegisterPlugin(&api.RegisterPluginInput{
		Name:"jose",
		SHA256:joseSHA,
		Command:"jose-plugin",
	})

	if err != nil {
		return err
	}

	fmt.Println("JOSE plugin installed.")
	return nil
}

func unseal(vaultClient *api.Client) error {
	secretYaml, err := pkg.NewCommand("kubectl get secret vault-unseal-keys -o yaml").RunOut()
	if err != nil {
		return err
	}


	var secret map[string]interface{}
	err = yaml.Unmarshal([]byte(secretYaml), &secret)
	if err != nil {
		return err
	}

	data := secret["data"].(map[interface{}]interface{})
	for k, v := range data {
		fmt.Printf("Unsealing with key %v\n", k)

		shard, _ := base64.StdEncoding.DecodeString(v.(string))
		_, err = vaultClient.Sys().Unseal(string(shard))
		if err != nil {
			return err
		}
	}

	return nil
}

func initialize(vaultClient *api.Client) (keys []string, rootToken string, err error) {
	err = pkg.NewCommand("kubectl delete secret vault-secret --ignore-not-found=true").RunE()
	if err != nil {
		return nil, "", err
	}

	initResp, err := vaultClient.Sys().Init(&api.InitRequest{
		SecretShares:1,
		SecretThreshold:1,
	})
	if err != nil {
		return nil, "", err
	}

	err = pkg.NewCommand("kubectl", "create", "secret", "generic", "vault-root-token", fmt.Sprintf("--from-literal=root=%s", initResp.RootToken)).RunE()
	if err != nil {
		return nil, "", err
	}

	for i, key := range initResp.Keys {
		fmt.Printf("Seal key %d: %q", i, key)
		err = pkg.NewCommand("kubectl", "create", "secret", "generic", "vault-unseal-keys", fmt.Sprintf("--from-literal=key%d=%s", i, key)).RunE()
		if err != nil {
			return nil, "", err
		}
	}

	root := initResp.RootToken

	vaultClient.SetToken(root)

	_, err = vaultClient.Auth().Token().Create(&api.TokenCreateRequest{
		ID:"root",
		Policies:[]string{"root"},
	})

	return initResp.Keys, initResp.RootToken, nil

}



func init() {


	vaultCmd.AddCommand(vaultInitCmd)

	rootCmd.AddCommand(vaultCmd)
}
