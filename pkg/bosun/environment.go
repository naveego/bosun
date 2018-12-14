package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type EnvironmentConfig struct {
	FromPath string `yaml:"fromPath,omitempty"`
	Name      string                 `yaml:name`
	Cluster   string                 `yaml:"cluster"`
	Domain    string                 `yaml:"domain"`
	Commands  []*EnvironmentCommand  `yaml:"commands,omitempty"`
	Variables []*EnvironmentVariable `yaml:"variables,omitempty"`
	Scripts   []*Script              `yaml:"scripts,omitempty"`
}

type EnvironmentVariable struct {
	FromPath string `yaml:"fromPath,omitempty"`
	Name    string   `yaml:"name"`
	Value   string   `yaml:"value,omitempty"`
	Command []string `yaml:"command,omitempty"`
	Script  string   `yaml:"script,omitempty"`
}

type EnvironmentCommand struct {
	FromPath string `yaml:"fromPath,omitempty"`
	Name           string   `yaml:"name"`
	Command        []string `yaml:"command,omitempty"`
	LinuxCommand   []string `yaml:"linux,omitempty"`
	DarwinCommand  []string `yaml:"darwin,omitempty"`
	WindowsCommand []string `yaml:"windows,omitempty"`
}

// Ensure sets the Value of the EnvironmentVariable
// if there are Command values.
func (e *EnvironmentVariable) Ensure() error {

	defer func() {
		os.Setenv(e.Name, e.Value)
	}()

	if e.Value != "" {
		return nil
	}

	if len(e.Command) > 0 {

		cmd := exec.Command(e.Command[0], e.Command[1:]...)
		pkg.Log.Debugf("Populating variable $%s by running command: %s", e.Name, strings.Join(e.Command, " "))
		err := cmd.Run()
		if err != nil {
			return errors.Errorf("error populating variable %q using command: %s", e.Name, err)
		}
		o, _ := cmd.Output()
		e.Value = string(o)
		pkg.Log.Debugf("Output: %s", e.Value)

		return nil
	}

	if e.Script != "" {
		pkg.Log.Debugf("Populating variable $%s by running script: %s", e.Name, e.Script)

		var err error
		e.Value, err = e.ensureWithScript()
		if err != nil {
			return errors.Errorf("error populating variable %q using script: %s", e.Name, err)
		}

		return nil
	}

	return errors.Errorf("no value, command, or script for variable %q", e.Name)
}


func (e *EnvironmentConfig) Ensure() error {
	for _, v := range e.Variables {
		if err := v.Ensure(); err != nil {
			return err
		}
	}

	return nil
}

func (e *EnvironmentConfig) Render() (string, error) {

	err := e.Ensure()
	if err != nil {
		return "", err
	}

	s := e.render()

	return s, nil
}

func (e *EnvironmentConfig) Execute() error {

	var cmds []*exec.Cmd

	for _, cmd := range e.Commands {
		var c []string
		switch runtime.GOOS {
		case "windows":
			c = cmd.WindowsCommand
		case "linux":
			c = cmd.LinuxCommand
		case "darwin":
			c = cmd.DarwinCommand
		}
		if len(c) == 0 {
			c = cmd.Command
		}
		if len(c) == 0 {
			return errors.Errorf(`command %q did not have an entry for "all" or for the current OS %q`, cmd.Name, runtime.GOOS)
		}

		cmds = append(cmds, exec.Command(c[0], c[1:]...))
	}

	for i, cmd := range cmds {
		label := fmt.Sprintf("%s (%s)", e.Commands[i].Name,
			strings.Join(cmd.Args, " "))
		pkg.Log.Debugf("Running command: %s", label)
		err := cmd.Run()
		if err != nil {
			return errors.Errorf("error running command %s: %s", label, err, )
		}
		o, _ := cmd.Output()
		if len(o) > 0 {
			pkg.Log.Debugf("Output: %s", string(o))
		}
	}

	return nil
}

func (e *EnvironmentConfig) Merge(other *EnvironmentConfig) {

	e.Cluster = firstNonemptyString(e.Cluster, other.Cluster)
	e.Domain = firstNonemptyString(e.Domain, other.Domain)

	e.Commands = append(e.Commands, other.Commands...)
	e.Variables = append(e.Variables, other.Variables...)

	for _, v := range other.Scripts {
		e.Scripts = append(e.Scripts, v)
	}
}

func firstNonemptyString(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}
