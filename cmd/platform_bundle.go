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
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"path/filepath"
	"strings"
)

func init() {

}

var bundleCmd = addCommand(platformCmd, &cobra.Command{
	Use:   "bundle",
	Short: "Bundling commands",
})

var _ = addCommand(bundleCmd, &cobra.Command{
	Use:          "create {name}",
	Args:         cobra.ExactArgs(1),
	Short:        "Creates a portable bundle zip from a platform and the active release.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		envNames := viper.GetStringSlice(argBundleCreateEnvs)
		if len(envNames) == 0 {
			envs, e := b.GetEnvironments()
			check(e)
			var envNameOptions []string
			for _, env := range envs {
				envNameOptions = append(envNameOptions, env.Name)
			}
			envNames = cli.RequestMultiChoice("Choose environments to include", envNameOptions)
		}

		release := cli.RequestChoiceIfEmpty(
			viper.GetString(argBundleCreateRelease),
			"Choose release to include",
			"current",
			"stable",
			"unstable",
		)

		bundler := bosun.NewPlatformBundler(b, p)

		result, err := bundler.Execute(bosun.BundlePlatformRequest{
			Prefix:       viper.GetString(args[0]),
			Environments: args,
			Releases:     []string{release},
		})
		if err != nil {
			return err
		}

		fmt.Println(result.OutPath)

		return nil
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().StringSlice(argBundleCreateEnvs, []string{}, "Environments to include")
	cmd.Flags().String(argBundleCreateRelease, "", "The release to include")
})

const (
	argBundleCreateEnvs    = "envs"
	argBundleCreateRelease = "release"
)

var _ = addCommand(bundleCmd, &cobra.Command{
	Use:          "push [name]",
	Args:         cobra.MaximumNArgs(1),
	Short:        "Pushes a bundle to a kubernetes cluster.",
	Long:         "By default, pushes to the current cluster selected in the kubeconfig",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		var bundleRef string
		bundleDir := p.ResolveRelative(p.EnvironmentDirectory)
		bundlePaths, _ := filepath.Glob(filepath.Join(bundleDir, "*.bundle"))
		if len(args) == 1 {
			bundleRef = args[0]
		} else {
			bundleRef = cli.RequestChoice("Choose a bundle", bundlePaths...)
		}

		var bundleCandidates []string
		for _, bundlePath := range bundlePaths {
			if strings.Contains(bundlePath, bundleRef) {
				bundleCandidates = append(bundleCandidates, bundlePath)
			}
		}
		if len(bundleCandidates) > 1 {
			return errors.Errorf("Multiple bundles matched name %q: %v (from %v)", bundleRef, bundleCandidates, bundlePaths)
		}

		bundleRef = bundlePaths[0]

		bundleContent, err := ioutil.ReadFile(bundleRef)
		if err != nil {
			return err
		}
		bundleHash := util.HashBytesToString(bundleContent)[0:63]

		k, err := kube.GetKubeClient()
		if err != nil {
			return err
		}

		namespace := viper.GetString(argBundlePushNamespace)
		configMapClient := k.CoreV1().ConfigMaps(namespace)

		bundleName := "bosun-bundle-" + strings.TrimSuffix(filepath.Base(bundleRef), ".bundle")

		bundleConfigMap := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:bundleName,
				Labels: map[string]string{
					LabelBundleConfigMapHash: bundleHash,
				},
			},
			BinaryData: map[string][]byte{
				"bundle": bundleContent,
			},
		}

		_, err = configMapClient.Create(bundleConfigMap)
		if kerrors.IsAlreadyExists(err) {
			_, err = configMapClient.Update(bundleConfigMap)
		}
		if err != nil {
			return errors.Wrapf(err, "create config map named %q", bundleConfigMap.Name)
		}

		fmt.Printf("Pushed to current cluster, namespace %q, configmap name %q", namespace, bundleConfigMap.Name)

		return nil
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().String(argBundlePushNamespace, "default", "The namespace to place the bundle in.")
})

const (
	argBundlePushNamespace = "namespace"
	LabelBundleConfigMapHash = "naveego.com/bosun-bundle-hash"
)

