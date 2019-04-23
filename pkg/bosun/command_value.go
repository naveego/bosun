package bosun

import (
	"runtime"
	"strings"
)

type CommandValue struct {
	Value   string `yaml:"value" json:"value"`
	Command `yaml:"-" json:"-"`
	OS      map[string]*CommandValue `yaml:"os,omitempty" json:"os,omitempty"`

	resolvedValue string
}

type commandValueMarshalling struct {
	Value   string                              `yaml:"value,omitempty" json:"value,omitempty"`
	Command []string                            `yaml:"command,omitempty,flow" json:"command,omitempty,flow"`
	Script  string                              `yaml:"script,omitempty" json:"script,omitempty"`
	OS      map[string]*commandValueMarshalling `yaml:"os,omitempty" json:"os,omitempty"`
	Tools   []string                            `yaml:"tools,omitempty" json:"tools,omitempty"`
}

func (c *CommandValue) toMarshalling() *commandValueMarshalling {
	m := commandValueMarshalling{
		Value:   c.Value,
		Command: c.Command.Command,
		Script:  c.Script,
		Tools:   c.Tools,
	}
	if len(c.OS) > 0 {
		m.OS = map[string]*commandValueMarshalling{}
		for k, v := range c.OS {
			m.OS[k] = v.toMarshalling()
		}
	}
	return &m
}

func (c commandValueMarshalling) apply(to *CommandValue) {
	to.Value = c.Value
	to.Command.Command = c.Command
	to.Script = c.Script
	to.Tools = c.Tools
	if len(c.OS) > 0 {
		to.OS = map[string]*CommandValue{}
		for k, v := range c.OS {
			o := &CommandValue{}
			v.apply(o)
			to.OS[k] = o
		}
	}
}

func (c *CommandValue) MarshalYAML() (interface{}, error) {
	if c.Value != "" && !isMultiline(c.Value) {
		return c.Value, nil
	}

	return c.toMarshalling(), nil
}

func (c *CommandValue) UnmarshalYAML(unmarshal func(interface{}) error) error {

	var s string
	var cmd []string
	var u commandValueMarshalling
	var err error

	if err = unmarshal(&s); err == nil {
		if isMultiline(s) {
			u.Script = s
		} else {
			u.Value = s
		}
	} else if err = unmarshal(&u); err == nil && (len(u.OS) > 0 || u.Value != "") {
		// direct unmarshal succeeded
	} else if err = unmarshal(&cmd); err == nil {
		u.Command = cmd
	}

	u.apply(c)

	return nil
}

func isMultiline(s string) bool {
	return strings.Index(s, "\n") >= 0
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
		return c.resolvedValue, nil
	}

	c.resolved = true

	if c.Value != "" {
		c.resolvedValue, err = renderTemplate(ctx, c.Value)
	} else {
		c.resolvedValue, err = c.Command.Execute(ctx, CommandOpts{IgnoreDryRun: true})
		// trim whitespace, as script output may contain line breaks at the end
		c.resolvedValue = strings.TrimSpace(c.resolvedValue)
	}

	return c.resolvedValue, err
}

type DynamicValueOpts struct {
	DiscardValue bool
	StreamOutput bool
}
