package bosun

import (
	"io/ioutil"
	"os"
	"runtime"
	"strings"

	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
)

type DynamicValue struct {
	Value    string                   `yaml:"value,omitempty"`
	Command  []string                 `yaml:"command,omitempty"`
	Script   string                   `yaml:"script,omitempty"`
	OS       map[string]*DynamicValue `yaml:"os,omitempty"`
	resolved bool
}

type dynamicValueMarshalling DynamicValue

func (d *DynamicValue) MarshalYAML() (interface{}, error) {

	if d == nil {
		return nil, nil
	}

	if len(d.Value) > 0 {
		return d.Value, nil
	}

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

func (d *DynamicValue) UnmarshalYAML(unmarshal func(interface{}) error) error {

	var s string
	var c []string
	var u dynamicValueMarshalling

	err := unmarshal(&s)
	if err == nil {
		multiline := len(strings.Split(s, "\n")) > 1
		if multiline {
			u.Script = s
		} else {
			u.Value = s
		}
		goto Convert
	}

	err = unmarshal(&c)
	if err == nil {
		u.Command = c
		goto Convert
	}

	err = unmarshal(&u)

Convert:
	x := DynamicValue(u)
	*d = x

	return err
}

func (d *DynamicValue) GetValue() string {
	if d == nil {
		return ""
	}
	if !d.resolved {
		panic("value accessed before Resolve called")
	}
	return d.Value
}

func (d *DynamicValue) String() string {
	if specific, ok := d.OS[runtime.GOOS]; ok {
		return specific.String()
	} else if len(d.Command) != 0 {
		return strings.Join(d.Command, "")
	} else if len(d.Script) > 0 {
		return d.Script
	} else if len(d.Value) > 0 {
		return d.Value
	}
	return ""
}

// Resolve sets the Value field by executing Script, Command, or an entry under OS.
// If resolve has been called before, the value from that resolve is returned.
func (d *DynamicValue) Resolve(ctx BosunContext) (string, error) {
	var err error

	if d.resolved {
		return d.Value, nil
	}

	d.resolved = true

	if d.Value == "" {
		d.Value, err = d.executeCore(ctx, DynamicValueOpts{})
	}

	// trim whitespace, as script output may contain line breaks at the end
	d.Value = strings.TrimSpace(d.Value)

	return d.Value, err
}

type DynamicValueOpts struct {
	DiscardValue bool
	StreamOutput bool
}

// Execute executes the DynamicValue, and treats the Value field as a script.
func (d *DynamicValue) Execute(ctx BosunContext, opts ...DynamicValueOpts) (string, error) {
	var opt DynamicValueOpts
	if len(opts) == 0 {
		opt = DynamicValueOpts{}
	} else {
		opt = opts[0]
	}
	return d.executeCore(ctx, opt)
}

func (d *DynamicValue) executeCore(ctx BosunContext, opt DynamicValueOpts) (string, error) {

	var err error
	var value string

	if ctx.GetParams().DryRun && opt.DiscardValue {
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
		} else if len(d.Value) > 0 {
			value, err = executeScript(d.Value, ctx, opt)
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

func executeScript(script string, ctx BosunContext, opt DynamicValueOpts) (string, error) {
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

	vars, err := ctx.Env.GetVariablesAsMap(ctx)
	if err != nil {
		return "", errors.Wrap(err, "resolve environment variables for script")
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
