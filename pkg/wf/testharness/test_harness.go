package testharness

import (
	"github.com/naveego/bosun/pkg/environment"
	"github.com/naveego/bosun/pkg/ioc"
	. "github.com/naveego/bosun/pkg/wf/wfcontracts"
	. "github.com/naveego/bosun/pkg/wf/wfengine"
	"github.com/naveego/bosun/pkg/wf/wfregistry"
	"github.com/sirupsen/logrus"
)

type MockStore struct {
	configs map[string]*Config
	states  map[string]*State
}

func (t *MockStore) LoadState(name string) (*State, error) {
	if state, ok := t.states[name]; ok {
		return state, nil
	}

	return nil, nil
}

func (t *MockStore) SaveState(state State) error {
	if t.states == nil {
		t.states = map[string]*State{}
	}
	t.states[state.Name] = &state
	return nil
}

func (t *MockStore) LoadConfigs() ([]Config, error) {
	var out []Config
	for _, v := range t.configs {
		out = append(out, *v)
	}
	return out, nil
}

func (t *MockStore) LoadConfig(name string) (*Config, error) {
	if config, ok := t.configs[name]; ok {
		return config, nil
	}

	return nil, nil
}

func (t *MockStore) SaveConfig(config Config) error {
	t.configs[config.Name] = &config
	return nil
}

type TestHarness struct {
	Env       *environment.Environment
	Registry  *wfregistry.Registry
	Container *ioc.Container
	Store     *MockStore
	Engine    *Engine
}

func New() *TestHarness {

	harness := &TestHarness{}

	harness.Env, _ = environment.New(environment.Config{}, environment.Options{})
	harness.Registry = wfregistry.New()

	harness.Container = ioc.NewContainer()
	harness.Store = &MockStore{
		configs: map[string]*Config{},
		states:  map[string]*State{},
	}
	engineConfig := EngineConfig{
		Environment: harness.Env,
		Registry:    harness.Registry,
		Provider:    harness.Container,
		ConfigStore: harness.Store,
		StateStore:  harness.Store,
		Log:         logrus.NewEntry(logrus.StandardLogger()),
	}
	var err error
	harness.Engine, err = NewEngine(engineConfig)
	if err != nil {
		panic("test harness could not be created")
	}

	return harness

}
