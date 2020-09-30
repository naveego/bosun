package wfengine

import (
	"context"
	"github.com/naveego/bosun/pkg/environment"
	"github.com/naveego/bosun/pkg/ioc"
	"github.com/naveego/bosun/pkg/wf/wfcontracts"
	"github.com/naveego/bosun/pkg/wf/wfregistry"
	"github.com/naveego/bosun/pkg/wf/wfstores"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/tomb.v2"
)

type EngineConfig struct {
	Environment *environment.Environment
	Registry    *wfregistry.Registry
	Provider    ioc.Provider
	Log         *logrus.Entry
	ConfigStore wfstores.ConfigStore
	StateStore  wfstores.StateStore
}

func NewEngine(config EngineConfig) (*Engine, error) {
	if config.Environment == nil {
		return nil, errors.Errorf("config.Environment required")
	}
	if config.Provider == nil {
		return nil, errors.Errorf("config.Provider required")
	}
	if config.ConfigStore == nil {
		return nil, errors.Errorf("config.ConfigStore required")
	}
	if config.StateStore == nil {
		return nil, errors.Errorf("config.StateStore required")
	}
	if config.Registry == nil {
		config.Registry = wfregistry.DefaultRegistry
	}
	if config.Log == nil {
		config.Log = logrus.NewEntry(logrus.StandardLogger())
	}
	config.Log = logrus.WithField("component", "wf.Engine")

	engine := &Engine{
		registry:    config.Registry,
		provider:    config.Provider,
		log:         config.Log,
		configStore: config.ConfigStore,
		stateStore:  config.StateStore,
		environment: config.Environment,
		t:           &tomb.Tomb{},
		wfs:         map[string]*activeTask{},
	}

	// Keep the engine running until actually killed or a task errors.
	engine.t.Go(func() error {
		<-engine.t.Dying()
		return nil
	})

	return engine, nil
}

type Engine struct {
	registry    *wfregistry.Registry
	provider    ioc.Provider
	log         *logrus.Entry
	configStore wfstores.ConfigStore
	stateStore  wfstores.StateStore
	t           *tomb.Tomb
	wfs         map[string]*activeTask
	environment *environment.Environment
}

func (e *Engine) Done() <-chan struct{} {
	return e.t.Dying()
}

func (e *Engine) Stop(err error) {
	if err != nil {
		e.log.Warnf("Stopping engine with error: %v", err)
	} else {
		e.log.Info("Stopping engine.")
	}
	e.t.Kill(err)
}

func (e *Engine) Configure(config wfcontracts.Config) error {
	return e.configStore.SaveConfig(config)
}

func (e *Engine) Run(name string) error {

	if _, ok := e.wfs[name]; ok {
		return errors.Errorf("workflow %q already running", name)
	}

	config, err := e.configStore.LoadConfig(name)
	if err != nil {
		return err
	}

	if config == nil {
		return errors.Errorf("config %q not found", name)
	}

	instance, err := e.registry.Create(config.Type)
	if err != nil {
		return err
	}

	state, err := e.stateStore.LoadState(name)
	if err != nil {
		return err
	}
	if state == nil {

	}

	parameters := wfcontracts.StartParameters{
		Services: wfcontracts.Services{
			Log:         nil,
			Environment: e.environment,
			Provider:    e.provider,
		},
		Config: *config,
		State:  *state,
	}

	task := &activeTask{
		latestEvent: wfcontracts.Event{},
		events:      make(chan wfcontracts.Event, 10),
	}
	task.ctx, task.cancel = context.WithCancel(e.t.Context(context.Background()))

	e.t.Go(func() error {
		taskEvents, taskErr := instance.Start(task.ctx, parameters)
		if taskErr != nil {
			task.events <- wfcontracts.Event{
				UpdatedState: nil,
				Commands:     nil,
				Error:        taskErr,
			}
			return nil
		} else {
			task.events <- wfcontracts.Event{
				Message: "Started",
			}
		}

		go func() {
			for {
				select {
				case <-task.ctx.Done():
					task.events <- wfcontracts.Event{
						Message: "Stopped",
					}
					close(task.events)
				case evt := <-taskEvents:
					task.events <- evt
					if evt.Error != nil {
						close(task.events)
					}
				}
			}
		}()

		return nil
	})

	// first event will be startup error or a "started" message
	firstEvent := <-task.events

	if firstEvent.Error != nil {
		// startup failed
		return errors.Wrap(firstEvent.Error, "startup failed")
	}

	e.t.Go(func() error {
		return e.runWorkflowLoop(task)
	})

	return nil
}

func (e *Engine) runWorkflowLoop(task *activeTask) error {

	return nil
}

type activeTask struct {
	latestEvent wfcontracts.Event
	cancel      func()
	ctx         context.Context
	events      chan wfcontracts.Event
}
