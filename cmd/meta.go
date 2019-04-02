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
	"encoding/json"
	"fmt"
	"github.com/coreos/go-semver/semver"
	"github.com/google/go-github/v20/github"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"gopkg.in/inconshreveable/go-update.v0"
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

var metaVersionCmd = addCommand(metaCmd, &cobra.Command{
	Use:   "version",
	Short: "Shows bosun version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf(`Version: %s\n
Timestamp: %s\n
Commit: %s\n
`, Version, Timestamp, Commit)
	},
})

var metaUpgradeCmd = addCommand(metaCmd, &cobra.Command{
	Use:"upgrade",
	Short:"Upgrades bosun if a newer release is available",
	SilenceUsage:true,
	RunE: func(cmd *cobra.Command, args []string) error {

		client := mustGetGithubClient()
		ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
var err error
		if Version == "" {
			Version, err = pkg.NewCommand("bosun", "app", "version", "bosun").RunOut()
			if err != nil {
				return errors.Wrap(err, "could not get version")
			}
		}

		currentVersion, err := semver.NewVersion(Version)

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
			fmt.Printf("Current version (%s) is up-to-date.\n", Version)
			return nil
		}

		pkg.Log.Infof("Found upgrade: %s", release.GetTagName())


		expectedAssetName := fmt.Sprintf("bosun_%s_%s_%s.tar.gz", release.GetTagName(), runtime.GOOS, runtime.GOARCH)
		var foundAsset bool
		var asset github.ReleaseAsset
		for _, asset = range release.Assets {
			name := asset.GetName()
			if name == expectedAssetName {
				foundAsset = true
				break
			}
		}
		if !foundAsset {
			return errors.Errorf("could not find an asset with name %q", expectedAssetName)
		}

		j, _ := json.MarshalIndent(asset, "", "  ")
		fmt.Println(string(j))


		tempDir, err := ioutil.TempDir(os.TempDir(), "bosun-upgrade")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tempDir)

		downloadURL := asset.GetBrowserDownloadURL()
		pkg.Log.Infof("Found upgrade asset, will download from %q to %q", downloadURL, tempDir)


		err = getter.Get(tempDir, "http::"+downloadURL)
		if err != nil {
			return errors.Errorf("error downloading from %q: %s", downloadURL, err)
		}

		executable, err := os.Executable()
		if err != nil {
			return errors.WithStack(err)
		}

		newVersion := filepath.Join(tempDir, filepath.Base(executable))

		err, errRecover := update.New().FromFile(newVersion)
		if err != nil {
			return err
		}
		if errRecover != nil {
			return errRecover
		}

		fmt.Println("Upgrade completed.")

		return nil
	},
})