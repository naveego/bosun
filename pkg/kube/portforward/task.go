package portforward

import (
	"bufio"
	"fmt"
	"github.com/sirupsen/logrus"
	"gopkg.in/tomb.v2"
	"io"
	"os/exec"
	"time"
)

type portForwardTask struct {
	config PortForwardConfig
	log    *logrus.Entry
	name   string
	daemon *PortForwardDaemon
	t      *tomb.Tomb
	cmd    *exec.Cmd
}

func newPortForward(p *PortForwardDaemon, name string, config PortForwardConfig) (*portForwardTask, error) {

	return &portForwardTask{
		name:   name,
		config: config,
		daemon: p,
		log:    p.log.WithField("task", name),
	}, nil

}

func (t *portForwardTask) updateState(mutator func(state *PortForwardState)) {
	t.daemon.updatePFState(t.name, mutator)
}

func (t *portForwardTask) Start() {

	if t.t != nil && t.t.Alive() {
		t.log.Warn("Already started.")
	}

	ctx := t.daemon.t.Context(nil)
	t.t, _ = tomb.WithContext(ctx)

	t.t.Go(func() (err error) {

		defer func() { err = t.daemon.recordPanic() }()

		maxTimeout := 60 * time.Second
		minTimeout := 1 * time.Second
		currentTimeout := minTimeout

		for t.t.Alive() {

			lastAttempt := time.Now()

			args := t.config.Args

			if len(args) == 0 {
				args = []string{"port-forward"}

				if t.config.Namespace != "" {
					args = append(args, "--namespace", t.config.Namespace)
				}

				if t.config.KubeContext != "" {
					args = append(args, "--context", t.config.KubeContext)
				}

				if t.config.KubeConfig != "" {
					args = append(args, "--kubeconfig", t.config.KubeConfig)
				}

				if t.config.TargetType != "" && t.config.TargetName != "" {
					args = append(args, t.config.TargetType+"/"+t.config.TargetName)
				} else {
					args = append(args, t.config.TargetName)
				}
			}

			args = append(args, fmt.Sprintf("%d:%d", t.config.LocalPort, t.config.TargetPort))

			t.log.Infof("Starting command with args %v", args)

			cmd := exec.Command("kubectl", args...)

			stderr, _ := cmd.StderrPipe()
			go pipeToLog(fmt.Sprintf("STDERR: %s: ", t.name), t.log, stderr)

			cmdErr := cmd.Start()
			if cmdErr != nil {
				t.log.WithError(cmdErr).Errorf("Command with args %v couldn't start.", args)
				t.updateState(func(state *PortForwardState) {
					state.State = "StartFailed"
					state.Error = cmdErr.Error()
				})
			} else {

				doneCh := make(chan struct{})

				t.log.Infof("Command with args %v started.", args)
				go func() {
					waitErr := cmd.Wait()
					t.log.WithError(waitErr).Infof("Command with args %v stopped.", args)
					close(doneCh)
				}()

				t.updateState(func(state *PortForwardState) {
					state.State = "Running"
					state.PID = cmd.Process.Pid
					state.Error = ""
				})

				select {
				case <-t.t.Dying():
					t.log.Infof("Task stop requested, stopping command...")
					_ = cmd.Process.Kill()

					select {
					case <-time.After(5 * time.Second):
						t.log.Warnf("Task stop requested, but command with pid %d didn't stop, you may need to stop it.", cmd.Process.Pid)
					case <-doneCh:
						t.updateState(func(state *PortForwardState) {
							state.State = "Stopped"
							state.Error = ""
							state.PID = 0
						})
					}
				case <-doneCh:
					t.log.Infof("Task stopped unexpectedly, will restart.")
				}
			}

			if t.t.Alive() {

				attemptDuration := time.Now().Sub(lastAttempt)

				if attemptDuration > maxTimeout {
					t.log.Infof("Attempt lasted %v, which is longer than max timeout %s, resetting timeout to %s", attemptDuration, maxTimeout, minTimeout)
					currentTimeout = minTimeout
				} else {
					currentTimeout = currentTimeout * 2
					if currentTimeout > maxTimeout {
						currentTimeout = maxTimeout
					}
					t.log.Infof("Doubling timeout to %v", currentTimeout)
				}
			}

			select {
			case <-t.t.Dying():
				t.log.Infof("Task stopped as requested.")
				t.updateState(func(state *PortForwardState) {
					state.State = "Stopped"
					state.Error = ""
				})
				return nil
			case <-time.After(currentTimeout):
			}
		}

		t.updateState(func(state *PortForwardState) {
			state.State = "Stopped"
			state.Error = ""
		})
		return nil
	})

	// Keep parent tomb alive until our tomb is dead.
	t.daemon.t.Go(func() error {
		<-t.t.Dead()
		t.log.Infof("Task goroutine is dead.")
		return nil
	})
}

func (t *portForwardTask) Stop() {

	if t.t != nil {
		t.t.Kill(nil)
		<-t.t.Dead()
	}

	t.t = nil
}

func pipeToLog(prefix string, log *logrus.Entry, reader io.Reader) {

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		log.Info(prefix + scanner.Text())
	}
}


