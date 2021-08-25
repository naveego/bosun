package command

import (
	"github.com/naveego/bosun/pkg/templating"
	"gopkg.in/yaml.v3"
	"runtime"
	"strings"
)

type CommandValue struct {
	Comment              string `yaml:"comment,omitempty" json:"comment,omitempty"`
	Value                string `yaml:"value" json:"value"`
	Command              `yaml:"-" json:"-"`
	OS                   map[string]*CommandValue `yaml:"os,omitempty" json:"os,omitempty"`
	WorkspaceCommand     string                   `yaml:"workspaceCommand,omitempty"`
	WorkspaceCommandHint string                   `yaml:"workspaceCommandHint,omitempty"`
	Disabled             bool                     `yaml:"disabled"`

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

func (c *CommandValue) UnmarshalYAML(node *yaml.Node) error {

	var cmd []string
	var u commandValueMarshalling
	var err error

	switch node.Kind {
	case yaml.ScalarNode:
		if isMultiline(node.Value) {
			u.Script = node.Value
		} else {
			u.Value = node.Value
		}
	case yaml.SequenceNode:
		for _, n := range node.Content {
			cmd = append(cmd, n.Value)
		}
		u.Command = cmd
	case yaml.MappingNode:
		err = node.Decode(&u)
		if err != nil {
			return err
		}
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
func (c *CommandValue) Resolve(ctx ExecutionContext) (string, error) {
	var err error

	if c.resolved {
		return c.resolvedValue, nil
	}

	c.resolved = true

	if c.WorkspaceCommandHint != "" {

		realCommandValue := ctx.GetWorkspaceCommand(c.WorkspaceCommand, c.WorkspaceCommandHint)
		c.resolvedValue, err = realCommandValue.Resolve(ctx)
	} else {

		if c.Value != "" {
			c.resolvedValue, err = templating.RenderTemplate(c.Value, ctx.TemplateValues())
		} else {
			c.resolvedValue, err = c.Command.Execute(ctx, CommandOpts{IgnoreDryRun: true, StreamOutput: ctx.GetParameters().Verbose})
			// trim whitespace, as script output may contain line breaks at the end
			c.resolvedValue = strings.TrimSpace(c.resolvedValue)
		}
	}

	return c.resolvedValue, err
}

type DynamicValueOpts struct {
	DiscardValue bool
	StreamOutput bool
}
