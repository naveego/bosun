package bosun

import (
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"os"
)

type EnvironmentConfig struct {
	FromPath  string                 `yaml:"fromPath,omitempty"`
	Name      string                 `yaml:name`
	Cluster   string                 `yaml:"cluster"`
	Domain    string                 `yaml:"domain"`
	IsLocal bool `yaml:"isLocal"`
	Commands  []*EnvironmentCommand  `yaml:"commands,omitempty"`
	Variables []*EnvironmentVariable `yaml:"variables,omitempty"`
	Scripts   []*Script              `yaml:"scripts,omitempty"`
}

type EnvironmentVariable struct {
	FromPath string        `yaml:"fromPath,omitempty"`
	Name     string        `yaml:"name"`
	From     *DynamicValue `yaml:"from"`
	Value    string        `yaml:"-"`
}

type EnvironmentCommand struct {
	FromPath string        `yaml:"fromPath,omitempty"`
	Name     string        `yaml:"name"`
	Exec     *DynamicValue `yaml:"exec,omitempty"`
}

func (e *EnvironmentConfig) SetFromPath(path string) {
	e.FromPath = path
	for i := range e.Scripts {
		e.Scripts[i].FromPath = path
	}
	for i := range e.Variables {
		e.Variables[i].FromPath = path
	}
	for i := range e.Commands {
		e.Variables[i].FromPath = path
	}
}

// Ensure sets Value using the From DynamicValue.
func (e *EnvironmentVariable) Ensure(ctx BosunContext) error {
	ctx = ctx.WithDir(e.FromPath)
	log := pkg.Log.WithField("name", e.Name).WithField("fromPath", e.FromPath)

	if e.From == nil {
		log.Warn("`from` was not set")
		return nil
	}


	if e.Value != "" {
		// set the value in the process environment
		os.Setenv(e.Name, e.Value)
		return nil
	}

	log.Debug("Resolving value...")


	var err error
	e.Value, err = e.From.Resolve(ctx)

	if err != nil {
		return errors.Errorf("error populating variable %q: %s", e.Name, err)
	}

	log.WithField("value", e.Value).Debug("Resolved value.")

	// set the value in the process environment
	os.Setenv(e.Name, e.Value)

	return nil
}

// Ensure resolves and sets all environment variables, and
// sets the cluster, but only if the environment has not already
// been set.
func (e *EnvironmentConfig) Ensure(ctx BosunContext) error {

	if os.Getenv(EnvEnvironment) == e.Name {
		pkg.Log.Debugf("Environment is already %q, based on value of %s", e.Name, EnvEnvironment)
		return nil
	}

	return e.ForceEnsure(ctx)
}

// ForceEnsure resolves and sets all environment variables,
// even if the environment already appears to have been configured.
func (e *EnvironmentConfig) ForceEnsure(ctx BosunContext) error {

	ctx = ctx.WithDir(e.FromPath)

	os.Setenv(EnvDomain, e.Domain)
	os.Setenv(EnvCluster, e.Cluster)
	os.Setenv(EnvEnvironment, e.Name)

	_, err := pkg.NewCommand("kubectl", "config", "use-context", e.Cluster).RunOut()
	if err != nil {
		return err
	}

	for _, v := range e.Variables {
		if err := v.Ensure(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (e *EnvironmentConfig) Render(ctx BosunContext) (string, error) {

	err := e.Ensure(ctx)
	if err != nil {
		return "", err
	}

	vars := map[string]string{
		EnvDomain: e.Domain,
		EnvCluster: e.Cluster,
		EnvEnvironment: e.Name,
	}
	for _, v := range e.Variables {
		vars[v.Name] = v.Value
	}

	s := render(vars)

	return s, nil
}

func (e *EnvironmentConfig) Execute(ctx BosunContext) error {

	ctx = ctx.WithDir(e.FromPath)

	for _, cmd := range e.Commands {
		log := pkg.Log.WithField("name", cmd.Name).WithField("fromPath", e.FromPath)
		if cmd.Exec == nil {
			log.Warn("`exec` not set")
			continue
		}
		log.Debug("Running command...")
		_, err := cmd.Exec.Execute(ctx)
		if err != nil {
			return errors.Errorf("error running command %s: %s", cmd.Name, err)
		}
		log.Debug("Command complete.")

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
