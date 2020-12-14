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
	"github.com/naveego/bosun/pkg"
	"io/ioutil"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// dockerCmd represents the group of docker-related commands.
var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Group of docker-related commands.",
}


var mapImagesCmd = &cobra.Command{
	Use:   "map-images {map file}",
	Args:  cobra.ExactArgs(1),
	Short: "Retags a list of images",
	Long: `Provide a file with images mapped like
	
x/imageA:0.2.1 y/imageA:0.2.1
x/imageB:0.5.0 x/imageB:0.5.0-rc
`,
	RunE: func(cmd *cobra.Command, args []string) error {

		b, err := ioutil.ReadFile(args[0])
		if err != nil {
			return err
		}
		lines := strings.Split(string(b), "\n")
		for i, line := range lines {
			f := strings.Fields(line)
			if len(f) != 2 {
				return errors.Errorf("invalid line %q at %d", line, i)
			}

			src, dst := f[0], f[1]
			pkg.Log.WithField("@from", src).WithField("@to", dst).Infof("Retagging %d of %d.", i, len(lines))
			new(pkg.ShellExe).WithExe("docker").WithArgs("pull", src).MustRun()
			new(pkg.ShellExe).WithExe("docker").WithArgs("tag", src, dst).MustRun()
			new(pkg.ShellExe).WithExe("docker").WithArgs("push", dst).MustRun()
			//new(ShellExe).WithExe("docker").WithArgs("rmi", src, "--force").MustRun()
			//new(ShellExe).WithExe("docker").WithArgs("rmi", dst, "--force").MustRun()

		}

		return nil
	},
}

func init() {

	dockerCmd.AddCommand(mapImagesCmd)

	rootCmd.AddCommand(dockerCmd)
}
