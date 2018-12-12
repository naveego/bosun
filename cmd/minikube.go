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
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

// minikubeCmd represents the minikube command
var minikubeCmd = &cobra.Command{
	Use:   "minikube",
	Args:  cobra.ExactArgs(1),
	Short: "Group of commands wrapping kubectl.",
	Long:  `You must have the cluster set in kubectl.`,
}

var minikubeUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Brings up minikube if it's not currently running.",
	RunE: func(cmd *cobra.Command, args []string) error {

		c := pkg.MinikubeCmd{}

		err := c.Up()

		return err
	},
}

var minikubePortForward = &cobra.Command{
	Use:   "forward",
	Short: "Forwards ports to the services running on minikube",
	RunE: func(cmd *cobra.Command, args []string) error {

		_, err := exec.LookPath("kubectl")
		if err != nil {
			return errors.Wrap(err, "no kubectl")
		}

		cmdMap := map[string]*exec.Cmd{}

		for i, v := range portForwards {
			segs := strings.Split(v, ":")
			if len(segs) != 2 {
				return errors.Errorf("services must be in the format serviceName:port, but argument %q at index %d was not", v, i)
			}
			svc, port := segs[0], segs[1]
			pkg.Log.WithField("svc", svc).WithField("port",port).Debug("Configuring service...")
			cmdMap[v] = exec.Command("kubectl", "port-forward", "services/"+svc, port)
		}

		wg := new(sync.WaitGroup)

		for l := range cmdMap {
			wg.Add(1)

			// capture closure
			label, c := l, cmdMap[l]

			go func() {
				log := pkg.Log.WithField("service", label)
				defer wg.Done()
				log.Debug("Starting forward.")

				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				err := c.Start()
				if err != nil {
					log.WithError(err).Error("Error on startup.")
					return
				}

				err = c.Wait()
				if err != nil {
					log.WithError(err).Error("Error on wait.")
				}
				log.Debug("Stopped.")

				return
			}()
		}

		done := make(chan struct{})

		go func() {
			<-time.After(100*time.Millisecond)
			wg.Wait()
			close(done)
		}()

		signals := make(chan os.Signal)
		signal.Notify(signals, os.Kill, os.Interrupt)

		for {
			select {
			case <-signals:
				fmt.Println("User quit.")

				for label, c := range cmdMap {
					log := pkg.Log.WithField("service", label)
					if c.Process == nil {
						log.Debug("Not started.")
						continue
					}
					log.Debug("Stopping.")

					err := c.Process.Kill()
					if err != nil {
						log.WithError(err).Error("Error stopping.")
					}

					log.Debug("Stopped.")
				}


				go func() {
					<-time.After(5 * time.Second)
					panic("Timed out stopping children.")
				}()

			case <-done:
				pkg.Log.Info("All forwards terminated.")
				return nil
			}
		}

	},
}

var portForwards []string

func init() {
	minikubeCmd.AddCommand(minikubeUpCmd)

	minikubePortForward.Flags().StringSliceVarP(&portForwards, "services", "s", []string{}, "Services to forward.")

	minikubeCmd.AddCommand(minikubePortForward)

	rootCmd.AddCommand(minikubeCmd)
}
