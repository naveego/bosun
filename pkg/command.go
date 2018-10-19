package pkg

import (
	"fmt"
	"github.com/pkg/errors"
	"os"
	"os/exec"
	"strings"
)

type Command struct {
	// Exe is the executable to invoke.
	Exe *string
	// Args is the arguments to be passed to the exe.
	Args []string
	Env []string
	// Command is the exe with its args as a single string, as you would type it on the CLI.
	Command  *string
	Dir      *string
	prepared bool
	cmd *exec.Cmd
}

func NewCommand(exe string, args ...string) *Command {
	c := new(Command)
	if len(args) == 0 {
		c.Command = &exe
	} else {
		c.Exe = &exe
		c.Args = args
	}

	return c
}

func (c *Command) WithDir(dir string) *Command{
	c.Dir = &dir
	return c
}
func (c *Command) WithExe(exe string) *Command{
	c.Exe = &exe
	return c
}

func (c *Command) WithEnvValue(key,value string) *Command{
	c.Env = append(c.Env, fmt.Sprintf("%s=%s", key, value))
	return c
}

func (c *Command) WithArgs(args ...string) *Command{
	c.Args = args
	return c
}

func (c *Command) WithCommand(cmd string) *Command{
	c.Command = &cmd
	return c
}

func (c *Command) prepare() {


	if c.cmd != nil{
		return
	}


	if c.Command != nil {
		segs := strings.Fields(*c.Command)
		c.Exe = &segs[0]
		c.Args = segs[1:]
	} else {
		command := fmt.Sprintf("%s %s", *c.Exe, strings.Join(c.Args, " "))
		c.Command = &command
	}

	c.cmd = exec.Command(*c.Exe, c.Args...)

	if c.Dir != nil {
		c.cmd.Dir = *c.Dir
	}

	c.cmd.Env = append(os.Environ(), c.Env...)

	Log.WithField("command", c.Command).WithField("env", c.Env).Debug("Command prepared.")

	c.prepared = true
}

// MustRun runs the command and kills this process if the command returns an error.
func (c *Command) MustRun()  {
	Must(c.RunE())
}

// RunE runs the command and returns the error only.
func (c *Command) RunE() error {
	c.prepare()

	c.cmd.Stdout = os.Stdout
	c.cmd.Stderr = os.Stderr
	err := c.cmd.Run()

	return err
}

// RunOut runs the command and returns the output or an error.
func (c *Command) RunOut() (string, error) {
	c.prepare()

	cmd := c.cmd
	out, err := cmd.Output()
	if exitErr, ok :=  err.(*exec.ExitError); ok {
		err = errors.WithMessage(err, string(exitErr.Stderr))
	}
	result := strings.Trim(string(out),"\n\r")
	return result, err
}

