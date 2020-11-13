package portforward

import (
	"github.com/gofrs/flock"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
)

type Controller struct {
	dir        string
	configPath string
	statePath  string
}

func NewController(dir string) (*Controller, error) {

	err := os.MkdirAll(dir, 0700)
	if err != nil {
		return nil, err
	}

	fileLock := flock.New(filepath.Join(dir, lockFileName))

	locked, err := fileLock.TryLock()
	if err != nil {
		return nil, errors.Wrap(err, "error checking file lock")
	}

	if locked {
		_ = fileLock.Unlock()
		return nil, errors.Errorf("port-forward daemon does not seem to be running, you can start it with `bosun kube port-forward daemon %s`", dir)
	}

	return &Controller{
		dir:        dir,
		configPath: filepath.Join(dir, configFileName),
		statePath:  filepath.Join(dir, stateFileName),
	}, nil
}

func (c *Controller) GetState() (DaemonState, error) {
	var state DaemonState

	err := yaml.LoadYaml(c.statePath, &state)

	return state, err
}

func (c *Controller) AddPortForward(name string, portForwardConfig PortForwardConfig) error {
	return c.updateConfig(func(config *DaemonConfig) error{
		config.Ports[name] = &portForwardConfig
		return nil
	})
}

func (c *Controller) RemovePortForward(name string) error {
	return c.updateConfig(func(config *DaemonConfig) error {
		delete(config.Ports, name)
		return nil
	})
}

func (c *Controller) StartPortForward(name string) error {
	return c.updateConfig(func(config *DaemonConfig) error {
		if portForwardConfig, ok := config.Ports[name]; ok {
			portForwardConfig.Active = true
			return nil
		}
		return errors.Errorf("no port-forward named %s", name)
	})
}

func (c *Controller) StopPortForward(name string) error {
	return c.updateConfig(func(config *DaemonConfig) error {
		if portForwardConfig, ok := config.Ports[name]; ok {
			portForwardConfig.Active = false
			return nil
		}
		return errors.Errorf("no port-forward named %s", name)
	})
}

func (c *Controller) updateConfig(mutator func(config *DaemonConfig) error) error {
	var daemonConfig DaemonConfig

	err := yaml.LoadYaml(c.configPath, &daemonConfig)
	if err != nil {
		return err
	}

	if daemonConfig.Ports == nil {
		daemonConfig.Ports = map[string]*PortForwardConfig{}
	}

	err = mutator(&daemonConfig)
	if err != nil {
		return err
	}

	return yaml.SaveYaml(c.configPath, daemonConfig)
}

func (c *Controller) GetConfig() (DaemonConfig, error){
	var daemonConfig DaemonConfig

	err := yaml.LoadYaml(c.configPath, &daemonConfig)
	if err != nil {
		return daemonConfig, err
	}
	return daemonConfig, nil
}

func (c *Controller) GetPortForwardConfig(name string) (*PortForwardConfig, error) {
	var daemonConfig DaemonConfig

	err := yaml.LoadYaml(c.configPath, &daemonConfig)
	if err != nil {
		return nil, err
	}

	if pfc, ok := daemonConfig.Ports[name]; ok {
		return pfc, nil
	}

	return nil, errors.Errorf("no port-forward found with name %q", name)
}
