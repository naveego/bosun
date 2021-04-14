package script

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/actions"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type ScriptContext interface {
	actions.ActionContext
	GetReleaseValues() *values.PersistableValues
}

type Script struct {
	core.ConfigShared `yaml:",inline"`
	Steps             []ScriptStep     `yaml:"steps,omitempty" json:"steps,omitempty"`
	Literal           *command.Command `yaml:"literal,omitempty" json:"literal,omitempty"`
	BranchFilter      string           `yaml:"branchFilter,omitempty" json:"branchFilter,omitempty"`
	Params            []ScriptParam    `yaml:"params,omitempty" json:"params,omitempty"`
}

type ScriptParam struct {
	Name         string      `yaml:"name,omitempty" json:"name,omitempty"`
	Type         string      `yaml:"type,omitempty" json:"type,omitempty"`
	Description  string      `yaml:"description,omitempty" json:"description,omitempty"`
	DefaultValue interface{} `yaml:"defaultValue,omitempty"`
}

func (s *Script) SetFromPath(path string) {
	if s == nil {
		return
	}
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
	core.ConfigShared `yaml:",inline"`
	// Bosun is a list of arguments to pass to a child instance of bosun, which
	// will be run in the directory containing this script.
	Bosun []string `yaml:"bosun,flow,omitempty" json:"bosun,omitempty"`
	// Cmd is a standard shell command.
	Cmd *command.Command `yaml:"cmd,omitempty" json:"cmd,omitempty"`
	// Action is an action to execute in the current context.
	Action *actions.AppAction `yaml:"action,omitempty" json:"action,omitempty"`
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
	Literal     *command.CommandValue  `yaml:"literal,omitempty" json:"literal,omitempty"`
}

func (s *Script) Execute(ctx ScriptContext, steps ...int) error {
	var err error

	ctx = ctx.WithPwd(s.FromPath).(ScriptContext)

	if s.BranchFilter != "" {
		branchRE, regexErr := regexp.Compile(s.BranchFilter)
		if regexErr != nil {
			return errors.Wrapf(regexErr, "invalid branchFilter %q", s.BranchFilter)
		}
		g, regexErr := git.NewGitWrapper(s.FromPath)
		if regexErr != nil {
			return errors.Wrapf(regexErr, "could not get git wrapper for branch filter %q using path %q", s.BranchFilter, s.FromPath)
		}
		branch := g.Branch()
		if !branchRE.MatchString(branch) {
			if ctx.GetParameters().Force {
				ctx.Log().Warnf("Current branch %q does not match branchFilter %q, but overridden by --force.", branch, s.BranchFilter)
			} else {
				ctx.Log().Errorf("Current branch %q does not match branchFilter %q (override using --force).", branch, s.BranchFilter)
				return nil
			}
		}
	}

	if len(s.Params) > 0 {
		releaseValues := ctx.GetReleaseValues()
		if releaseValues == nil {
			return errors.New("script has params but no release values provided")
		}

		for _, param := range s.Params {
			_, ok := releaseValues.Values[param.Name]
			if !ok {
				if param.DefaultValue == nil {
					return errors.Errorf("script param %q does not have a value set", param.Name)
				}
				releaseValues.Values[param.Name] = param.DefaultValue
			}
		}
	}

	if s.Literal != nil {
		ctx.Log().Debug("Executing literal script, not bosun script.")
		scriptCtx := ctx.WithPwd(filepath.Dir(s.FromPath)).(ScriptContext)
		_, err = s.Literal.Execute(scriptCtx, command.CommandOpts{StreamOutput: true})
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

		stepCtx := ctx.WithLogField("step", i).(ScriptContext)
		err = step.Execute(stepCtx, i)
		if err != nil {
			return errors.Wrapf(err, "script %q abended on step %q (%d)", s.Name, s.Name, i)
		}
	}

	return nil
}

func (s ScriptStep) Execute(ctx ScriptContext, index int) error {

	log := ctx.Log()
	if s.Name != "" {
		log = log.WithField("name", s.Name)
	}
	if s.Description != "" {
		log.Info(s.Description)
	}

	if s.Cmd != nil {
		log.Debug("Step is a shell command, not a bosun command.")
		cmdCtx := ctx.WithPwd(filepath.Dir(s.FromPath)).(command.ExecutionContext)

		_, err := s.Cmd.Execute(cmdCtx, command.CommandOpts{StreamOutput: true})
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
	if ctx.GetParameters().Verbose {
		stepArgs = append(stepArgs, "--verbose")
	}
	if ctx.GetParameters().DryRun {
		stepArgs = append(stepArgs, "--dry-run")
	}

	stepArgs = append(stepArgs, "--cluster", ctx.GetStringValue(core.KeyCluster))

	log.WithField("args", stepArgs).Info("Executing step")

	err = pkg.NewShellExe(exe, stepArgs...).WithDir(ctx.Pwd()).RunE()
	if err != nil {
		log.WithError(err).WithField("args", stepArgs).Error("Step failed.")
		return err
	}

	return nil
}
