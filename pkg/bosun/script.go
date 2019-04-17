package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Script struct {
	ConfigShared `yaml:",inline"`
	File         *File        `yaml:"-" json:"-"`
	Steps        []ScriptStep `yaml:"steps,omitempty" json:"steps,omitempty"`
	Literal      *Command     `yaml:"literal,omitempty" json:"literal,omitempty"`
}

func (s *Script) SetFromPath(path string) {
	s.FromPath = path
	for i := range s.Steps {
		step := s.Steps[i]
		step.FromPath = path
		if step.Action != nil {
			step.Action.FromPath = path
		}
		s.Steps[i] = step
	}
}

type ScriptStep struct {
	ConfigShared `yaml:",inline"`
	// Bosun is a list of arguments to pass to a child instance of bosun, which
	// will be run in the directory containing this script.
	Bosun []string `yaml:"bosun,flow,omitempty" json:"bosun,omitempty"`
	// Cmd is a standard shell command.
	Cmd *Command `yaml:"cmd,omitempty" json:"cmd,omitempty"`
	// Action is an action to execute in the current context.
	Action *AppAction `yaml:"action,omitempty" json:"action,omitempty"`
}

func (s *ScriptStep) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var m map[string]interface{}
	err := unmarshal(&m)
	if err != nil {
		return err
	}

	_, hasCommand := m["command"]
	_, hasLiteral := m["literal"]
	if hasCommand || hasLiteral {
		// this is a v1 script step
		var v1 scriptStepV1
		err = unmarshal(&v1)
		if err != nil {
			return err
		}
		if s == nil {
			*s = ScriptStep{}
		}
		if hasCommand {
			s.Bosun = append([]string{v1.Command}, v1.Args...)
			for k, v := range v1.Flags {
				s.Bosun = append(s.Bosun, "--"+k, fmt.Sprint(v))
			}
		}
		if hasLiteral {
			s.Cmd = &v1.Literal.Command
		}
	} else {
		type proxy ScriptStep
		var p proxy
		err = unmarshal(&p)
		if err != nil {
			return err
		}
		*s = ScriptStep(p)
	}
	return nil
}

// Deprecated script step format.
type scriptStepV1 struct {
	Name        string                 `yaml:"name,omitempty" json:"name,omitempty"`
	Description string                 `yaml:"description,omitempty" json:"description,omitempty"`
	Command     string                 `yaml:"command" json:"command"`
	Args        []string               `yaml:"args" json:"args"`
	Flags       map[string]interface{} `yaml:"flags" json:"flags"`
	Literal     *CommandValue          `yaml:"literal,omitempty" json:"literal,omitempty"`
}

func (s *Script) Execute(ctx BosunContext, steps ...int) error {
	var err error

	ctx = ctx.WithDir(s.FromPath)
	env := ctx.Env

	if err = env.Ensure(ctx); err != nil {
		return errors.Wrap(err, "ensure environment")
	}

	if _, err = env.Render(ctx); err != nil {
		return errors.Wrap(err, "render environment")
	}

	if s.Literal != nil {
		ctx.Log.Debug("Executing literal script, not bosun script.")
		_, err := s.Literal.Execute(ctx.WithDir(filepath.Dir(s.FromPath)), CommandOpts{StreamOutput: true})
		if err != nil {
			return err
		}
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if !strings.Contains(exe, "bosun") {
		exe = "bosun" // support working under debugger
	}

	exe, err = exec.LookPath(exe)
	if err != nil {
		return err
	}

	if len(steps) == 0 {
		for i := range s.Steps {
			steps = append(steps, i)
		}
	}

	for _, i := range steps {
		if i >= len(s.Steps) {
			return errors.Errorf("invalid step %d (there are %d steps)", i, len(s.Steps))
		}
		step := s.Steps[i]

		ctx := ctx.WithLog(ctx.Log.WithField("step", i))
		err := step.Execute(ctx, i)
		if err != nil {
			return errors.Wrapf(err, "script %q abended on step %q (%d)", s.Name, s.Name, i)
		}
	}

	return nil
}

func (s ScriptStep) Execute(ctx BosunContext, index int) error {

	log := ctx.Log
	if s.Name != "" {
		log = log.WithField("name", s.Name)
	}
	if s.Description != "" {
		log.Info(s.Description)
	}

	if s.Cmd != nil {
		log.Debug("Step is a shell command, not a bosun command.")

		_, err := s.Cmd.Execute(ctx.WithDir(filepath.Dir(s.FromPath)), CommandOpts{StreamOutput: true})
		return err
	}

	if s.Action != nil {
		log.Debug("Step is an action.")
		if s.Action.Name == "" {
			s.Action.Name = s.Name
		}

		err := s.Action.Execute(ctx)
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	var stepArgs []string
	stepArgs = append(stepArgs, s.Bosun...)
	stepArgs = append(stepArgs, "--step", fmt.Sprintf("%d", index))
	if ctx.IsVerbose() {
		stepArgs = append(stepArgs, "--verbose")
	}
	if ctx.IsDryRun() {
		stepArgs = append(stepArgs, "--dry-run")
	}

	stepArgs = append(stepArgs, "--domain", ctx.GetDomain())
	stepArgs = append(stepArgs, "--cluster", ctx.GetCluster())

	log.WithField("args", stepArgs).Info("Executing step")

	err = pkg.NewCommand(exe, stepArgs...).WithDir(ctx.Dir).RunE()
	if err != nil {
		log.WithError(err).WithField("args", stepArgs).Error("Step failed.")
		return err
	}

	return nil
}
