package command

import (
	"context"
	"fmt"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
	"io"
	"os"
	"os/exec"
	"strings"
)

type ShellExe struct {
	// Exe is the executable to invoke.
	Exe *string
	// Args is the arguments to be passed to the exe.
	Args []string
	Env  []string
	// ShellExe is the exe with its args as a single string, as you would type it on the CLI.
	Command  *string
	Dir      *string
	ctx      context.Context
	prepared bool
	cmd      *exec.Cmd
	sudo     bool
}

func NewShellExe(exe string, args ...string) *ShellExe {
	c := new(ShellExe)
	if len(args) == 0 {
		c.Command = &exe
	} else {
		c.Exe = &exe
		c.Args = args
	}

	return c
}

func NewShellExeFromSlice(args ...string) *ShellExe {
	return NewShellExe(args[0], args[1:]...)
}

func (c *ShellExe) WithDir(dir string) *ShellExe {
	c.Dir = &dir
	return c
}
func (c *ShellExe) WithExe(exe string) *ShellExe {
	c.Exe = &exe
	return c
}

func (c *ShellExe) IncludeEnv(env map[string]string) *ShellExe {
	for k, v := range env {
		c.WithEnvValue(k, v)
	}
	return c
}

func (c *ShellExe) WithEnvValue(key, value string) *ShellExe {
	c.Env = append(c.Env, fmt.Sprintf("%s=%s", key, value))
	return c
}

func (c *ShellExe) WithContext(ctx context.Context) *ShellExe {
	c.ctx = ctx
	return c
}

func (c *ShellExe) WithArgs(args ...string) *ShellExe {
	c.Args = append(c.Args, args...)
	return c
}

func (c *ShellExe) WithCommand(cmd string) *ShellExe {
	c.Command = &cmd
	return c
}

func (c *ShellExe) GetCmd() *exec.Cmd {
	c.prepare()
	return c.cmd
}

func (c *ShellExe) prepare() {

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

	core.Log.WithField("exe", exe).
		WithField("args", c.Args).
		WithField("dir", c.cmd.Dir).
		//	WithField("env", c.cmd.Env).
		Debug("ShellExe prepared.")

	c.prepared = true
}

// MustRun runs the command and kills this process if the command returns an error.
func (c *ShellExe) MustRun() {
	util.Must(c.RunE())
}

func (c *ShellExe) MustOut() string {
	out, err := c.RunOut()
	util.Must(err)
	return out
}

// RunE runs the command and returns the error only.
// Input and output for current process are attached to the command process.
func (c *ShellExe) RunE() error {
	c.prepare()

	c.cmd.Stdin = os.Stdin
	c.cmd.Stdout = os.Stdout
	c.cmd.Stderr = os.Stderr
	var err error

	executeWithContext(c.ctx, c.cmd, func() {
		err = c.cmd.Run()
	})

	return errors.Wrapf(err, "command failed: %s", c.String())
}

func (c *ShellExe) String() string {
	return fmt.Sprintf("%s %s", to.String(c.Exe), strings.Join(c.Args, " "))
}

// RunOut runs the command and returns the output or an error.
func (c *ShellExe) RunOut() (string, error) {
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
func (c *ShellExe) RunOutLog() (string, error) {
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

func (c *ShellExe) Sudo(enabled bool) *ShellExe {
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
