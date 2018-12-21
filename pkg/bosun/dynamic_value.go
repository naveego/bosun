package bosun

import (
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
)

type DynamicValue struct {
	Value    string                   `yaml:"value,omitempty"`
	Command  []string                 `yaml:"command,omitempty"`
	Script   string                   `yaml:"script,omitempty"`
	OS       map[string]*DynamicValue `yaml:"os,omitempty"`
	resolved bool
}

type dynamicValueUnmarshall DynamicValue

func (d *DynamicValue) UnmarshalYAML(unmarshal func(interface{}) error) error {

	var s string
	var c []string
	var u dynamicValueUnmarshall

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

// Resolve sets the Value field by executing Script, Command, or an entry under OS.
// If resolve has been called before, the value from that resolve is returned.
func (d *DynamicValue) Resolve(ctx BosunContext) (string, error) {
	var err error

	if d.resolved {
		return d.Value, nil
	}

	d.resolved = true

	if d.Value == "" {
		d.Value, err = d.Execute(ctx)
	}

	// trim whitespace, as script output may contain line breaks at the end
	d.Value = strings.TrimSpace(d.Value)

	return d.Value, err
}

// Execute executes the DynamicValue, and treats the Value field as a script.
func (d *DynamicValue) Execute(ctx BosunContext) (string, error) {
	var err error
	var value string

	doneCh := make(chan struct{})

	go func() {
		if specific, ok := d.OS[runtime.GOOS]; ok {
			value, err = specific.Execute(ctx)
		} else if len(d.Command) != 0 {
			value, err = pkg.NewCommand(d.Command[0], d.Command[1:]...).WithDir(ctx.Dir).IncludeEnv(ctx.ValuesAsEnvVars).WithContext(ctx.Ctx()).RunOut()
		} else if len(d.Script) > 0 {
			value, err = executeScript(d.Script, ctx)
		} else if len(d.Value) > 0 {
			value, err = executeScript(d.Value, ctx)
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

func executeScript(script string, ctx BosunContext) (string, error) {
	tmp, err := ioutil.TempFile(os.TempDir(), "bosun-script")
	if err != nil {
		return "", err
	}
	tmp.Close()
	ioutil.WriteFile(tmp.Name(), []byte(script), 0700)

	//defer os.Remove(tmp.Name())

	// pkg.Log.Debugf("running script from temp file %q", tmp.Name())
	cmd := getCommandForScript(tmp.Name()).
		WithDir(ctx.Dir).
		IncludeEnv(ctx.ValuesAsEnvVars).
		WithContext(ctx.Ctx())

	o, err := cmd.RunOut()
	if err != nil {
		return "", err
	}

	return string(o), nil
}
