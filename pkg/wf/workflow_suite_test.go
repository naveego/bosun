package wf_test

import (
	"context"
	"github.com/naveego/bosun/pkg/wf/wfcontracts"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestWorkflow(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Suite")
}

type MockWorkflow struct {
	n string
	events chan wfcontracts.Event

}

func (m *MockWorkflow) Name() string {
	return m.n
}

func (m *MockWorkflow) Start(ctx context.Context, parameters wfcontracts.StartParameters) (<-chan wfcontracts.Event, error) {

	m.events = make(chan wfcontracts.Event)
	return m.events, nil
}

func (m *MockWorkflow) Commands() []wfcontracts.CommandTemplate {
	panic("implement me")
}

func (m *MockWorkflow) Execute(command wfcontracts.Command) error {
	panic("implement me")
}

func (m *MockWorkflow) Config() wfcontracts.Config {
	panic("implement me")
}
