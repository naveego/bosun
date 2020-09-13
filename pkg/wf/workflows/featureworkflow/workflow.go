package featureworkflow

import (
	"context"
	"github.com/naveego/bosun/pkg/wf/wfcontracts"
	"github.com/naveego/bosun/pkg/wf/wfregistry"
)

const Type = "Feature"

type StateNames struct {
	Initializing string
}

var States = StateNames{
	Initializing: "initializing",
}

func init() {
	wfregistry.DefaultRegistry.Register(Type, New)
}

type FeatureWorkflow struct {
	*wfcontracts.WorkflowHelper
}

func New() wfcontracts.Workflow {
	helper := wfcontracts.NewHelper(Type, States.Initializing)
	return &FeatureWorkflow{helper}
}

func (f FeatureWorkflow) Start(ctx context.Context, parameters wfcontracts.StartParameters) (<-chan wfcontracts.Event, error) {
	panic("implement me")
}

func (f FeatureWorkflow) Commands() []wfcontracts.CommandTemplate {
	panic("implement me")
}

func (f FeatureWorkflow) Execute(command wfcontracts.Command) error {
	panic("implement me")
}

func (f FeatureWorkflow) Config() wfcontracts.Config {
	panic("implement me")
}
