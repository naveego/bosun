package bosun

import (
	"runtime"
	"strings"
)

type CommandValue struct {
	Value    string `yaml:"value"`
	Command `yaml:"-"`
	OS       map[string]*CommandValue `yaml:"os,omitempty"`

	resolved bool
}

type commandValueMarshalling CommandValue

func (c *CommandValue) MarshalYAML() (interface{}, error) {

	if c == nil {
		return nil, nil
	}

	if len(c.Value) > 0 {
		return c.Value, nil
	}

	if len(c.OS) > 0 {
		return c.OS, nil
	}

	child := &c.Command

	return child, nil
}

func (c *CommandValue) UnmarshalYAML(unmarshal func(interface{}) error) error {

	var s string
	var cmd Command
	var u commandValueMarshalling
	var err error

	if err = unmarshal(&s) ; err == nil {
		if isMultiline(s) {
			u.Command.Script = s
		} else {
			u.Value = s
		}
	} else if err = unmarshal(&u); err == nil && (len(u.OS) > 0 || u.Value != ""){

	} else if err = unmarshal(&cmd); err == nil {
		u.Command = cmd
	}

	x := CommandValue(u)
	*c = x

	return err
}

func isMultiline(s string) bool {
	return strings.Contains(s, "\n")
}

func (c *CommandValue) GetValue() string {
	if c == nil {
		return ""
	}
	if !c.resolved {
		panic("value accessed before Resolve called")
	}
	return c.Value
}

func (c *CommandValue) String() string {
	if specific, ok := c.OS[runtime.GOOS]; ok {
		return specific.String()
	} else if len(c.Value) > 0 {
		return c.Value
	}
	return c.Command.String()
}

// Resolve sets the Value field by executing Script, Command, or an entry under OS.
// If resolve has been called before, the value from that resolve is returned.
func (c *CommandValue) Resolve(ctx BosunContext) (string, error) {
	var err error

	if c.resolved {
		return c.Value, nil
	}

	c.resolved = true

	if c.Value == "" {
		c.Value, err = c.Command.Execute(ctx, CommandOpts{IgnoreDryRun:true})
	}

	// trim whitespace, as script output may contain line breaks at the end
	c.Value = strings.TrimSpace(c.Value)

	return c.Value, err
}

type DynamicValueOpts struct {
	DiscardValue bool
	StreamOutput bool
}
