package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Script struct {
	Name     string       `yaml:"name"`
	FromPath string `yaml:"fromPath,omitempty"`
	Description string `yaml:"description,omitempty"`
	Steps    []ScriptStep `yaml:"steps,omitempty"`
	Literal *DynamicValue `yaml:"literal,omitempty"`
}

type ScriptStep struct {
	Name string `yaml:"name,omitempty"`
	Description string `yaml:"description,omitempty"`
	Command string `yaml:"command"`
	Args    []string `yaml:"args"`
	Flags   map[string]interface{} `yaml:"flags"`
	Literal *DynamicValue `yaml:"literal,omitempty"`
}

func (b *Bosun) Execute(s *Script, steps ...int) error {

	log := pkg.Log.WithField("name", s.Name)

	relativeDir := filepath.Dir(s.FromPath)

	env := b.GetCurrentEnvironment()
	var err error
	ctx := b.NewContext("")

	if err = env.Ensure(ctx); err != nil {
		return errors.Wrap(err, "ensure environment")
	}

	if _, err = env.Render(ctx); err != nil {
		return errors.Wrap(err, "render environment")
	}

	if s.Literal != nil {
		log.Debug("Executing literal script, not bosun script.")
		_, err = s.Literal.Execute(ctx.WithDir(filepath.Dir(s.FromPath)))
		return err
	}


	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if !strings.Contains(exe, "bosun") {
		exe = "bosun" // support working under debugger
	}

	exe, err = exec.LookPath(exe)
	if err != nil {
		return err
	}

	if len(steps) == 0 {
		for i := range s.Steps {
			steps = append(steps, i)
		}
	}

	for _, i := range steps {
		if i >= len(s.Steps) {
			return errors.Errorf("invalid step %d (there are %d steps)", i, len(s.Steps))
		}
		step := s.Steps[i]
		log := pkg.Log.WithField("step", i).WithField("command", step.Command)
		if step.Name != "" {
			log = log.WithField("name", step.Name)
		}
		if step.Description != "" {
			log.Info(step.Description)
		}

		if step.Literal != nil {
			log.Info("Step is a literal script, not a bosun action.")

			_, err = step.Literal.Execute(ctx.WithDir(filepath.Dir(s.FromPath)))
			if err != nil {
				return err
			}
			continue
		}

		if step.Flags == nil {
			step.Flags = make(map[string]interface{})
		}

		var stepArgs []string
		stepArgs = append(stepArgs, strings.Fields(step.Command)...)
		stepArgs = append(stepArgs, "--step", fmt.Sprintf("%d", i))
		if b.params.Verbose {
			stepArgs = append(stepArgs, "--verbose")
		}
		if b.params.DryRun {
			stepArgs = append(stepArgs, "--dry-run")
		}

		step.Flags["domain"] = env.Domain
		step.Flags["cluster"] = env.Cluster

		for k, v := range step.Flags {
			switch vt := v.(type) {
			case []interface{}:
				var arr []string
				for _, i := range vt {
					arr = append(arr, fmt.Sprint(i))
				}
				stepArgs = append(stepArgs, fmt.Sprintf("--%s", k), strings.Join(arr, ","))
			case bool:
				stepArgs = append(stepArgs, fmt.Sprintf("--%s", k))
			default:
				stepArgs = append(stepArgs, fmt.Sprintf("--%s", k), fmt.Sprintf("%v", vt))
			}
		}

		for _, v := range step.Args {
			stepArgs = append(stepArgs, v)
		}

		log.WithField("args", stepArgs).Info("Executing step")

		err = pkg.NewCommand(exe, stepArgs...).WithDir(relativeDir).RunE()
		if err != nil {
			log.WithField("flags", step.Flags).WithField("args", step.Args).Error("Step failed.")
			return errors.New("s abended")
		}
	}

	return nil
}
