package environment

import (
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/script"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"os"
)

type Config struct {
	FromPath       string                 `yaml:"-" json:"-"`
	Name           string                 `yaml:"name" json:"name"`
	Roles          []core.EnvironmentRole `yaml:"roles" json:"roles"`
	DefaultCluster string                 `yaml:"defaultCluster" json:"defaultCluster"`
	// If true, commands which would cause modifications to be deployed will
	// trigger a confirmation prompt.
	Protected bool             `yaml:"protected" json:"protected"`
	IsLocal   bool             `yaml:"isLocal" json:"isLocal"`
	Commands  []*Command       `yaml:"commands,omitempty" json:"commands,omitempty"`
	Variables []*Variable      `yaml:"variables,omitempty" json:"variables,omitempty"`
	Scripts   []*script.Script `yaml:"scripts,omitempty" json:"scripts,omitempty"`
	// Contains app value overrides which should be applied when deploying
	// apps to this environment.
	AppValues     *values.ValueSet `yaml:"appValues" json:"appValues"`
	ValueSetNames []string         `yaml:"valueSets,omitempty" json:"valueSets,omitempty"`
}

type Command struct {
	FromPath string                `yaml:"fromPath,omitempty" json:"fromPath,omitempty"`
	Name     string                `yaml:"name" json:"name"`
	Exec     *command.CommandValue `yaml:"exec,omitempty" json:"exec,omitempty"`
}

func (e *Config) SetFromPath(path string) {
	e.FromPath = path
	for i := range e.Scripts {
		e.Scripts[i].SetFromPath(path)
	}
	for i := range e.Variables {
		e.Variables[i].FromPath = path
	}
	for i := range e.Commands {
		e.Variables[i].FromPath = path
	}
}

// Ensure resolves and sets all environment variables, and
// sets the cluster, but only if the environment has not already
// been set.
func (e *Environment) Ensure(ctx EnsureContext) error {

	if os.Getenv(core.EnvEnvironment) == e.Name {
		ctx.Log().Debugf("Environment is already %q, based on value of %s", e.Name, core.EnvEnvironment)
		return nil
	}

	return e.ForceEnsure(ctx)
}

// ForceEnsure resolves and sets all environment variables,
// even if the environment already appears to have been configured.
func (e *Environment) ForceEnsure(ctx EnsureContext) error {

	ctx = ctx.WithPwd(e.FromPath).(EnsureContext)

	log := ctx.Log()

	_ = os.Setenv(core.EnvEnvironment, e.Name)

	_, err := pkg.NewShellExe("kubectl", "config", "use-context", e.Cluster).RunOut()
	if err != nil {
		log.Println(color.RedString("Error setting kubernetes context: %s\n", err))
		log.Println(color.YellowString(`try running "bosun kube configure-cluster %s"`, e.Cluster))
	}

	for _, v := range e.Variables {
		if err := v.Ensure(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (e *Environment) GetVariablesAsMap(ctx EnsureContext) (map[string]string, error) {

	err := e.Ensure(ctx)
	if err != nil {
		return nil, err
	}

	vars := map[string]string{
		core.EnvEnvironment: e.Name,
	}
	for _, v := range e.Variables {
		vars[v.Name] = v.Value
	}

	return vars, nil
}

func (e *Environment) Render(ctx EnsureContext) (string, error) {

	err := e.Ensure(ctx)
	if err != nil {
		return "", err
	}

	vars, err := e.GetVariablesAsMap(ctx)
	if err != nil {
		return "", err
	}

	s := command.RenderEnvironmentSettingScript(vars)

	return s, nil
}

func (e *Environment) Execute(ctx EnsureContext) error {

	ctx = ctx.WithPwd(e.FromPath).(EnsureContext)

	for _, cmd := range e.Commands {
		log := ctx.Log().WithField("name", cmd.Name).WithField("fromPath", e.FromPath)
		if cmd.Exec == nil {
			log.Warn("`exec` not set")
			continue
		}
		log.Debug("Running command...")
		_, err := cmd.Exec.Execute(ctx)
		if err != nil {
			return errors.Errorf("error running command %s: %s", cmd.Name, err)
		}
		log.Debug("ShellExe complete.")

	}

	return nil
}

func (e *Config) Merge(other *Config) {

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
