package bosun

import (
	"github.com/naveego/bosun/pkg"
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

type DynamicValueContext struct {
	Dir string
}

func NewDynamicValueContext(dir string) DynamicValueContext {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return DynamicValueContext{
		Dir: dir,
	}
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
func (d *DynamicValue) Resolve(ctx DynamicValueContext) (string, error) {
	var err error

	if d.resolved {
		return d.Value, nil
	}

	d.resolved = true

	if d.Value == "" {
		if specific, ok := d.OS[runtime.GOOS]; ok {
			d.Value, err = specific.Resolve(ctx)
		} else if len(d.Command) != 0 {
			d.Value, err = pkg.NewCommand(d.Command[0], d.Command[1:]...).WithDir(ctx.Dir).RunOut()
		} else if len(d.Script) > 0 {
			d.Value, err = executeScript(d.Script)
		}
	}

	return d.Value, err
}

// Execute executes the DynamicValue, and treats the Value field as a script.
func (d *DynamicValue) Execute(ctx DynamicValueContext) (string, error) {
	var err error
	var value string

	if specific, ok := d.OS[runtime.GOOS]; ok {
		value, err = specific.Execute(ctx)
	} else if len(d.Command) != 0 {
		value, err = pkg.NewCommand(d.Command[0], d.Command[1:]...).WithDir(ctx.Dir).RunOut()
	} else if len(d.Script) > 0 {
		value, err = executeScript(d.Script)
	} else if len(d.Value) > 0 {
		value, err = executeScript(d.Value)
	}

	return value, err
}
