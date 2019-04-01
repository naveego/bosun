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
	"context"
	"fmt"
	"github.com/coreos/go-semver/semver"
	"github.com/google/go-github/v20/github"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/naveego/bosun/pkg"
 "github.com/hashicorp/go-getter"
	"github.com/spf13/cobra"
	"strings"
	"time"
)

var metaCmd = addCommand(rootCmd, &cobra.Command{
	Use:          "meta",
	Short:        "Commands for managing bosun itself.",

})

var metaUpgradeCmd = addCommand(metaCmd, &cobra.Command{
	Use:"upgrade",
	Short:"Upgrades bosun if a newer release is available",
	RunE: func(cmd *cobra.Command, args []string) error {

		client := mustGetGithubClient()
		ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
var err error
		if version == "" {
			version, err = pkg.NewCommand("bosun", "app", "version", "bosun").RunOut()
			if err != nil {
				return errors.Wrap(err, "could not get version")
			}
		}

		currentVersion, err := semver.NewVersion(version)

		releases, _, err := client.Repositories.ListReleases(ctx, "naveego", "bosun", nil)
		if err != nil {
			return err
		}
		var release *github.RepositoryRelease
		var upgradeAvailable bool
		for _, release = range releases {
			tag := release.GetTagName()
			tagVersion, err := semver.NewVersion(strings.TrimLeft(tag, "v"))
			if err != nil{
				continue
			}
			if currentVersion.LessThan(*tagVersion){
				upgradeAvailable = true
				break
			}
		}

		if !upgradeAvailable {
			fmt.Printf("Current version (%s) is up-to-date.\n", version)
			return nil
		}

		pkg.Log.Infof("Found upgrade: %s", release.GetTagName())


		expectedAssetName := fmt.Sprintf("bosun_%s_%s_%s.tar.gz", release.GetTagName(), runtime.GOOS, runtime.GOARCH)
		var foundAsset bool
		var asset *github.ReleaseAsset
		for _, asset := range release.Assets {
			name := asset.GetName()
			if name == expectedAssetName {
				foundAsset = true
				break
			}
		}
		if !foundAsset {
			return errors.Errorf("could not find an asset with name %q", expectedAssetName)
		}

		downloadURL := asset.GetBrowserDownloadURL()
		pkg.Log.Infof("Found upgrade asset, will download from %q", downloadURL)

		executable, err := os.Executable()
		if err != nil {
			return errors.WithStack(err)
		}

		outputDir := filepath.Dir(executable)

		err = getter.Get(outputDir, "http::"+downloadURL)
		if err != nil {
			return errors.Errorf("error downloading from %q: %s", downloadURL, err)
		}

		fmt.Println("Upgrade completed.")

		return nil
	},
})