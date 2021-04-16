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
	"github.com/google/uuid"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/templating"
	"github.com/naveego/bosun/pkg/vault"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"os/user"
	"strings"
	"time"
)

// vaultCmd represents the vault command
var vaultCmd = &cobra.Command{
	Use:   "vault",
	Args:  cobra.MinimumNArgs(1),
	Short: "Contains vault-related commands.",
}

var vaultApplyCmd = addCommand(vaultCmd, &cobra.Command{
	Use:   "apply {app} {file}",
	Args:  cobra.ExactArgs(2),
	Short: "Applies a vault layout for an app, without doing the full deploy.",
	Long: `This command has environmental pre-reqs:
- You must be authenticated to vault (with VAULT_ADDR set and either VAULT_TOKEN set or a ~/.vault-token file created by logging in to vault).

The {vault-layouts...} argument is one or more paths to a vault layout yaml, or a glob which will locate a set of files.

The vault layout yaml file can use go template syntax for formatting.

The .Domain and .ClusterBrn values are populated from the flags to this command, or inferred from VAULT_ADDR.
Any values provided using --values will be in {{ .Values.xxx }}
`,
	Example: "vault green-auth.yaml green-kube.yaml green-default.yaml",
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		var err error

		b := MustGetBosun()

		ctx := b.NewContext()

		g := globalParameters{}
		err = g.init()
		if err != nil {
			return err
		}
		vaultClient, err := vault.NewVaultLowlevelClient(g.vaultToken, g.vaultAddr)
		if err != nil {
			return err
		}

		app := mustGetApp(b, args[0:1])

		appDeploy, err := getAppDeploy(b, app)
		if err != nil {
			return err
		}
		templateValues, err := appDeploy.GetResolvedValues(ctx)
		if err != nil {
			return err
		}

		if viper.GetBool(argVaultApplyDumpValues) {

			fmt.Println("# Attribution")
			fmt.Println("---")
			y, _ := yaml.Marshal(templateValues.Attribution)
			fmt.Println(string(y))

			fmt.Println()
			fmt.Println("# Values")
			fmt.Println("---")
			y, _ = yaml.Marshal(templateValues.Values)
			fmt.Println(string(y))

			return nil
		}

		templateArgs := templating.TemplateValues{
			Values: templateValues.Values,
		}

		for _, kv := range viper.GetStringSlice(ArgGlobalValues) {
			segs := strings.Split(kv, "=")
			if len(segs) != 2 {
				return errors.Errorf("invalid values flag value: %q (should be key=value)", kv)
			}
			templateArgs.Values[segs[0]] = segs[1]
		}

		vaultLayout, err := vault.LoadVaultLayoutFromFiles(args, templateArgs, vaultClient)
		if err != nil {
			return err
		}

		if viper.GetBool(ArgGlobalDryRun) {
			out, err2 := yaml.Marshal(vaultLayout)
			if err2 != nil {
				return err2
			}
			color.Yellow("Dry run. This is the rendered template that would be applied:")
			fmt.Println(string(out))
			return nil
		}

		key := strings.Join(args, "-")
		force := viper.GetBool(ArgGlobalForce)
		err = vaultLayout.Apply(key, force, vaultClient)

		if err != nil {
			ctx.Log().Warnf("Error applying layout. Vault URL was %s, token was %s...", vaultClient.Address(), vaultClient.Token()[0:3])
		}

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(argVaultApplyDumpValues, false, "Dump values instead of actually applying")

})

const argVaultApplyDumpValues = "dump-values"

var vaultInitCmd = &cobra.Command{
	Use:          "init [namespace]",
	Aliases:      []string{"bootstrap-dev"},
	Short:        "Initializes or unseals a vault instance.",
	SilenceUsage: true,
	Long: `
If Vault has not been initialized, this will initialize it and store the keys in Kubernetes secrets.
If Vault is initialized, but sealed, this will unseal it using the keys stored in Kubernetes.
Otherwise, this will do nothing.
`,
	Example: "vault init",
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		var err error

		g := globalParameters{}
		err = g.init()
		if err != nil {
			return err
		}

		core.Log.Infof("Bootstrapping vault using address %s and token %s", g.vaultAddr, g.vaultToken)

		vaultClient, err := vault.NewVaultLowlevelClient(g.vaultToken, g.vaultAddr)
		if err != nil {
			return err
		}

		b := MustGetBosun()

		env := b.GetCurrentEnvironment()

		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		platformApps := p.GetKnownAppMap()

		namespace := "default"

		if len(args) > 0 {
			namespace = args[1]
		} else {

			vaultPlatformApp, ok := platformApps["vault"]

			if !ok {
				return errors.New("vault not found in platform apps")
			}

			stack := env.Stack()

			for _, requestedNamespaceRole := range vaultPlatformApp.NamespaceRoles {
				for namespaceRole, namespaceConfig := range stack.StackTemplate.Namespaces {
					if namespaceRole == requestedNamespaceRole {
						namespace = namespaceConfig.Name
						break
					}
				}
			}
		}

		core.Log.Infof("Initializing vault in namespace %q", namespace)

		initializer := vault.VaultInitializer{
			Client:         vaultClient,
			VaultNamespace: namespace,
		}

		err = initializer.Init()
		if err != nil {
			return err
		}

		return nil
	},
}

var vaultUnsealCmd = &cobra.Command{
	Use:           "unseal [namespace]",
	Short:         "Unseals vault using the keys in k8s.",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {


		namespace := "default"
		if len(args) > 0 {
			namespace = args[0]
		}

		viper.BindPFlags(cmd.Flags())

		g := globalParameters{}
		err := g.init()
		if err != nil {
			return err
		}

		vaultClient, err := vault.NewVaultLowlevelClient(g.vaultToken, g.vaultAddr)
		if err != nil {
			return err
		}

		core.Log.Infof("Unsealing vault in namespace %q", namespace)


		initializer := vault.VaultInitializer{
			Client:         vaultClient,
			VaultNamespace: namespace,
		}

		err = initializer.Unseal()

		return err
	},
}

var vaultSecretCmd = &cobra.Command{
	Use:           "secret {path} [key]",
	Args:          cobra.RangeArgs(1, 2),
	Short:         "Gets a secret value from vault, optionally populating the value if not found.",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		_ = MustGetBosun()

		vaultClient, err := vault.NewVaultLowlevelClient("", "")
		if err != nil {
			return err
		}

		path := args[0]
		key := "key"
		if len(args) > 1 {
			key = args[1]
		}

		defaultValue := viper.GetString(ArgVaultSecretDefault)
		if viper.GetBool(ArgVaultSecretGenerate) {
			defaultValue = strings.Replace(uuid.New().String(), "-", "", -1)
		}

		action := vault.GetOrUpdateVaultSecretAction{
			Client:       vaultClient,
			Path:         path,
			Key:          key,
			Replace:      viper.GetBool(ArgVaultSecretOverwrite),
			DefaultValue: defaultValue,
		}

		p, err := action.Execute()

		if err != nil {
			return err
		}

		fmt.Println(p)

		return err
	},
}

var vaultJWTCmd = &cobra.Command{
	Use:     "jwt",
	Short:   "Creates a JWT.",
	Long:    ``,
	Example: "vault init-dev",
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		viper.BindEnv(ArgVaultAddr, "VAULT_ADDR")
		viper.BindEnv(ArgVaultToken, "VAULT_TOKEN")

		g := globalParameters{}
		err := g.init()
		if err != nil {
			return err
		}

		vaultClient, err := vault.NewVaultLowlevelClient(g.vaultToken, g.vaultAddr)
		if err != nil {
			return err
		}

		role := viper.GetString(ArgVaultJWTRole)
		tenant := viper.GetString(ArgVaultJWTTenant)
		sub := viper.GetString(ArgVaultJWTSub)
		claimsStrings := viper.GetStringSlice(ArgVaultJWTClaims)
		claims := map[string]interface{}{
			"tid": tenant,
			"sub": sub,
		}
		for _, c := range claimsStrings {
			segs := strings.Split(c, "=")
			if len(segs) == 2 {
				claims[segs[0]] = segs[1]
			} else {
				return errors.Errorf("invalid claim %q (wanted k=v format)", c)
			}
		}
		ttl := viper.GetDuration(ArgVaultJWTTTL)
		exp := time.Now().Add(ttl).Unix()
		claims["exp"] = exp

		req := map[string]interface{}{
			"claims":    claims,
			"token_ttl": ttl.Seconds(),
		}

		s, err := vaultClient.Logical().Write(fmt.Sprintf("jose/jwt/issue/%s", role), req)
		if err != nil {
			return err
		}

		fmt.Println(s.Data["token"])

		return err
	},
}

const (
	ArgVaultAddr            = "vault-addr"
	ArgVaultToken           = "vault-token"
	ArgVaultJWTRole         = "role"
	ArgVaultJWTTenant       = "tenant"
	ArgVaultJWTSub          = "sub"
	ArgVaultJWTTTL          = "ttl"
	ArgVaultJWTClaims       = "claims"
	ArgVaultSecretGenerate  = "generate"
	ArgVaultSecretOverwrite = "overwrite"
	ArgVaultSecretDefault   = "default"
	ArgVaultNamespace       = "vault-namespace"
	ArgVaultCluster         = "cluster"
)

func init() {

	sub := "admin"
	u, err := user.Current()
	if err == nil {
		sub = u.Username
	}

	vaultJWTCmd.Flags().StringP(ArgVaultJWTRole, "r", "auth", "The role to use when creating the token.")
	vaultJWTCmd.Flags().StringP(ArgVaultJWTTenant, "t", "", "The tenant to set.")
	vaultJWTCmd.MarkFlagRequired(ArgVaultJWTTenant)
	vaultJWTCmd.Flags().StringP(ArgVaultJWTSub, "s", sub, "The sub to set.")
	vaultJWTCmd.Flags().Duration(ArgVaultJWTTTL, 15*time.Minute, "The TTL for the JWT, in go duration format.")
	vaultJWTCmd.Flags().StringSlice(ArgVaultJWTClaims, []string{}, "Additional claims to set, as k=v pairs. Use the flag multiple times or delimit claims with commas.")
	addVaultFlags(vaultJWTCmd)
	vaultCmd.AddCommand(vaultJWTCmd)

	addVaultFlags(vaultInitCmd)
	vaultCmd.AddCommand(vaultInitCmd)

	addVaultFlags(vaultUnsealCmd)
	vaultCmd.AddCommand(vaultUnsealCmd)

	addVaultFlags(vaultSecretCmd)
	vaultSecretCmd.Flags().Bool(ArgVaultSecretGenerate, false, "Generate the secret if it's not found.")
	vaultSecretCmd.Flags().Bool(ArgVaultSecretOverwrite, false, "Overwrite existing secret.")
	vaultSecretCmd.Flags().String(ArgVaultSecretDefault, "", "Set the secret to this value if not found or --overwrite is set.")
	vaultCmd.AddCommand(vaultSecretCmd)

	addVaultFlags(vaultCmd)
	rootCmd.AddCommand(vaultCmd)
}

func addVaultFlags(cmd *cobra.Command) {
	cmd.Flags().String(ArgVaultAddr, "", "URL to Vault. Or set VAULT_ADDR.")
	cmd.Flags().String(ArgVaultToken, "", "Vault token. Or set VAULT_TOKEN.")
	cmd.Flags().String(ArgVaultNamespace, "kube-system", "Namespace of the vault pod (default: kube-system)")
}
