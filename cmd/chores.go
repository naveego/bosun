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
	"bufio"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/util/stringsn"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var choresCmd = addCommand(rootCmd, &cobra.Command{
	Use:     "chores",
	Aliases: []string{"chore"},
	Short:   "Commands for automating tiresome tasks, like migrating bosun files.",
})

var choresImportDescendentsCmd = addCommand(choresCmd, &cobra.Command{
	Use:   "import-descendents {into-file} {pattern} [anti-pattern]",
	Args:  cobra.RangeArgs(2, 3),
	Short: "Adds all bosun files in the directory containing {into-file} to the imports in that file.",
	RunE: func(cmd *cobra.Command, args []string) error {
		targetPath, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		targetDir := filepath.Dir(targetPath)

		log := pkg.Log
		pattern, err := regexp.Compile(args[1])
		if err != nil {
			return err
		}

		antipattern := regexp.MustCompile("NEVER")
		if len(args) > 2 {
			antipattern, err = regexp.Compile(args[2])
			if err != nil {
				return errors.Wrap(err, "parse antipattern")
			}
		}

		var file *bosun.File
		err = yaml.LoadYaml(targetPath, &file)
		if err != nil {
			return err
		}
		file.SetFromPath(targetPath)

		err = filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			if path == targetPath {
				return nil
			}

			if pattern.MatchString(path) && !antipattern.MatchString(path) {
				path = strings.Replace(path, targetDir, ".", 1)
				log.Infof("Adding path: %s", path)
				file.Imports = stringsn.AppendIfNotPresent(file.Imports, path)
			}

			return nil
		})

		if err != nil {
			return err
		}

		err = file.Save()

		return err
	},
})

var choresMigrateFilesCmd = addCommand(choresCmd, &cobra.Command{
	Use:   "migrate-files {files...}",
	Short: "Opens and saves the listed files. If no files provided reads from stdin.",
	RunE: func(cmd *cobra.Command, args []string) error {

		files := args
		if len(files) == 0 {
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				files = append(files, scanner.Text())
			}
		}

		for _, path := range files {
			var file *bosun.File
			err := yaml.LoadYaml(path, &file)
			if err != nil {
				return err
			}
			file.SetFromPath(path)

			migrateAppsToRolePatterns(file)

			err = file.Save()
			if err != nil {
				return err
			}
		}

		return nil
	},
})

var moveAppsToChartFilesCmd = addCommand(choresCmd, &cobra.Command{
	Use:   "move-app-to-chart {app}",
	Short: "Moves the app config for an app into the folder containing the chart for the app.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {

		b := MustGetBosun()
		app := mustGetApp(b, args)

		if app.ChartPath == "" {
			return errors.New("no chart path")
		}

		chartPath := app.ResolveRelative(app.ChartPath)

		bosunPath := filepath.Join(chartPath, "bosun.yaml")

		oldFile := app.FileSaver.(*bosun.File)

		newFile := &bosun.File{
			FromPath: bosunPath,
			Apps: []*bosun.AppConfig{
				app.AppConfig,
			},
		}

		// clean up app
		app.Files = []string{
			".",
		}
		app.ChartPath = "."

		migrateAppToRolePatterns(app.AppConfig)

		err := newFile.Save()
		if err != nil {
			return err
		}

		var appsWithoutApp []*bosun.AppConfig
		for _, oldApp := range oldFile.Apps {
			if oldApp.Name != app.Name {
				appsWithoutApp = append(appsWithoutApp, oldApp)
			}
		}
		oldFile.Apps = appsWithoutApp

		relPath, _ := filepath.Rel(filepath.Dir(app.FromPath), bosunPath)
		oldFile.Imports = append(oldFile.Imports, relPath)

		err = oldFile.Save()
		return err
	},
})

var roleMap = map[core.EnvironmentRole]core.EnvironmentRole{
	"blue":    "prod",
	"preprod": "prod",
	"purple":  "prod",
	"prod":    "prod",
	"qa":      "nonprod",
	"uat":     "nonprod",
	"nonprod": "nonprod",
	"red":     "local",
	"local":   "local",
}

func mapRoles(input []core.EnvironmentRole) []core.EnvironmentRole {
	var order []core.EnvironmentRole
	unique := map[core.EnvironmentRole]bool{}
	for _, role := range input {
		if mapped, ok := roleMap[role]; ok {
			if !unique[mapped] {
				order = append(order, mapped)
			}
			unique[mapped] = true
		}
	}
	return order
}

func migrateAppsToRolePatterns(file *bosun.File) {

	for i, app := range file.Apps {
		migrateAppToRolePatterns(app)
		file.Apps[i] = app
	}

}

func migrateAppToRolePatterns(app *bosun.AppConfig) {

	for i, vs := range app.Values.ValueSets {
		vs.Roles = mapRoles(vs.Roles)
		app.Values.ValueSets[i] = vs
	}

	for i, a := range app.Actions {
		where := mapRoles(a.Where)
		if a.WhereFilter == nil {
			a.WhereFilter = filter.MatchMapConfig{}
		}
		a.WhereFilter[core.KeyEnvironmentRole] = nil
		for _, r := range where {
			a.WhereFilter[core.KeyEnvironmentRole] = append(a.WhereFilter[core.KeyEnvironmentRole], filter.MatchMapConfigValue(r))
		}
		a.Where = nil

		hasFilters := false
		for _, v := range a.WhereFilter {
			if len(v) > 0 {
				hasFilters = true
			}
		}
		if !hasFilters {
			a.WhereFilter = nil
		}

		app.Actions[i] = a


	}

}
