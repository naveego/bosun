package wf_test

import (
	"github.com/naveego/bosun/pkg/ioc"
	"github.com/naveego/bosun/pkg/wf/wfregistry"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	. "github.com/naveego/bosun/pkg/wf"
	. "github.com/naveego/bosun/pkg/wf/wfcontracts"

)

type MockStore struct {
	configs []Config
	states map[string]State
}

func (t *MockStore) LoadState(name string) (State, error) {
	if state, ok := t.states[name]; ok {
		return state, nil
	}

	return State{}, errors.Errorf("no state named %q", name)
}

func (t *MockStore) SaveState(state State) error {
	if t.states == nil {
		t.states = map[string]State{}
	}
	t.states[state.Name] = state
	return nil
}

func (t *MockStore) LoadConfigs() ([]Config, error) {
	return t.configs, nil
}

func (t *MockStore) SaveConfigs(configs []Config) error {
	t.configs = configs
	return nil
}

var _ = Describe("Engine", func() {


	var registry *wfregistry.Registry
	var workflow *MockWorkflow
	var container *ioc.Container
	var store *MockStore
	var engine *Engine

	BeforeEach(func() {
		registry = wfregistry.New()
		workflow = &MockWorkflow{}
		container = ioc.NewContainer()
		store = &MockStore{}
		engine = &Engine{
			Registry:  registry,
			Provider: container,
			Log:       logrus.NewEntry(logrus.StandardLogger()),
			ConfigStore: store,
			StateStore: store,
		}
	})

	It("should load and initialize workflow", func() {

		Expect(engine.Run("mock")).To(Succeed())


	})


})
