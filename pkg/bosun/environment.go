package bosun

import (
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/script"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"os"
)

type EnvironmentConfig struct {
	FromPath string `yaml:"-" json:"-"`
	Name     string `yaml:"name" json:"name"`
	Cluster  string `yaml:"cluster" json:"cluster"`
	Domain   string `yaml:"domain" json:"domain"`
	// If true, commands which would cause modifications to be deployed will
	// trigger a confirmation prompt.
	Protected bool                   `yaml:"protected" json:"protected"`
	IsLocal   bool                   `yaml:"isLocal" json:"isLocal"`
	Commands  []*EnvironmentCommand  `yaml:"commands,omitempty" json:"commands,omitempty"`
	Variables []*EnvironmentVariable `yaml:"variables,omitempty" json:"variables,omitempty"`
	Scripts   []*script.Script       `yaml:"scripts,omitempty" json:"scripts,omitempty"`
	// Contains app value overrides which should be applied when deploying
	// apps to this environment.
	AppValues *values.ValueSet `yaml:"appValues" json:"appValues"`
	HelmRepos []HelmRepo       `yaml:"helmRepos,omitempty" json:"helmRepos,omitempty"`
	ValueSets []string         `yaml:"valueSets,omitempty" json:"valueSets,omitempty"`
}

type EnvironmentVariable struct {
	FromPath         string                `yaml:"fromPath,omitempty" json:"fromPath,omitempty"`
	Name             string                `yaml:"name" json:"name"`
	WorkspaceCommand string                `yaml:"workspaceCommand,omitempty"`
	From             *command.CommandValue `yaml:"from" json:"from"`
	Value            string                `yaml:"-" json:"-"`
}

type EnvironmentCommand struct {
	FromPath string                `yaml:"fromPath,omitempty" json:"fromPath,omitempty"`
	Name     string                `yaml:"name" json:"name"`
	Exec     *command.CommandValue `yaml:"exec,omitempty" json:"exec,omitempty"`
}

type HelmRepo struct {
	Name        string            `yaml:"name" json:"name"`
	URL         string            `yaml:"url" json:"url"`
	Environment map[string]string `yaml:"environment" json:"environment"`
}

func (e *EnvironmentConfig) SetFromPath(path string) {
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

// Ensure sets Value using the From CommandValue.
func (e *EnvironmentVariable) Ensure(ctx BosunContext) error {

	if e.WorkspaceCommand != "" {
		ws := ctx.Bosun.GetWorkspace()
		e.From = ws.GetWorkspaceCommand(e.WorkspaceCommand)
	}

	ctx = ctx.WithDir(e.FromPath)
	log := ctx.Log().WithField("name", e.Name).WithField("fromPath", e.FromPath)

	if e.From == nil {
		log.Warn("`from` was not set")
		return nil
	}

	if e.Value == "" {
		log.Debug("Resolving value...")

		var err error
		e.Value, err = e.From.Resolve(ctx)

		if err != nil {
			return errors.Errorf("error populating variable %q: %s", e.Name, err)
		}

		log.WithField("value", e.Value).Debug("Resolved value.")
	}

	// set the value in the process environment
	return os.Setenv(e.Name, e.Value)
}

// Ensure resolves and sets all environment variables, and
// sets the cluster, but only if the environment has not already
// been set.
func (e *EnvironmentConfig) Ensure(ctx BosunContext) error {

	if os.Getenv(EnvEnvironment) == e.Name {
		ctx.Log().Debugf("Environment is already %q, based on value of %s", e.Name, EnvEnvironment)
		return nil
	}

	return e.ForceEnsure(ctx)
}

// ForceEnsure resolves and sets all environment variables,
// even if the environment already appears to have been configured.
func (e *EnvironmentConfig) ForceEnsure(ctx BosunContext) error {

	ctx = ctx.WithDir(e.FromPath)

	log := ctx.Log()

	os.Setenv(EnvDomain, e.Domain)
	os.Setenv(EnvCluster, e.Cluster)
	os.Setenv(EnvEnvironment, e.Name)

	_, err := pkg.NewShellExe("kubectl", "config", "use-context", e.Cluster).RunOut()
	if err != nil {
		log.Println(color.RedString("Error setting kubernetes context: %s\n", err))
		log.Println(color.YellowString(`try running "bosun kube add-eks %s"`, e.Cluster))
	}

	for _, v := range e.Variables {
		if err := v.Ensure(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (e *EnvironmentConfig) GetVariablesAsMap(ctx BosunContext) (map[string]string, error) {

	err := e.Ensure(ctx)
	if err != nil {
		return nil, err
	}

	vars := map[string]string{
		EnvDomain:      e.Domain,
		EnvCluster:     e.Cluster,
		EnvEnvironment: e.Name,
	}
	for _, v := range e.Variables {
		vars[v.Name] = v.Value
	}

	return vars, nil
}

func (e *EnvironmentConfig) Render(ctx BosunContext) (string, error) {

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

func (e *EnvironmentConfig) Execute(ctx BosunContext) error {

	ctx = ctx.WithDir(e.FromPath)

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
