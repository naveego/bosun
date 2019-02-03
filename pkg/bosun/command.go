package bosun

import (
	"io/ioutil"
	"os"
	"runtime"
	"strings"

	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
)

type Command struct {
	Command  []string                 `yaml:"command,omitempty"`
	Script   string                   `yaml:"script,omitempty"`
	OS       map[string]*Command `yaml:"os,omitempty"`
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

// Execute executes the Command, and treats the Value field as a script.
func (d *Command) Execute(ctx BosunContext, opts ...CommandOpts) (string, error) {
	var opt CommandOpts
	if len(opts) == 0 {
		opt = CommandOpts{}
	} else {
		opt = opts[0]
	}
	return d.executeCore(ctx, opt)
}

func (d *Command) executeCore(ctx BosunContext, opt CommandOpts) (string, error) {

	var err error
	var value string

	if ctx.GetParams().DryRun && !opt.IgnoreDryRun {
		// don't execute side-effect-only commands during dry run
		ctx.Log.WithField("command", d.String()).Info("Skipping side-effecting command because this is a dry run.")
		return "", nil
	}

	doneCh := make(chan struct{})

	go func() {
		if specific, ok := d.OS[runtime.GOOS]; ok {
			value, err = specific.executeCore(ctx, opt)
		} else if len(d.Command) != 0 {
			cmd := pkg.NewCommand(d.Command[0], d.Command[1:]...).WithDir(ctx.Dir).IncludeEnv(ctx.GetValuesAsEnvVars()).WithContext(ctx.Ctx())
			if opt.StreamOutput {
				value, err = cmd.RunOutLog()
			} else {
				value, err = cmd.RunOut()
			}
		} else if len(d.Script) > 0 {
			value, err = executeScript(d.Script, ctx, opt)
		}

		close(doneCh)
	}()

	select {
	case <-doneCh:
	case <-ctx.Ctx().Done():
		return "", errors.New("timed out")
	}

	return value, err
}

func executeScript(script string, ctx BosunContext, opt CommandOpts) (string, error) {
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

	var vars map[string]string
	if ctx.Env != nil {
		vars, err = ctx.Env.GetVariablesAsMap(ctx)
		if err != nil {
			return "", errors.Wrap(err, "resolve environment variables for script")
		}
	}

	ctx.Log.Debugf("Running script:\n%s\n", script)

	cmd := getCommandForScript(tmp.Name()).
		WithDir(ctx.Dir).
		IncludeEnv(ctx.GetValuesAsEnvVars()).
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
