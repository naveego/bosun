package bosun

import (
	"github.com/google/go-github/github"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os/exec"
	"time"
)

type Bosun struct {
	params           Parameters
	config           *Config
	state            *State
	microservices    map[string]*App
	clusterAvailable *bool
}

type Parameters struct {
	Verbose bool
	DryRun  bool
}

func New(params Parameters, config *Config, state *State) *Bosun {

	if state == nil {
		state = new(State)
	}

	if state.Microservices == nil {
		state.Microservices = make(map[string]AppState)
	}

	b := &Bosun{
		params:        params,
		config:        config,
		state:         state,
		microservices: make(map[string]*App),
	}

	for _, m := range config.Apps {
		b.addMicroservice(m)
	}

	return b
}

func (b *Bosun) addMicroservice(config *AppConfig) *App {
	ms := &App{
		bosun:  b,
		Config: config,
	}
	b.microservices[config.Name] = ms
	ms.DesiredState = b.state.Microservices[config.Name]
	return ms
}

func (b *Bosun) GetMicroservices() []*App {
	var ms []*App

	for _, x := range b.microservices {
		ms = append(ms, x)
	}
	return ms
}

func (b *Bosun) GetScripts() ([]*Script, error) {
	env, err := b.GetCurrentEnvironment()
	if err != nil {
		return nil, err
	}

	return env.Scripts, nil
}

func (b *Bosun) GetScript(name string) (*Script, error) {
	env, err := b.GetCurrentEnvironment()
	if err != nil {
		return nil, err
	}
	for _, s := range env.Scripts {
		if s.Name == name {
			return s, nil
		}
	}

	return nil, errors.Errorf("no script in environment %q with name %q", env.Name, name)
}

func (b *Bosun) GetMicroservice(name string) (*App, error) {
	m, ok := b.microservices[name]
	if !ok {
		return nil, errors.Errorf("no service named %q", name)
	}
	return m, nil
}

func (b *Bosun) GetOrAddMicroserviceForPath(path string) (*App, error) {
	for _, m := range b.microservices {
		if m.Config.FromPath == path {
			return m, nil
		}
	}

	c, _, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}

	pkg.Log.WithField("path", path).Debug("New microservice found at path.")

	b.config.Merge(c)

	var name string
	for _, m := range c.Apps {
		b.addMicroservice(m)
		name = m.Name
	}

	err = b.SaveConfig()
	if err != nil {
		return nil, err
	}

	m, _ := b.GetMicroservice(name)
	return m, nil
}

func (b *Bosun) SetCurrentEnvironment(name string) error {

	for _, env := range b.config.Environments {
		if env.Name == name {
			b.config.CurrentEnvironment = name
			return nil
		}
	}

	return errors.Errorf("environment %q does not exist", name)
}

func (b *Bosun) GetCurrentEnvironment() (*EnvironmentConfig, error) {

	for _, env := range b.config.Environments {
		if env.Name == b.config.CurrentEnvironment {
			return env, nil
		}
	}

	return nil, errors.Errorf("current environment %q does not exist", b.config.CurrentEnvironment)
}

func (b *Bosun) Save() error {
	err := b.SaveConfig()
	if err != nil {
		return err
	}

	return b.SaveState()
}

func (b *Bosun) SaveConfig() error {

	config := b.config.Unmerge(b.config.Path)

	data, err := yaml.Marshal(config)
	if err != nil {
		return errors.Wrap(err, "marshalling for save")
	}

	err = ioutil.WriteFile(config.Path, data, 0700)
	if err != nil {
		return errors.Wrap(err, "writing for save")
	}

	return nil
}

func (b *Bosun) SaveState() error {
	statePath := getStatePath(b.config.Path)

	state := State{
		Microservices: make(map[string]AppState),
	}

	for _, m := range b.microservices {
		state.Microservices[m.Config.Name] = m.DesiredState
	}

	data, err := yaml.Marshal(state)
	if err != nil {
		return errors.Wrap(err, "marshalling state")
	}

	err = ioutil.WriteFile(statePath, data, 0700)
	if err != nil {
		return errors.Wrap(err, "writing state")
	}

	return nil
}

func (b *Bosun) GetMergedConfig() string {

	data, _ := yaml.Marshal(b.config)

	return string(data)

}

func (b *Bosun) AddImport(file string) bool {
	for _, i := range b.config.Imports {
		if i == file {
			return false
		}
	}
	b.config.Imports = append(b.config.Imports, file)
	return true
}

func (b *Bosun) IsClusterAvailable() bool {
	if b.clusterAvailable == nil {
		pkg.Log.Debug("Checking if cluster is available...")
		resultCh := make(chan bool)
		cmd := exec.Command("kubectl", "cluster-info")
		go func() {
			err := cmd.Run()
			if err != nil {
				resultCh <- false
			} else {
				resultCh <- true
			}
		}()

		select {
		case result := <-resultCh:
			b.clusterAvailable = &result
			pkg.Log.Debugf("Cluster is available: %t", result)
		case <-time.After(2 * time.Second):
			pkg.Log.Warn("Cluster did not respond quickly, I will assume it is unavailable.")
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			b.clusterAvailable = github.Bool(false)
		}
	}

	return *b.clusterAvailable
}
