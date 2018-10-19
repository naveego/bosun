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
	"io/ioutil"
	"strings"

	"github.com/naveego/bosun/internal"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// dockerCmd represents the group of docker-related commands.
var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Group of docker-related commands.",
}

// tagImageCmd represents the tagImage command
var tagImageCmd = &cobra.Command{
	Use:   "choose-release-image {service-name}:{version-tag}",
	Args:  cobra.ExactArgs(1),
	Short: "Tags an image for release.",
	Long:  `The image must be on docker.n5o.black/private. If the current branch is a marketing release number it will be used, otherwise you will be prompted.`,
	RunE: func(cmd *cobra.Command, args []string) error {

		marketingRelease, err := getMarketingRelease()
		if err != nil {
			return err
		}

		imageTag := args[0]

		segs := strings.Split(imageTag, ":")
		if len(segs) != 2 {
			return errors.Errorf("invalid image:tag %q", imageTag)
		}

		src := fmt.Sprintf("docker.n5o.black/private/%s", imageTag)
		dst := fmt.Sprintf("docker.n5o.black/private/%s-%s", imageTag, marketingRelease)

		fmt.Printf("tagging image %q for release %q\n", src, marketingRelease)

		new(internal.Command).WithExe("docker").WithArgs("pull", src).MustRun()
		new(internal.Command).WithExe("docker").WithArgs("tag", src, dst).MustRun()
		new(internal.Command).WithExe("docker").WithArgs("push", dst).MustRun()

		return nil

	},
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
			internal.Log.WithField("@from", src).WithField("@to", dst).Infof("Retagging %d of %d.", i, len(lines))
			new(internal.Command).WithExe("docker").WithArgs("pull", src).MustRun()
			new(internal.Command).WithExe("docker").WithArgs("tag", src, dst).MustRun()
			new(internal.Command).WithExe("docker").WithArgs("push", dst).MustRun()
			//new(internal.Command).WithExe("docker").WithArgs("rmi", src, "--force").MustRun()
			//new(internal.Command).WithExe("docker").WithArgs("rmi", dst, "--force").MustRun()

		}

		return nil
	},
}

func init() {

	tagImageCmd.Flags().String(ArgHelmsmanMarketingRelease, "", "The value of {{ .MarketingRelease }} in the template. If not set, will default to the current branch.")

	dockerCmd.AddCommand(tagImageCmd)
	dockerCmd.AddCommand(mapImagesCmd)

	rootCmd.AddCommand(dockerCmd)
}
