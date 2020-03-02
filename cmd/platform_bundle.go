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
	"github.com/spf13/viper"
	"path/filepath"
)

func init() {

}

var bundleCmd = addCommand(platformCmd, &cobra.Command{
	Use: "bundle",
	Short:"Bundling commands",
})

var _ = addCommand(bundleCmd, &cobra.Command{
	Use:          "create {environment [, environment...]}",
	Args:         cobra.MinimumNArgs(1),
	Short:        "Creates a portable bundle zip from a platform and the active release.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil  {
			return err
		}

		release, err := p.GetCurrentRelease()
		if err != nil {
			return err
		}

		bundler := bosun.NewPlatformBundler(b, p)

		result, err := bundler.Execute(bosun.BundlePlatformRequest{
			Prefix:viper.GetString(argBundleCreateName),
			Environments:args,
			Releases: []string{release.Slot},
		})
		if err != nil {
			return err
		}

		fmt.Println(result.OutPath)

		return nil
	},
}, func(cmd *cobra.Command) {
cmd.Flags().String(argBundleCreateName, "", "Name of the bundle")
})

const (
	argBundleCreateName = "name"
)

func loadDeploymentPlan(b *bosun.Bosun, p *bosun.Platform, pathOrName string) (*bosun.DeploymentPlan, error) {
	if pathOrName == "release" {
		release, err := p.GetCurrentRelease()
		if err != nil {
			return nil, err
		}
		pathOrName = filepath.Join(p.GetDeploymentsDir(), fmt.Sprintf("%s/plan.yaml", release.Version.String()))
	}

	plan, err := bosun.LoadDeploymentPlanFromFile(pathOrName)
	return plan, err
}