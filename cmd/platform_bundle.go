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
	"github.com/spf13/cobra"
	"os"
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
		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		bundleDir := os.Getenv("BOSUN_BUNDLE_DIR")
		bundleEnvPath := os.Getenv("BOSUN_BUNDLE_ENV")

		pusher := bosun.NewPlatformPusher(b, p)

		err = pusher.Push(bosun.PlatformPushRequest{
			BundleDir: bundleDir,
			EnvironmentPath: bundleEnvPath,
		})

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().String(argBundlePushNamespace, "default", "The namespace to place the bundle in.")
})

const (
	argBundlePushNamespace = "namespace"
	LabelBundleConfigMapHash = "naveego.com/bosun-bundle-hash"
)

