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
	FromPath string `yaml:"fromPath,omitempty"`
	Name     string       `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Steps    []ScriptStep `yaml:steps`
}

type ScriptStep struct {
	Command string `yaml:"command"`
	Args    []string `yaml:"args"`
	Flags   map[string]interface{} `yaml:"flags"`
}

func (b *Bosun) Execute(s *Script, steps ...int) error {

	relativeDir := filepath.Dir(s.FromPath)

	env, err := b.GetCurrentEnvironment()
	if err != nil {
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
