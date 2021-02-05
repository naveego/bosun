package cmd

import (
	"fmt"
	"github.com/naveego/bosun/pkg/kube/portforward"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"os/signal"
	"path/filepath"
)

var kubeExectoken = addCommand(kubeCmd, &cobra.Command{
	Use:   "execcredential {path}",
	Args: cobra.ExactArgs(1),
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