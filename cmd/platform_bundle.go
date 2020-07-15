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
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/environment"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
)

func init() {

}

var bundleCmd = addCommand(platformCmd, &cobra.Command{
	Use:   "bundle",
	Short: "Bundling commands",
})

var _ = addCommand(bundleCmd, &cobra.Command{
	Use:          "create",
	Short:        "Creates a portable bundle zip from a platform and the active release.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		result, err := bosun.NewPlatformBundler(b, p).Execute()
		if err != nil {
			return err
		}

		fmt.Println(result.OutPath)

		return nil
	},
}, func(cmd *cobra.Command) {

})


var _ = addCommand(bundleCmd, &cobra.Command{
	Use:          "push",
	Short:        "Pushes a bundle to a kubernetes cluster.",
	Long:         "By default, pushes to the current cluster selected in the kubeconfig",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		bundleDir := viper.GetString(argBundleDir)
		bundleEnvPath := viper.GetString(argBundleEnv)
		bundlePushAllApps := viper.GetBool(argBundlePushAll)

		envConfig, err := environment.LoadConfig(bundleEnvPath)
		if err != nil {
			return err
		}

		bundleImports := []string{ filepath.Join(bundleDir, "platform.yaml") }

		if os.Getenv("BOSUN_ENVIRONMENT") == "" {
			os.Setenv("BOSUN_ENVIRONMENT", envConfig.Name)
		}

		if os.Getenv("BOSUN_BUNDLE_ENV") == "" {
			os.Setenv("BOSUN_BUNDLE_ENV", bundleEnvPath)
		}

		workspace, err := bosun.LoadWorkspaceWithStaticImports(bundleEnvPath, bundleImports)
		if err != nil {
			return err
		}

		workspace.CurrentEnvironment = envConfig.Name

		var params cli.Parameters
		b, err := bosun.New(params, workspace)
		if err != nil {
			return err
		}

		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		pushApp := ""
		if len(args) > 0 {
			pushApp = args[0]
		}

		pusher := bosun.NewPlatformPusher(b, p)

		err = pusher.Push(bosun.PlatformPushRequest{
			BundleDir: bundleDir,
			ManifestDir: filepath.Join(bundleDir, "releases", "current"),
			EnvironmentPath: bundleEnvPath,
			PushApp: pushApp,
			PushAllApps: bundlePushAllApps,
			Cluster: envConfig.Clusters[0].Name,
		})

		if err != nil {
			logrus.Errorf("Could not push deployment: %v", err)
		}
		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().String(argBundlePushNamespace, "default", "The namespace to place the bundle in.")
	cmd.Flags().String(argBundleDir, os.Getenv("BOSUN_BUNDLE_DIR"), "The directory of the unzipped bundle")
	cmd.Flags().String(argBundleEnv, os.Getenv("BOSUN_BUNDLE_ENV"), "The path to the environment file")
	cmd.Flags().Bool(argBundlePushAll, os.Getenv("BOSUN_BUNDLE_PUSH_ALL") != "", "Whether or not to push all apps or only upgraded apps")

})

const (
	argBundlePushNamespace = "namespace"
	argBundleDir = "bundle-dir"
	argBundleEnv = "bundle-env"
	argBundlePushAll = "bundle-push-all"

	LabelBundleConfigMapHash = "naveego.com/bosun-bundle-hash"
)



