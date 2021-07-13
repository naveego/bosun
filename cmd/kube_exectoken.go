package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/kube/portforward"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io/ioutil"
	"k8s.io/client-go/pkg/apis/clientauthentication/v1beta1"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"time"
)

var kubeExectoken = addCommand(kubeCmd, &cobra.Command{
	Use:   "execcredential {path}",
	Args:  cobra.ExactArgs(1),
	Short: "Gets a password from lastpass and returns it in the kubeconfig ExecCredential format",
	RunE: func(cmd *cobra.Command, args []string) error {

		dir := filepath.Join(filepath.Dir(viper.GetString(ArgBosunConfigFile)), "port-forwards")
		if len(args) > 0 {
			dir = args[0]
		}

		daemon, err := portforward.NewDaemon(dir)
		if err != nil {
			return err
		}

		err = daemon.Start()
		if err != nil {
			return err
		}

		signalChan := make(chan os.Signal, 1)
		signal.Notify(signalChan, os.Interrupt)

		<-signalChan
		fmt.Println("Received an interrupt, stopping services...")

		return daemon.Stop()
	},
})

var kubeOCICreds = addCommand(kubeCmd, &cobra.Command{
	Use:   "ocicreds {cluster-id} {region}",
	Args:  cobra.ExactArgs(2),
	Short: "Gets creds from OCI but caches them for performance.",
	RunE: func(cmd *cobra.Command, args []string) error {

		clusterID := args[0]
		region := args[1]

		cacheDir := os.ExpandEnv("$HOME/.bosun")
		_ = os.MkdirAll(cacheDir, 0700)

		var output string

		cachePath := path.Join(cacheDir, clusterID)
		cachedCreds, err := ioutil.ReadFile(cachePath)
		if err == nil {

			var execCreds v1beta1.ExecCredential
			err = json.Unmarshal(cachedCreds, &execCreds)
			if err == nil {
				if execCreds.Status.ExpirationTimestamp != nil && execCreds.Status.ExpirationTimestamp.Time.After(time.Now()) {
					output = string(cachedCreds)
				}
			}
		}

		if output == "" {

			output, err = command.NewShellExe("oci", "ce", "cluster", "generate-token", "--cluster-id", clusterID, "--region", region).RunOut()
			if err != nil {
				return errors.Wrap(err, "run oci command")
			}

			err = ioutil.WriteFile(cachePath, []byte(output), 0600)
			if err != nil {
				return errors.Wrap(err, "write to cache")
			}

		}

		fmt.Println(output)

		return nil

	},
})
