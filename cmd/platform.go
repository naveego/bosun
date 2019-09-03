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
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"os"
	"path/filepath"
)

func init() {

}

var platformCmd = addCommand(rootCmd, &cobra.Command{
	Use:   "platform",
	Args:  cobra.NoArgs,
	Short: "Contains platform related sub-commands.",
})

var _ = addCommand(platformCmd, &cobra.Command{
	Use:   "list",
	Short: "Lists platforms.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		platforms, err := b.GetPlatforms()
		if err != nil {
			return err
		}
		for _, e := range platforms {
			fmt.Println(e.Name)
		}
		return nil
	},
})

var _ = addCommand(platformCmd, &cobra.Command{
	Use:          "use [name]",
	Args:         cobra.ExactArgs(1),
	Short:        "Sets the platform.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		err := b.UsePlatform(args[0])
		if err != nil {
			return err
		}

		return b.Save()
	},
})

var _ = addCommand(platformCmd, &cobra.Command{
	Use:   "update-unstable [names...]",
	Short: "Updates the manifests of the provided apps on the unstable branch with the provided apps. Defaults to using the 'develop' branch of the apps.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		ctx := b.NewContext()

		if viper.GetBool(argPlatformUpdateKnown) {
			args = []string{}
			release, err := p.GetUnstableRelease()
			if err != nil {
				return err
			}
			for name := range release.GetAllAppMetadata() {
				args = append(args, name)
			}
		}

		apps := mustGetKnownApps(b, args)

		for _, app := range apps {
			ctx = ctx.WithApp(app)
			ctx.Log.Debug("Refreshing...")

			if !app.IsRepoCloned() {
				ctx.Log.Warn("App is not cloned, refresh will be incomplete.")
				continue
			}

			branch := viper.GetString(argPlatformUpdateBranch)
			if branch == "" {
				branch = app.Branching.GetBranchTemplate(git.BranchTypeDevelop)
			}

			err = p.RefreshApp(ctx, app.Name, branch, bosun.SlotUnstable)
			if err != nil {
				ctx.Log.WithError(err).Warn("Could not refresh.")
			}
		}

		err = p.Save(ctx)

		return err
	},
}, withFilteringFlags, func(cmd *cobra.Command) {
	cmd.Flags().String(argPlatformUpdateBranch, "", "The branch to update from.")
	cmd.Flags().Bool(argPlatformUpdateKnown, false, "If set, updates all apps currently in the unstable release.")
})

const (
	argPlatformUpdateBranch = "branch-type"
	argPlatformUpdateKnown  = "known"
)

var _ = addCommand(platformCmd, &cobra.Command{
	Use:   "include [appNames...]",
	Short: "Adds an app from the workspace to the platform.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}
		ctx := b.NewContext()
		apps := mustGetKnownApps(b, args)
		for _, app := range apps {
			err = p.IncludeApp(ctx, app.Name)
			if err != nil {
				return err
			}
		}

		err = p.Save(ctx)
		return err
	},
}, withFilteringFlags)

var _ = addCommand(platformCmd, &cobra.Command{
	Use:   "add-repo {org/repo...}",
	Args:  cobra.ExactArgs(1),
	Short: "Adds a repo and its apps to the platform.",
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRef, err := issues.ParseRepoRef(args[0])
		if err != nil {
			return err
		}
		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}
		ctx := b.NewContext()
		log := ctx.GetLog()
		ws := b.GetWorkspace()
		path := ""
		for _, gitRoot := range ws.GitRoots {
			dir := filepath.Join(gitRoot, repoRef.String())
			if _, err = os.Stat(dir); err == nil {
				path = dir
				break
			}
		}
		if path != "" {
			log.Infof("Found repo locally at %q", path)
		} else {
			dir, err := getOrAddGitRoot(b, "")
			if err != nil {
				return err
			}
			log.Infof("Cloning repo into %q", dir)
			err = git.CloneRepo(repoRef, ws.GithubCloneProtocol, dir)
			if err != nil {
				return err
			}
			path = filepath.Join(dir, repoRef.String())
		}

		//bosunFilePath := filepath.Join(path, "bosun.yaml")

		err = p.Save(ctx)
		return err
	},
}, withFilteringFlags)

var _ = addCommand(platformCmd, &cobra.Command{
	Use:     "show [name]",
	Args:    cobra.MaximumNArgs(1),
	Aliases: []string{"dump"},
	Short:   "Shows the named platform, or the current platform if no name provided.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		var platform *bosun.Platform
		var err error
		if len(args) == 1 {
			platform, err = b.GetPlatform(args[0])
		} else {
			platform, err = b.GetCurrentPlatform()
		}

		if err != nil {
			return err
		}
		var y []byte
		y, err = yaml.Marshal(platform)
		if err != nil {
			return err
		}

		fmt.Println(string(y))
		return nil
	},
})
