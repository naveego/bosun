package wfcontracts

import (
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/wf/statecraft"
)

type WorkflowHelper struct {
	*statecraft.Machine
	name             string
	typ              string
	exampleConfig    values.Values
	events           chan Event
	commandSets      map[string][]CommandTemplate
	activeCommandSet string
	initialState     string
	initialPayload   values.Values
}

func NewHelper(typ string, initialState string) *WorkflowHelper {
	return &WorkflowHelper{
		Machine:        statecraft.NewMachine(initialState),
		typ:            typ,
		events:         make(chan Event),
		commandSets:    map[string][]CommandTemplate{},
		initialState:   initialState,
		initialPayload: values.Values{},
		exampleConfig:  values.Values{},
	}
}

func (w *WorkflowHelper) WithExampleConfig(exampleConfig values.Values) *WorkflowHelper {
	w.exampleConfig = exampleConfig
	return w
}

func (w *WorkflowHelper) WithInitialPayload(initialPayload values.Values) *WorkflowHelper {
	w.initialPayload = initialPayload
	return w
}

func (w *WorkflowHelper) WithStateCommands(state string, commands []CommandTemplate) *WorkflowHelper {
	w.commandSets[state] = commands
	return w
}

func (w *WorkflowHelper) Commands() []CommandTemplate {
	if w.activeCommandSet != "" {
		return w.commandSets[w.activeCommandSet]
	}
	return nil
}

func (w *WorkflowHelper) Templates() (Config, State) {
	return Config{
			Name:   "",
			Type:   w.typ,
			Values: w.exampleConfig.Clone(),
		}, State{
			Name:    "",
			Current: w.initialState,
			Values:  w.initialPayload,
		}
}
