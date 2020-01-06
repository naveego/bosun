package environment

import (
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/workspace"
	"github.com/pkg/errors"
	"os"
)

type Variable struct {
	FromPath         string                `yaml:"fromPath,omitempty" json:"fromPath,omitempty"`
	Name             string                `yaml:"name" json:"name"`
	WorkspaceCommand string                `yaml:"workspaceCommand,omitempty"`
	From             *command.CommandValue `yaml:"from" json:"from"`
	Value            string                `yaml:"-" json:"-"`
}

type EnsureContext interface {
	command.ExecutionContext
	workspace.Contexter
	GetWorkspaceCommand(name string) *command.CommandValue
}

// Ensure sets Value using the From CommandValue.
func (e *Variable) Ensure(ctx EnsureContext) error {

	if e.WorkspaceCommand != "" {
		e.From = ctx.GetWorkspaceCommand(e.WorkspaceCommand)
	}

	ctx = ctx.WithPwd(e.FromPath).(EnsureContext)
	log := ctx.Log().WithField("name", e.Name).WithField("fromPath", e.FromPath)

	if e.From == nil {
		log.Warn("`from` was not set")
		return nil
	}

	if e.Value == "" {
		log.Debug("Resolving value...")

		var err error
		e.Value, err = e.From.Resolve(ctx)

		if err != nil {
			return errors.Errorf("error populating variable %q: %s", e.Name, err)
		}

		log.WithField("value", e.Value).Debug("Resolved value.")
	}

	// set the value in the process environment
	return os.Setenv(e.Name, e.Value)
}
