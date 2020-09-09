package wf_test

import (
	"github.com/naveego/bosun/pkg/ioc"
	"testing"


	. "github.com/naveego/bosun/pkg/wf/wfcontracts"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestWorkflow(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Suite")
}

type MockWorkflow struct {

}

func (t MockWorkflow) Name() string {
	panic("implement me")
}

func (t MockWorkflow) Init(services Services, container ioc.Container, config []byte) error {
	panic("implement me")
}

func (t MockWorkflow) LoadState(state []byte) error {
	panic("implement me")
}

func (t MockWorkflow) SaveState() ([]byte, error) {
	panic("implement me")
}

func (t MockWorkflow) Commands() []CommandTemplate {
	panic("implement me")
}

func (t MockWorkflow) Execute(command Command) error {
	panic("implement me")
}

func (t MockWorkflow) Update() error {
	panic("implement me")
}