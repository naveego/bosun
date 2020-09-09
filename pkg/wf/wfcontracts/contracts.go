package wfcontracts

import (
	"github.com/naveego/bosun/pkg/environment"
	"github.com/naveego/bosun/pkg/ioc"
	"github.com/naveego/bosun/pkg/values"
	"github.com/sirupsen/logrus"
)

type CommandArgumentTemplate struct {
	Name string
	Description string
	Options []string
}

type CommandTemplate struct {
	Name string
	Description string
	Arguments []CommandArgumentTemplate
}

type CommandArgument struct{
	Name string
	Value string
}

type Command struct {
	Name string
	Arguments []CommandArgument
}

type Services struct {
	Log *logrus.Entry
	Environment *environment.Environment
}

type State struct {
	Name string
	Values values.Values
}

type Config struct {
	Name string
	Type string
	Values values.Values
}

type StartParameters struct {
	Services Services
	Provider ioc.Provider
	Config values.Values
	State values.Values
}

type Event struct {
	// UpdatedState is the latest state for the workflow, which should be persisted if not nil.
	UpdatedState values.Values
	// Commands contains the commands which the user should be prompted with, if not empty.
	Commands []CommandTemplate
	// Error contains the error which the workflow has experienced, if not nil.
	Error error
}

type Workflow interface {
 	Name() string
 	Start(parameters StartParameters) (<-chan Event, error)
	Commands() []CommandTemplate
 	Execute(command Command) error
 }

