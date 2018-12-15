package bosun

import (
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
)

type EnvironmentConfig struct {
	FromPath  string                 `yaml:"fromPath,omitempty"`
	Name      string                 `yaml:name`
	Cluster   string                 `yaml:"cluster"`
	Domain    string                 `yaml:"domain"`
	Commands  []*EnvironmentCommand  `yaml:"commands,omitempty"`
	Variables []*EnvironmentVariable `yaml:"variables,omitempty"`
	Scripts   []*Script              `yaml:"scripts,omitempty"`
}

type EnvironmentVariable struct {
	FromPath string        `yaml:"fromPath,omitempty"`
	Name     string        `yaml:"name"`
	From     *DynamicValue `yaml:"from"`
	Value string `yaml:"-"`
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
func (e *EnvironmentVariable) Ensure() error {
	log := pkg.Log.WithField("name", e.Name).WithField("fromPath", e.FromPath)

	if e.From == nil {
		log.Warn("`from` was not set")
		return nil
	}

	if e.Value != "" {
		return nil
	}

	log.Debug("Resolving value...")

	ctx := NewResolveContext(e.FromPath)

	var err error
	e.Value, err = e.From.Resolve(ctx)

	if err != nil {
		return errors.Errorf("error populating variable %q", e.Name, err)
	}

	log.Debug("Resolved value.")

	return nil
}

func (e *EnvironmentConfig) Ensure() error {
	for _, v := range e.Variables {
		if err := v.Ensure(); err != nil {
			return err
		}
	}

	return nil
}

func (e *EnvironmentConfig) Render() (string, error) {

	err := e.Ensure()
	if err != nil {
		return "", err
	}

	s := e.render()

	return s, nil
}

func (e *EnvironmentConfig) Execute() error {

	ctx := NewResolveContext(e.FromPath)

	for _, cmd := range e.Commands {
		log := pkg.Log.WithField("name", cmd.Name).WithField("fromPath", e.FromPath)
		if cmd.Exec == nil {
			log.Warn("`exec` not set")
			continue
		}
		log.Debug("Running command...")
		_, err := cmd.Exec.Resolve(ctx)
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
