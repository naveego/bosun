package wfengine_test

import (
	"github.com/naveego/bosun/pkg/environment"
	"github.com/naveego/bosun/pkg/ioc"
	"github.com/naveego/bosun/pkg/wf/wfregistry"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	. "github.com/naveego/bosun/pkg/wf"
	. "github.com/naveego/bosun/pkg/wf/wfcontracts"
)



var _ = Describe("Engine", func() {

	const MockWorkflowType = "mock-workflow"
	const MockWorkflowName = "mock-instance"

	var env *environment.Environment
	var registry *wfregistry.Registry
	var workflow *MockWorkflow
	var container *ioc.Container
	var store *MockStore
	var engine *Engine

	BeforeEach(func() {
		env, _ = environment.New(environment.Config{}, environment.Options{})
		registry = wfregistry.New()
		workflow = &MockWorkflow{}
		registry.Register(MockWorkflowType, func() Workflow {
			return workflow
		})
		container = ioc.NewContainer()
		store = &MockStore{
			configs: map[string]*Config{},
			states:  map[string]*State{},
		}
		engineConfig := EngineConfig{
			Environment: env,
			Registry:    registry,
			Provider:    container,
			Log:         logrus.NewEntry(logrus.StandardLogger()),
			ConfigStore: store,
			StateStore:  store,
		}
		var err error
		engine, err = NewEngine(engineConfig)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should be able to run and stop engine", func() {

		config := Config{
			Name: MockWorkflowName,
			Type: MockWorkflowType,
		}
		Expect(engine.Configure(config)).To(Succeed())

		Expect(engine.Run(MockWorkflowName)).To(Succeed())

		engine.Stop(nil)

		Eventually(engine.Done()).Should(BeClosed())
	})
})
