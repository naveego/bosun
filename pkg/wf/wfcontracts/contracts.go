package wfcontracts

import (
	"context"
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
	Provider ioc.Provider
}

type State struct {
	Name string
	Current string
	Values values.Values
}

type Config struct {
	Name string
	Type string
	Values values.Values
}

type StartParameters struct {
	Services Services
	Config Config
	State State
}

type Event struct {
	// Message to log or display when this event happens
	Message string
	// UpdatedState is the latest state for the workflow, which should be persisted if not nil.
	UpdatedState values.Values
	// Commands contains the commands which the user should be prompted with, if not empty.
	Commands []CommandTemplate
	// Error contains the error which the workflow has experienced, if not nil.
	Error error
}

type Workflow interface {
 	Start(ctx context.Context, parameters StartParameters) (<-chan Event, error)
	Commands() []CommandTemplate
 	Execute(command Command) error
	Templates() (Config, State)
 }

