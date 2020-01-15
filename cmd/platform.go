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
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"os"
	"path/filepath"
	"strings"
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
	Use:   "update {stable|unstable} [names...]",
	Args:  cobra.MinimumNArgs(1),
	Short: "Updates the manifests of the provided apps on the unstable branch with the provided apps. Defaults to using the 'develop' branch of the apps.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		ctx := b.NewContext()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		slot := args[0]
		switch slot {
		case bosun.SlotStable, bosun.SlotUnstable:
		default:
			return errors.Errorf("invalid slot, wanted %s or %s, got %q", bosun.SlotStable, bosun.SlotUnstable, slot)
		}

		release, err := p.GetReleaseManifestBySlot(slot)
		if err != nil {
			return err
		}
		if viper.GetBool(argPlatformUpdateKnown) {
			args = []string{}
			for name := range release.GetAllAppMetadata() {
				args = append(args, name)
			}
		} else if viper.GetBool(argPlatformUpdateDeployed) {
			args = []string{}
			for name := range release.UpgradedApps {
				args = append(args, name)
			}
		}

		apps := mustGetKnownApps(b, args)

		for _, app := range apps {
			ctx = ctx.WithApp(app)
			ctx.Log().Debug("Refreshing...")

			if !app.IsRepoCloned() {
				ctx.Log().Warn("App is not cloned, refresh will be incomplete.")
				continue
			}

			branch := viper.GetString(argPlatformUpdateBranch)
			switch branch {
			case string(git.BranchTypeRelease):
				branch, err = app.Branching.RenderRelease(release.GetBranchParts())
				if err != nil {
					return err
				}
			case "":
				branch = app.Branching.GetBranchTemplate(git.BranchTypeDevelop)
			}

			err = p.RefreshApp(ctx, app.Name, branch, slot)
			if err != nil {
				ctx.Log().WithError(err).Warn("Could not refresh.")
			}
		}

		err = p.Save(ctx)

		return err
	},
}, withFilteringFlags, func(cmd *cobra.Command) {
	cmd.Flags().String(argPlatformUpdateBranch, "", "The branch to update from.")
	cmd.Flags().Bool(argPlatformUpdateKnown, false, "If set, updates all apps currently in the release.")
	cmd.Flags().Bool(argPlatformUpdateDeployed, false, "If set, updates all apps currently marked to be deployed in the release.")
})

const (
	argPlatformUpdateBranch   = "branch"
	argPlatformUpdateKnown    = "known"
	argPlatformUpdateDeployed = "deployed"
)

var _ = addCommand(platformCmd, &cobra.Command{
	Use:   "include {name} --cluster-roles {cluster-role, ...} --namespace-roles {namespace-role, ...)",
	Args:  cobra.ExactArgs(1),
	Short: "Adds an app from the workspace to the platform.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}
		ctx := b.NewContext()
		app := mustGetApp(b, []string{args[0]})

		repoRef, err := issues.ParseRepoRef(app.RepoName)
		if err != nil {
			return err
		}

		clusterRoles := viper.GetStringSlice(argPlatformAddClusterRoles)
		if len(clusterRoles) == 0 {
			return errors.Errorf("at least one cluster role must be specified using --%s", argPlatformAddClusterRoles)
		}
		namespaceRoles := viper.GetStringSlice(argPlatformAddNamespaceRoles)
		if len(namespaceRoles) == 0 {
			return errors.Errorf("at least one namespace role must be specified using --%s", argPlatformAddNamespaceRoles)
		}

		err = p.IncludeApp(ctx, &bosun.PlatformAppConfig{
			Name:           app.Name,
			RepoRef:        repoRef,
			ClusterRoles:   core.ClusterRolesFromStrings(clusterRoles),
			NamespaceRoles: core.NamespaceRolesFromStrings(namespaceRoles),
		})
		if err != nil {
			return err
		}

		err = p.Save(ctx)
		return err
	},
}, withFilteringFlags,
	func(cmd *cobra.Command) {
		cmd.Flags().StringSlice(argPlatformAddClusterRoles, []string{}, "The cluster roles this app should be deployed to.")
		cmd.Flags().StringSlice(argPlatformAddNamespaceRoles, []string{}, "The namespace roles this app should be deployed to.")
	})

const (
	argPlatformAddClusterRoles   = "cluster-roles"
	argPlatformAddNamespaceRoles = "namespace-roles"
)

var _ = addCommand(platformCmd, &cobra.Command{
	Use:   "add-value-overrides {appName} {override-name} {cluster|clusterRole|environment={value,...} ...}",
	Args:  cobra.MinimumNArgs(2),
	Short: "Adds default values for an app to a cluster.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		matches := map[string][]string{}
		pairs := args[2:]
		for _, pair := range pairs {
			keyValues := strings.Split(pair, "=")
			if len(keyValues) != 2 {
				return errors.New("invalid match values")
			}
			key := keyValues[0]
			values := strings.Split(keyValues[1], ",")
			matches[key] = values
		}

		ctx := b.NewContext()

		err = p.AddAppValuesForCluster(ctx, args[0], args[1], matches)

		if err != nil {
			return err
		}

		return p.Save(b.NewContext())
	},
})

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
		log := ctx.Log()
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

		// bosunFilePath := filepath.Join(path, "bosun.yaml")

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

var _ = addCommand(platformCmd, &cobra.Command{
	Use:   "tree",
	Args:  cobra.ExactArgs(0),
	Short: "Prints off the apps in the platform in dependency order.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		var appNames []string
		for _, app := range p.Apps {
			appNames = append(appNames, app.Name)
		}

		appsAndDeps, err := p.GetAppsAndDependencies(b, bosun.CreateDeploymentPlanRequest{
			Apps: appNames,
		})

		if err != nil {
			return err
		}

		for _, appName := range appsAndDeps.TopologicalOrder {
			fmt.Println(color.BlueString("- %s", appName), color.WhiteString(" : %v", appsAndDeps.Dependencies[appName]))
		}

		return nil

	},
})
