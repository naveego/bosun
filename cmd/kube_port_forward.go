package cmd

import (
	"fmt"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/kube/portforward"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
)

// kubeCmd represents the kube command
var kubePortForwardCmd = addCommand(kubeCmd, &cobra.Command{
	Use:   "port-forward",
	Aliases: []string{"pf"},
	Short: "Group of commands for managing port forwarding.",
})

var kubePortForwardDaemon = addCommand(kubePortForwardCmd, &cobra.Command{
	Use:   "daemon",
	Short: "Runs the port-forwarding daemon",
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

var kubePortForwardState = addCommand(kubePortForwardCmd, &cobra.Command{
	Use:   "state",
	Aliases: []string{"show","list", "ls"},
	Short: "Reports on state of port forwards",
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error

		controller, err := getKubePortForwardController(args)
		if err != nil {
			return err
		}

		state, err := controller.GetState()
		if err != nil {
			return err
		}

		if state.Error != "" {
			return errors.New(state.Error)
		}


		return printOutputWithDefaultFormat("table", state)
	},
})

var kubePortForwardStart = addCommand(kubePortForwardCmd, &cobra.Command{
	Use:   "start [name]",
	Short: "Starts a port forward task",
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error

		controller, err := getKubePortForwardController(args)
		if err != nil {
			return err
		}


		var name string
		switch len(args){
		case 1:
		name = args[0]
		default:
			state, stateErr := controller.GetState()
			if stateErr != nil {
				return stateErr
			}

			var names []string
			for n, s := range state.Ports {
				if !s.Config.Active {
					names = append(names, n)
				}
			}
			sort.Strings(names)
			if len(names) == 0{
				return errors.New("all port-forwards are running")
			}

			name = cli.RequestChoice("Choose a port-forward to start", names...)
		}

		err = controller.StartPortForward(name)
		return err
	},
})

var kubePortForwardStop = addCommand(kubePortForwardCmd, &cobra.Command{
	Use:   "stop [name]",
	Short: "Stops a port forward task",
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error

		controller, err := getKubePortForwardController(args)
		if err != nil {
			return err
		}


		var name string
		switch len(args){
		case 1:
			name = args[0]
		default:
			state, stateErr := controller.GetState()
			if stateErr != nil {
				return stateErr
			}

			var names []string
			for n, s := range state.Ports {
				if s.Config.Active {
					names = append(names, n)
				}
			}
			sort.Strings(names)
			if len(names) == 0{
				return errors.New("no port-forwards running")
			}

			name = cli.RequestChoice("Choose a port-forward to stop", names...)
		}

		err = controller.StopPortForward(name)
		return err
	},
})

func getKubePortForwardController(args []string) (*portforward.Controller, error) {

	dir := filepath.Join(filepath.Dir(viper.GetString(ArgBosunConfigFile)), "port-forwards")

	controller, err := portforward.NewController(dir)
	return controller, err
}

var kubePortForwardAdd = addCommand(kubePortForwardCmd, &cobra.Command{
	Use:   "add {name} [args...]",
	Args:  cobra.MinimumNArgs(1),
	Short: "Adds a port-forward to the daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error

		controller, err := getKubePortForwardController(args)
		if err != nil {
			return err
		}

		name := args[0]

		var portForwardConfig portforward.PortForwardConfig

		if len(args) > 1 {
			portForwardConfig.Args = args[1:]
		} else {
			tempFile, _ := ioutil.TempFile(os.TempDir(), "port-forward-*.yaml")

			b, _ := yaml.Marshal(portForwardConfig)
			_, err = tempFile.Write(b)
			if err != nil {
				return err
			}

			err = tempFile.Close()
			if err != nil {
				return err
			}

			err = cli.Edit(tempFile.Name())
			if err != nil {
				return err
			}

			err = yaml.LoadYaml(tempFile.Name(), &portForwardConfig)
			if err != nil {
				return err
			}
		}

		err = controller.AddPortForward(name, portForwardConfig)

		return err
	},
})

var kubePortForwardEdit = addCommand(kubePortForwardCmd, &cobra.Command{
	Use:   "edit [name]",
	Short: "Edit a port-forward",
	RunE: func(cmd *cobra.Command, args []string) error {

		controller, err := getKubePortForwardController(args)
		if err != nil {
			return err
		}


		var name string
		switch len(args){
		case 1:
			name = args[0]
		default:
			state, stateErr := controller.GetState()
			if stateErr != nil {
				return stateErr
			}

			var names []string
			for n, s := range state.Ports {
				if s.Config.Active {
					names = append(names, n)
				}
			}
			sort.Strings(names)
			if len(names) == 0{
				return errors.New("no port-forwards found")
			}

			name = cli.RequestChoice("Choose a port-forward to edit", names...)
		}

		portForwardConfig, err := controller.GetPortForwardConfig(name)

		tempFile, _ := ioutil.TempFile(os.TempDir(), "port-forward-*.yaml")

		b, _ := yaml.Marshal(portForwardConfig)
		_, err = tempFile.Write(b)
		if err != nil {
			return err
		}

		err = tempFile.Close()
		if err != nil {
			return err
		}

		err = cli.Edit(tempFile.Name())
		if err != nil {
			return err
		}

		err = yaml.LoadYaml(tempFile.Name(), &portForwardConfig)
		if err != nil {
			return err
		}

		err = controller.AddPortForward(name, *portForwardConfig)

		return err
	},
})
