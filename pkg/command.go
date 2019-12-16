package pkg

import (
	"context"
	"fmt"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/pkg/errors"
	"io"
	"os"
	"os/exec"
	"strings"
)

type Command struct {
	// Exe is the executable to invoke.
	Exe *string
	// Args is the arguments to be passed to the exe.
	Args []string
	Env  []string
	// Command is the exe with its args as a single string, as you would type it on the CLI.
	Command  *string
	Dir      *string
	ctx      context.Context
	prepared bool
	cmd      *exec.Cmd
	sudo     bool
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

func (c *Command) WithDir(dir string) *Command {
	c.Dir = &dir
	return c
}
func (c *Command) WithExe(exe string) *Command {
	c.Exe = &exe
	return c
}

func (c *Command) IncludeEnv(env map[string]string) *Command {
	for k, v := range env {
		c.WithEnvValue(k, v)
	}
	return c
}

func (c *Command) WithEnvValue(key, value string) *Command {
	c.Env = append(c.Env, fmt.Sprintf("%s=%s", key, value))
	return c
}

func (c *Command) WithContext(ctx context.Context) *Command {
	c.ctx = ctx
	return c
}

func (c *Command) WithArgs(args ...string) *Command {
	c.Args = append(c.Args, args...)
	return c
}

func (c *Command) WithCommand(cmd string) *Command {
	c.Command = &cmd
	return c
}

func (c *Command) GetCmd() *exec.Cmd {
	c.prepare()
	return c.cmd
}

func (c *Command) prepare() {

	if c.cmd != nil {
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

	if c.sudo {
		c.Args = append([]string{*c.Exe}, c.Args...)
		c.Exe = to.StringPtr("sudo")
	}

	exe, _ := exec.LookPath(*c.Exe)

	c.cmd = exec.Command(exe, c.Args...)

	c.cmd.Stdin = os.Stdin

	if c.Dir != nil {
		c.cmd.Dir = *c.Dir
	}

	c.cmd.Env = append(os.Environ(), c.Env...)

	Log.WithField("exe", exe).WithField("args", c.Args).
		//	WithField("env", c.cmd.Env).
		Debug("Command prepared.")

	c.prepared = true
}

// MustRun runs the command and kills this process if the command returns an error.
func (c *Command) MustRun() {
	Must(c.RunE())
}

func (c *Command) MustOut() string {
	out, err := c.RunOut()
	Must(err)
	return out
}

// RunE runs the command and returns the error only.
// Input and output for current process are attached to the command process.
func (c *Command) RunE() error {
	c.prepare()

	c.cmd.Stdin = os.Stdin
	c.cmd.Stdout = os.Stdout
	c.cmd.Stderr = os.Stderr
	var err error

	executeWithContext(c.ctx, c.cmd, func() {
		err = c.cmd.Run()
	})

	return err
}

// RunOut runs the command and returns the output or an error.
func (c *Command) RunOut() (string, error) {
	c.prepare()
	var result string
	var err error

	cmd := c.cmd
	executeWithContext(c.ctx, cmd, func() {
		var out []byte
		out, err = cmd.Output()
		if exitErr, ok := err.(*exec.ExitError); ok {
			err = errors.WithMessage(err, string(exitErr.Stderr))
		}
		result = strings.Trim(string(out), "\n\r")
	})

	return result, err
}

// RunOutLog runs the command and returns all output as a string.
func (c *Command) RunOutLog() (string, error) {
	c.prepare()
	var result string
	var err error

	cmd := c.cmd
	outCollector := new(strings.Builder)
	errCollector := new(strings.Builder)
	c.cmd.Stdout = io.MultiWriter(outCollector, os.Stdout)
	c.cmd.Stderr = io.MultiWriter(errCollector, os.Stderr)
	executeWithContext(c.ctx, cmd, func() {
		err = cmd.Run()
		if err != nil {
			err = errors.Errorf("cmd failed: %s, %s", err, errCollector.String())
		}
		result = strings.Trim(outCollector.String(), "\n\r")
	})

	return result, err
}

func (c *Command) Sudo(enabled bool) *Command {
	c.sudo = enabled
	return c

}

// Blocks until fn returns, or ctx is done. If ctx is done first
// and cmd is still running, cmd will be killed.
func executeWithContext(ctx context.Context, cmd *exec.Cmd, fn func()) {
	if ctx == nil {
		ctx = context.Background()
	}

	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()

	select {
	case <-done:
		return
	case <-ctx.Done():
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}

	<-done
}
