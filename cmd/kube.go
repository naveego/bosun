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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var pullSecretForce bool

const (
	ArgKubeDashboardUrl = "url"
)

func init() {
	kubeCmd.AddCommand(dashboardTokenCmd)

	rootCmd.AddCommand(kubeCmd)
}

// kubeCmd represents the kube command
var kubeCmd = &cobra.Command{
	Use:   "kube {kube-layout}",
	Args:  cobra.ExactArgs(1),
	Short: "Group of commands wrapping kubectl.",
	Long:  `You must have the cluster set in kubectl.`,
}

var dashboardTokenCmd = &cobra.Command{
	Use:   "dashboard-token",
	Short: "Writes out a dashboard UI access token.",
	Long:  `You must have the cluster set in kubectl.`,
	RunE: func(cmd *cobra.Command, args []string) error {

		secretName, err := pkg.NewCommand("kubectl get serviceaccount kubernetes-dashboard-user -n kube-system -o jsonpath={.secrets[0].name}").RunOut()
		if err != nil {
			return err
		}

		b64, err := pkg.NewCommand(fmt.Sprintf("kubectl get secret %s -n kube-system -o jsonpath={.data.token}", secretName)).RunOut()
		if err != nil {
			return err
		}

		token, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return err
		}

		fmt.Println(string(token))

		return err
	},
}

var kubeAddEKSCmd = addCommand(kubeCmd, &cobra.Command{
	Use:          "add-eks {name} [region]",
	Args:         cobra.RangeArgs(1, 2),
	Short:        "Adds an EKS cluster to your kubeconfig. ",
	Long:         `You must the AWS CLI installed.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		region := "us-east-1"
		if len(args) > 1 {
			region = args[1]
		}
		name := args[0]

		err := pkg.NewCommand("aws", "eks", "--region", region, "update-kubeconfig", "--name", name, "--alias", name).RunE()
		if err != nil {
			return err
		}

		return nil
	},
})

var kubeAddNamespaceCmd = addCommand(kubeCmd, &cobra.Command{
	Use:          "add-namespace {name}",
	Args:         cobra.ExactArgs(1),
	Short:        "Adds a namespace to your cluster. ",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		name := args[0]

		_, err := pkg.NewCommand("kubectl", "create", "namespace", name).RunOut()
		if err != nil {
			if strings.Contains(err.Error(), "AlreadyExists") {
				color.Yellow("Namespace %q already exists.", name)
				return nil
			}
			return err
		}

		return nil
	},
})

var dashboardCmd = addCommand(kubeCmd, &cobra.Command{
	Use:   "dashboard",
	Short: "Opens dashboard for current cluster.",
	Long:  `You must have the cluster set in kubectl.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		ns := "kube-system"
		svc := "kubernetes-dashboard"

		p, hostPort, err := kubectlProxy()
		if err != nil {
			return errors.Wrap(err, "kubectl proxy")
		}
		url := dashboardURL(hostPort, ns, svc)

		if viper.GetBool(ArgKubeDashboardUrl) {
			fmt.Fprintln(os.Stdout, url)
		} else {
			fmt.Fprintln(os.Stdout, fmt.Sprintf("Opening %s in your default browser...", url))
			if err = browser.OpenURL(url); err != nil {
				fmt.Fprintf(os.Stderr, fmt.Sprintf("failed to open browser: %v", err))
			}
		}

		if err = p.Wait(); err != nil {
			return err
		}
		return nil
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(ArgKubeDashboardUrl, false, "Display dashboard URL instead of opening a browser")
})

var pullSecretCmd = addCommand(kubeCmd, &cobra.Command{
	Use:   "pull-secret [username] [password]",
	Args:  cobra.RangeArgs(0, 2),
	Short: "Sets a pull secret in kubernetes.",
	Long:  `If username and password not provided then the value from your docker config will be used.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error
		viper.BindPFlags(cmd.Flags())

		namespace := viper.GetString(ArgKubePullSecretNamespace)

		name := viper.GetString(ArgKubePullSecretName)
		target := viper.GetString(ArgKubePullSecretTarget)

		force := viper.GetBool("force")
		if !force {
			out, err := pkg.NewCommand(fmt.Sprintf("kubectl get secret %s -n %s", name, namespace)).RunOut()
			fmt.Println(out)
			if err == nil {
				color.Yellow("Pull secret already exists (run with --force parameter to overwrite).")
				return nil
			}
		} else {
			_ = pkg.NewCommand(fmt.Sprintf("kubectl delete secret %s -n %s", name, namespace)).RunE()

		}

		var username string
		var password string

		if len(args) == 0 {
			var dockerConfig map[string]interface{}
			dockerConfigPath, ok := os.LookupEnv("DOCKER_CONFIG")
			if !ok {
				dockerConfigPath = os.ExpandEnv("$HOME/.docker/config.json")
			}
			data, err := ioutil.ReadFile(dockerConfigPath)
			if err != nil {
				return errors.Errorf("error reading docker config from %q: %s", dockerConfigPath, err)
			}

			err = json.Unmarshal(data, &dockerConfig)
			if err != nil {
				return errors.Errorf("error docker config from %q, file was invalid: %s", dockerConfigPath, err)
			}

			auths, ok := dockerConfig["auths"].(map[string]interface{})

			entry, ok := auths[target].(map[string]interface{})
			if !ok {
				return errors.Errorf("no %q entry in docker config, you should docker login first", target)
			}
			authBase64, _ := entry["auth"].(string)
			auth, err := base64.StdEncoding.DecodeString(authBase64)
			if err != nil {
				return errors.Errorf("invalid %q entry in docker config, you should docker login first: %s", target, err)
			}
			segs := strings.Split(string(auth), ":")
			username, password = segs[0], segs[1]
		} else {
			if len(args) > 0 {
				username = args[0]
			} else if viper.GetString(ArgKubePullSecretUsername) != "" {
				username = viper.GetString(ArgKubePullSecretUsername)
			} else {
				username = pkg.RequestStringFromUser("Please provide username")
			}

			if len(args) == 2 {
				password = args[1]
			} else if viper.GetString(ArgKubePullSecretPassword) != "" {
				password = viper.GetString(ArgKubePullSecretPassword)
			} else if viper.GetString(ArgKubePullSecretPasswordLpassPath) != "" {
				path := viper.GetString(ArgKubePullSecretPasswordLpassPath)
				pkg.Log.WithField("path", path).Info("Trying to get password from LastPass.")
				password, err = pkg.NewCommand("lpass", "show", "--password", path).RunOut()
				if err != nil {
					return err
				}
			} else {
				password = pkg.RequestSecretFromUser("Please provide password for user %s", username)
			}
		}

		err = pkg.NewCommand("kubectl",
			"create", "secret", "docker-registry",
			name,
			"-n", namespace,
			fmt.Sprintf("--docker-server=%s", target),
			fmt.Sprintf("--docker-username=%s", username),
			fmt.Sprintf("--docker-password=%s", password),
			fmt.Sprintf("--docker-email=%s", username),
		).RunE()
		if err != nil {
			return err
		}

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&pullSecretForce, "force", "f", false, "Force create (overwrite) the secret even if it already exists.")
	cmd.Flags().String(ArgKubePullSecretName, "docker-n5o-black", "Name of pull secret in k8s.")
	cmd.Flags().String(ArgKubePullSecretTarget, "docker.n5o.black", "Domain of docker repository.")
	cmd.Flags().String(ArgKubePullSecretUsername, "", "User for pulling from docker.")
	cmd.Flags().String(ArgKubePullSecretPassword, "", "Secret password for pulling from docker.")
	cmd.Flags().String(ArgKubePullSecretPasswordLpassPath, "", "FromPath in LastPass for the password for pulling from docker harbor.")

	cmd.Flags().String(ArgKubePullSecretNamespace, "default", "The namespace to deploy the secret into.")
})

const (
	ArgKubePullSecretName              = "name"
	ArgKubePullSecretTarget            = "target"
	ArgKubePullSecretUsername          = "username"
	ArgKubePullSecretPassword          = "password"
	ArgKubePullSecretPasswordLpassPath = "password-path"
	ArgKubePullSecretNamespace         = "namespace"
)

// kubectlProxy runs "kubectl proxy", returning host:port
func kubectlProxy() (*exec.Cmd, string, error) {
	path, err := exec.LookPath("kubectl")
	if err != nil {
		return nil, "", errors.Wrap(err, "kubectl not found in PATH")
	}

	// port=0 picks a random system port
	// config.GetMachineName() respects the -p (profile) flag
	cmd := exec.Command(path, "proxy", "--port=0")
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", errors.Wrap(err, "cmd stdout")
	}

	pkg.Log.Infof("Executing: %s %s", cmd.Path, cmd.Args)
	if err := cmd.Start(); err != nil {
		return nil, "", errors.Wrap(err, "proxy start")
	}

	pkg.Log.Infof("Waiting for kubectl to output host:port ...")
	reader := bufio.NewReader(stdoutPipe)

	var out []byte
	for {
		r, timedOut, err := readByteWithTimeout(reader, 5*time.Second)
		if err != nil {
			return cmd, "", fmt.Errorf("readByteWithTimeout: %v", err)
		}
		if r == byte('\n') {
			break
		}
		if timedOut {
			pkg.Log.Infof("timed out waiting for input: possibly due to an old kubectl version.")
			break
		}
		out = append(out, r)
	}
	pkg.Log.Infof("proxy stdout: %s", string(out))
	return cmd, hostPortRe.FindString(string(out)), nil
}

// Matches: 127.0.0.1:8001
var hostPortRe = regexp.MustCompile(`127.0.0.1:\d{4,}`)

// readByteWithTimeout returns a byte from a reader or an indicator that a timeout has occurred.
func readByteWithTimeout(r io.ByteReader, timeout time.Duration) (byte, bool, error) {
	bc := make(chan byte)
	ec := make(chan error)
	go func() {
		b, err := r.ReadByte()
		if err != nil {
			ec <- err
		} else {
			bc <- b
		}
		close(bc)
		close(ec)
	}()
	select {
	case b := <-bc:
		return b, false, nil
	case err := <-ec:
		return byte(' '), false, err
	case <-time.After(timeout):
		return byte(' '), true, nil
	}
}

// dashboardURL generates a URL for accessing the dashboard service
func dashboardURL(proxy string, ns string, svc string) string {
	// Reference: https://github.com/kubernetes/dashboard/wiki/Accessing-Dashboard---1.7.X-and-above
	return fmt.Sprintf("http://%s/api/v1/namespaces/%s/services/http:%s:/proxy/", proxy, ns, svc)
}
