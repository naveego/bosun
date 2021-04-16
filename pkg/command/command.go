package command

import (
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/templating"
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
)

type Command struct {
	Command []string            `yaml:"command,omitempty,flow" json:"command,omitempty,flow"`
	Script  string              `yaml:"script,omitempty" json:"script,omitempty"`
	OS      map[string]*Command `yaml:"os,omitempty" json:"os,omitempty"`
	// List of tools required for this command to succeed.
	Tools    []string `yaml:"tools,omitempty" json:"tools,omitempty"`
	resolved bool
}

type commandMarshalling Command

func (d Command) MarshalYAML() (interface{}, error) {

	if len(d.Command) > 0 {
		return d.Command, nil
	}

	if len(d.Script) > 0 {
		return d.Script, nil
	}

	if len(d.OS) > 0 {
		return d.OS, nil
	}

	return nil, nil
}

func (d *Command) UnmarshalYAML(unmarshal func(interface{}) error) error {

	var s string
	var c []string
	var u commandMarshalling

	err := unmarshal(&s)
	if err == nil {
		u.Script = s
		goto Convert
	}

	err = unmarshal(&c)
	if err == nil {
		u.Command = c
		goto Convert
	}

	err = unmarshal(&u)

Convert:
	x := Command(u)
	*d = x

	return err
}

type ExecutionContext interface {
	cli.ParametersGetter
	cli.WithPwder
	cli.EnvironmentVariableGetter
	templating.TemplateValuer
	util.WithLogFielder
	core.Ctxer
}

func (d *Command) String() string {
	if specific, ok := d.OS[runtime.GOOS]; ok {
		return specific.String()
	} else if len(d.Command) != 0 {
		return strings.Join(d.Command, " ")
	} else if len(d.Script) > 0 {
		return d.Script
	}
	return ""
}

type CommandOpts struct {
	// If true, echo output to stdout while running.
	StreamOutput bool
	// If true, execute even if --dry-run was passed.
	IgnoreDryRun bool
}

// Execute executes the ShellExe, and treats the Value field as a script.
func (d *Command) Execute(ctx ExecutionContext, opts ...CommandOpts) (string, error) {
	var opt CommandOpts
	if len(opts) == 0 {
		opt = CommandOpts{}
	} else {
		opt = opts[0]
	}
	return d.executeCore(ctx, opt)
}

func (d *Command) executeCore(ctx ExecutionContext, opt CommandOpts) (string, error) {

	if d == nil {
		return "", errors.New("command was nil")
	}

	var err error
	var value string

	if ctx.GetParameters().DryRun && !opt.IgnoreDryRun {
		// don't execute side-effect-only commands during dry run
		ctx.Log().WithField("command", d.String()).Info("Skipping side-effecting command because this is a dry run.")
		return "", nil
	}

	doneCh := make(chan struct{})

	go func() {
		defer func() {
			r := recover()
			if r != nil {
				if e, ok  := r.(error); ok {
					err = e
				} else {
					err = errors.Errorf("error while executing script: %v; command was\n:%v", r, d)
				}
			}
			close(doneCh)
		}()
		if specific, ok := d.OS[runtime.GOOS]; ok {
			value, err = specific.executeCore(ctx, opt)
		} else if len(d.Command) != 0 {
			cmd := NewShellExe(d.Command[0], d.Command[1:]...).WithDir(ctx.Pwd()).IncludeEnv(ctx.GetEnvironmentVariables()).WithContext(ctx.Ctx())
			if opt.StreamOutput {
				value, err = cmd.RunOutLog()
			} else {
				value, err = cmd.RunOut()
			}
		} else if len(d.Script) > 0 {
			value, err = executeScript(d.Script, ctx, opt)
		}

	}()

	select {
	case <-doneCh:
	case <-ctx.Ctx().Done():
		return "", errors.New("timed out")
	}

	return value, errors.WithStack(err)
}

func executeScript(script string, ctx ExecutionContext, opt CommandOpts) (string, error) {
	pattern := "bosun-script*"
	if runtime.GOOS == "windows" {
		pattern = "bosun-script*.bat"
	}
	tmp, err := ioutil.TempFile(os.TempDir(), pattern)
	if err != nil {
		return "", err
	}
	tmp.Close()
	ioutil.WriteFile(tmp.Name(), []byte(script), 0700)

	defer os.Remove(tmp.Name())

	vars := ctx.GetEnvironmentVariables()

	dir := ctx.Pwd()

	ctx.Log().Debugf("Running script in %s:\n%s\n", dir, script)

	cmd := GetCommandForScript(tmp.Name()).
		WithDir(dir).
		IncludeEnv(vars).
		WithContext(ctx.Ctx())

	var output string
	if opt.StreamOutput {
		output, err = cmd.RunOutLog()
	} else {
		output, err = cmd.RunOut()
	}

	if err != nil {
		return "", err
	}

	return output, nil
}

