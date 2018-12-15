package bosun

import (
	"github.com/naveego/bosun/pkg"
	"os"
	"runtime"
)

type DynamicValue struct {
	Value   string                   `yaml:"value:omitempty"`
	Command []string                 `yaml:"command,omitempty"`
	Script  string                   `yaml:"script,omitempty"`
	OS      map[string]*DynamicValue `yaml:"os,omitempty"`
	resolved bool
}

type ResolveContext struct {
	Dir string
}

func NewResolveContext(dir string) ResolveContext {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return ResolveContext{
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

func (d *DynamicValue) Resolve(ctx ResolveContext) (string, error) {
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
