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
	"gopkg.in/yaml.v2"
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
		b := mustGetBosun()
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
	Use:   "use [name]",
	Args:  cobra.ExactArgs(1),
	Short: "Sets the platform.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()
		err := b.UsePlatform(args[0])
		if err != nil {
			err = b.Save()
		}
		return err
	},
})

var _ = addCommand(platformCmd, &cobra.Command{
	Use:   "pull [names...]",
	Short: "Pulls the latest code, and updates the `latest` release.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		ctx := b.NewContext()
		apps := mustGetKnownApps(b, args)

		for _, app := range apps {
			ctx = ctx.WithApp(app)
			ctx.Log.Debug("Refreshing...")

			if !app.IsRepoCloned() {
				ctx.Log.Warn("App is not cloned, refresh will be incomplete.")
				continue
			}

			err = p.RefreshApp(ctx, app.Name)
			if err != nil {
				ctx.Log.WithError(err).Warn("Could not refresh.")
			}
		}

		err = p.Save(ctx)

		return err
	},
}, withFilteringFlags)

var _ = addCommand(platformCmd, &cobra.Command{
	Use:   "include [appNames...]",
	Short: "Adds an app from the workspace to the platform.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()
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
	Use:     "show [name]",
	Args:    cobra.MaximumNArgs(1),
	Aliases: []string{"dump"},
	Short:   "Shows the named platform, or the current platform if no name provided.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := mustGetBosun()
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
