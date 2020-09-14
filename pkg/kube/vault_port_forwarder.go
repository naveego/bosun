package kube

import (
	"github.com/naveego/bosun/pkg"
	"strconv"
	"time"
)

// PortForward creates a long running port forward command that runs until you call
// the return function.  It will keep trying to re open the port indefinitely.
func PortForward(podName, namespace string, port int) func() {
	doneCh := make(chan struct{}, 1)
	stopCh := make(chan struct{}, 1)

	var cmd *pkg.ShellExe
	var isRunning bool
	var isStopping bool

	go func() {
		for {
			select {
			case <-doneCh:
				pkg.Log.Infof("Stopping port forward on pod '%s'", podName)
				if cmd != nil {
					cmd.GetCmd().Process.Kill()
				}
				return
			case <-time.After(3 * time.Second):
				if isRunning == false && isStopping == false {
					isRunning = true

					go func() {
						cmd = pkg.NewShellExe("kubectl",
							"port-forward",
							"-n",
							namespace,
							podName,
							strconv.Itoa(port),
						)

						cmd.RunOut()
						isRunning = false
						if isStopping {
							stopCh <- struct{}{}
						}
					}()
				}
			}
		}
	}()

	return func() {
		isStopping = true
		doneCh <- struct{}{}
		if cmd != nil {
			select {
			case <-stopCh:
			case <-time.After(5 * time.Second):
				return
			}
		}
	}
}
