package portforward

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/gofrs/flock"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/x-cray/logrus-prefixed-formatter"
	"gopkg.in/natefinch/lumberjack.v2"
	"gopkg.in/tomb.v2"
	"reflect"
	"sync"

	"github.com/rifflock/lfshook"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

const (
	lockFileName   = "bosun.lock"
	configFileName = "config.yaml"
	errorFileName  = "err.log"
	stateFileName  = "state.yaml"
)

func NewDaemon(dir string) (*PortForwardDaemon, error) {

	err := os.MkdirAll(dir, 0700)
	if err != nil {
		return nil, err
	}

	fileLock := flock.New(filepath.Join(dir, lockFileName))
	locked, err := fileLock.TryLock()
	if err != nil {
		return nil, errors.Wrap(err, "error checking file lock")
	}

	if !locked {
		return nil, errors.Errorf("port-forward daemon appears to already be running for dir %s", dir)
	}

	path := filepath.Join(dir, fmt.Sprintf("daemon.log"))
	writer := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    1,
		MaxBackups: 2,
	}

	logger := logrus.New()

	logger.AddHook(lfshook.NewHook(
		lfshook.WriterMap{
			logrus.DebugLevel: writer,
			logrus.InfoLevel:  writer,
			logrus.WarnLevel:  writer,
			logrus.ErrorLevel: writer,
			logrus.PanicLevel: writer,
		},
		&prefixed.TextFormatter{
			TimestampFormat:  time.RFC3339Nano,
			FullTimestamp:    true,
			DisableUppercase: true,
			ForceFormatting:  true,
		},
	))

	return &PortForwardDaemon{
		dir:        dir,
		children:   map[string]*portForwardTask{},
		log:        logrus.NewEntry(logger),
		configPath: filepath.Join(dir, configFileName),
		fileLock:   fileLock,
	}, nil
}

type PortForwardDaemon struct {
	dir string

	state      DaemonState
	children   map[string]*portForwardTask
	log        *logrus.Entry
	configPath string
	config     DaemonConfig
	t          *tomb.Tomb
	fileLock   *flock.Flock
	mu         sync.Mutex
}

func (p *PortForwardDaemon) Start() error {

	p.t = &tomb.Tomb{}

	p.log.Infof("Using config at %s", p.configPath)

	if _, err := os.Stat(p.configPath); os.IsNotExist(err) {
		if err = ioutil.WriteFile(p.configPath, []byte{}, 0600); err != nil {
			return errors.Wrap(err, "could not create config file")
		}
	}

	p.setErrorState(nil)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(err, "create watcher")
	}

	p.reloadConfig()

	p.t.Go(func() (err error) {

		defer func() { err = p.recordPanic() }()

		for {
			select {
			case <-p.t.Dying():
				return nil
			case _, ok := <-watcher.Events:
				if !ok {
					return nil
				}
				p.log.Info("Got config file event.")
				p.reloadConfig()
			case watcherErr, ok := <-watcher.Errors:
				if !ok {
					return nil
				}
				p.setErrorState(watcherErr)
			}
		}
	})

	err = watcher.Add(p.configPath)
	return errors.Wrap(err, "add config path to watcher")

}

func (p *PortForwardDaemon) recordPanic() (err error) {
	if r := recover(); r != nil {
		err = errors.New(fmt.Sprint(r))
		p.updateState(func(state *DaemonState) {
			state.OK = false
			state.Error = err.Error()
		})
		p.log.WithError(err).Error("Panic detected, will probably shut down.")
	}

	return
}

func (p *PortForwardDaemon) updateState(mutator func(state *DaemonState)) {

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state.Ports == nil {
		p.state.Ports = map[string]PortForwardState{}
	}

	mutator(&p.state)
	if err := yaml.SaveYaml(filepath.Join(p.dir, stateFileName), p.state); err != nil {
		p.log.WithError(err).Error("Could not save state.")
	}
}

func (p *PortForwardDaemon) updatePFState(name string, mutator func(state *PortForwardState)) {
	p.updateState(func(state *DaemonState) {
		pfstate := state.Ports[name]
		mutator(&pfstate)
		state.Ports[name] = pfstate
	})
}

func (p *PortForwardDaemon) setErrorState(err error) {
	if err == nil {
		p.updateState(func(state *DaemonState) {
			state.OK = true
			state.Error = ""
		})

	} else {
		p.log.WithError(err).Error("Error occurred.")
		p.updateState(func(state *DaemonState) {
			state.OK = false
			state.Error = err.Error()

		})
	}
}

func (p *PortForwardDaemon) reloadConfig() {
	p.log.Info("Loading config file...")

	actual := p.config

	var desired DaemonConfig

	err := yaml.LoadYaml(p.configPath, &desired)
	if err != nil {
		p.setErrorState(err)
		return
	}

	if reflect.DeepEqual(desired, actual) {
		p.log.Info("No change detected in config.")
		return
	}

	p.log.Info("Config appears to have changed, reconciling...")

	for name, desiredPortConfig := range desired.Ports {
		av := p.config.Ports[name]
		if err = p.reconcilePortForward(name, av, desiredPortConfig); err != nil {
			p.updatePFState(name, func(state *PortForwardState) {
				state.Error = err.Error()
				state.Config = desiredPortConfig
			})
		} else {
			p.updatePFState(name, func(state *PortForwardState) {
				state.Config = desiredPortConfig
			})
		}
	}
	// Reconcile ports which were desired but aren't any more
	for name, actualPortConfig := range actual.Ports {
		if _, ok := desired.Ports[name]; !ok {
			if err = p.reconcilePortForward(name, actualPortConfig, nil); err != nil {
				p.updatePFState(name, func(state *PortForwardState) {
					state.Error = err.Error()
					state.Config = actualPortConfig
				})
			} else {
				p.updatePFState(name, func(state *PortForwardState) {
					state.Config = actualPortConfig
				})
			}
		}
	}

	p.log.Info("Config changes reconciled.")
	p.config = desired
}

func (p *PortForwardDaemon) reconcilePortForward(name string, actual *PortForwardConfig, desired *PortForwardConfig) error {

	p.log.Infof("Reconciling port forward config %s", name)

	if reflect.DeepEqual(actual, desired) {
		p.log.Infof("Port forward %q unchanged.", name)
		return nil
	}

	var err error
	task, taskExists := p.children[name]

	if taskExists {
		p.log.Info("Removing port forward %s", name)

		task.Stop()

		delete(p.children, name)
		p.updateState(func(state *DaemonState) {
			delete(state.Ports, name)
		})
		taskExists = false
	}

	if desired.Active {
		p.log.Infof("Activating port forward %s", name)
		task, err = newPortForward(p, name, *desired)
		p.children[name] = task
		if err != nil {
			return err
		}
		task.Start()
	} else {
		p.log.Infof("Port forward %s is not desired to be active", name)
	}

	return nil
}

func (p *PortForwardDaemon) Stop() error {

	if p.t != nil {
		p.t.Kill(nil)
	}

	_ = p.fileLock.Unlock()

	<-p.t.Dead()

	return p.t.Err()
}
