package environment

import (
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environmentvariables"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/script"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"os"
	"strings"
)

type Config struct {
	FromPath       string                 `yaml:"-" json:"-"`
	Name           string                 `yaml:"name" json:"name"`
	Role           core.EnvironmentRole   `yaml:"role" json:"role"`
	DefaultCluster string                 `yaml:"defaultCluster" json:"defaultCluster"`
	Clusters       kube.ConfigDefinitions `yaml:"clusters"`
	PullSecrets    []kube.PullSecret      `yaml:"pullSecrets"`
	// If true, commands which would cause modifications to be deployed will
	// trigger a confirmation prompt.
	Protected bool                             `yaml:"protected" json:"protected"`
	IsLocal   bool                             `yaml:"isLocal" json:"isLocal"`
	Commands  []*Command                       `yaml:"commands,omitempty" json:"commands,omitempty"`
	Variables []*environmentvariables.Variable `yaml:"variables,omitempty" json:"variables,omitempty"`
	Scripts   []*script.Script                 `yaml:"scripts,omitempty" json:"scripts,omitempty"`
	// Contains app value overrides which should be applied when deploying
	// apps to this environment.
	AppValues      *values.ValueSet           `yaml:"appValues" json:"appValues"`
	ValueSetNames  []string                   `yaml:"valueSets,omitempty" json:"valueSets,omitempty"`
	ValueOverrides *values.ValueSetCollection `yaml:"valueOverrides,omitempty"`
}

func (e *Config) MarshalYAML() (interface{}, error) {
	if e == nil {
		return nil, nil
	}
	type proxy Config
	p := proxy(*e)

	return &p, nil
}

func (e *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type proxy Config
	var p proxy
	if e != nil {
		p = proxy(*e)
	}

	err := unmarshal(&p)

	if err == nil {
		*e = Config(p)
	}

	return err
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
	for i := range e.Clusters {
		e.Clusters[i].FromPath = path
	}
}

// Ensure resolves and sets all environment variables, and
// sets the cluster, but only if the environment has not already
// been set.
func (e *Environment) Ensure(ctx environmentvariables.EnsureContext) error {

	if os.Getenv(core.EnvEnvironment) == e.Name {
		ctx.Log().Debugf("Environment is already %q, based on value of %s", e.Name, core.EnvEnvironment)

		return e.EnsureCluster(ctx)
	}

	return e.ForceEnsure(ctx)
}

// ForceEnsure resolves and sets all environment variables,
// even if the environment already appears to have been configured.
func (e *Environment) ForceEnsure(ctx environmentvariables.EnsureContext) error {

	ctx = ctx.WithPwd(e.FromPath).(environmentvariables.EnsureContext)

	for _, v := range e.Variables {
		if err := v.Ensure(ctx); err != nil {
			return err
		}
	}

	return e.EnsureCluster(ctx)
}

// Ensure resolves and sets all environment variables, and
// sets the cluster, but only if the environment has not already
// been set.
func (e *Environment) EnsureCluster(ctx environmentvariables.EnsureContext) error {

	if ctx.GetParameters().NoCluster {
		return nil
	}

	if e.ClusterName == "" {
		e.ClusterName = os.Getenv(core.EnvCluster)
	}

	if e.ClusterName == "" {
		e.ClusterName = e.DefaultCluster
	}

	var err error
	e.Cluster, err = e.GetClusterByName(e.ClusterName)
	if err != nil {
		return err
	}

	previouslySetCluster := os.Getenv(core.EnvCluster)
	if previouslySetCluster == e.Cluster.Name {
		return nil
	}

	_ = os.Setenv(core.EnvCluster, e.Cluster.Name)

	for _, v := range e.Cluster.Variables {
		if err = v.Ensure(ctx); err != nil {
			return err
		}
	}

	currentContext, _ := pkg.NewShellExe("kubectl config current-context ").RunOut()
	if currentContext != e.Cluster.Name {

		core.SetInternalEnvironmentAndCluster(e.Name, e.Cluster.Name)

		pkg.Log.Infof("Switching to cluster %q", e.Cluster.Name)

		out, err := pkg.NewShellExe("kubectl", "config", "use-context", e.Cluster.Name).RunOut()

		if strings.Contains(out, "no context exists with the name") {
			return errors.Errorf("No context found with name %q (try running `bosun kube configure-cluster %s`)", e.ClusterName, e.ClusterName)
		}
		return err
	}

	return nil
}

func (e *Environment) GetVariablesAsMap(ctx environmentvariables.EnsureContext) (map[string]string, error) {

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

	if e.Cluster != nil {
		for _, v := range e.Cluster.Variables {
			vars[v.Name] = v.Value
		}
	}

	return vars, nil
}

func (e *Environment) Render(ctx environmentvariables.EnsureContext) (string, error) {

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

func (e *Environment) Execute(ctx environmentvariables.EnsureContext) error {

	ctx = ctx.WithPwd(e.FromPath).(environmentvariables.EnsureContext)

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
