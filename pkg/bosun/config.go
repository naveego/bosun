package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
)

type RootConfig struct {
	Path               string               `yaml:"-"`
	CurrentEnvironment string               `yaml:"currentEnvironment"`
	Imports            []string             `yaml:"imports,omitempty"`
	AppStates               map[string]AppState         `yaml:"appStates"`
	MergedConfig *Config `yaml:"-"`
	ImportedConfigs map[string]*Config `yaml:"-"`
}

type Config struct {
	CurrentEnvironment string               `yaml:"currentEnvironment"`
	Repo               string               `yaml:"repo,omitempty"`
	Imports            []string             `yaml:"imports,omitempty"`
	Environments       []*EnvironmentConfig `yaml:"environments"`
	Apps               []*AppConfig         `yaml:"apps"`
	Path               string               `yaml:"-"`
	RootConfig *Config `yaml:"-"`
}

type State struct {
	Microservices map[string]AppState
}

type AppConfig struct {
	FromPath   string               `yaml:"fromPath,omitempty"`
	Name       string               `yaml:"name"`
	Namespace string 				`yaml:"namespace,omitempty"`
	Repo       string               `yaml:"repo,omitempty"`
	Version    string               `yaml:"version,omitempty"`
	ChartPath  string               `yaml:"chartPath,omitempty"`
	RunCommand []string             `yaml:"runCommand,omitempty"`
	DependsOn  []Dependency         `yaml:"dependsOn,omitempty"`
	Labels     []string             `yaml:"labels,omitempty"`
	Values     map[string]AppValues `yaml:"values,omitempty"`
	Error      error                `yaml:"-"`
}

type Dependency struct {
	Name string `yaml:"name"`
	Repo string `yaml:"repo"`
}

type AppValues struct {
	Set   map[string]string `yaml:"set,omitempty"`
	Files []string          `yaml:"files,omitempty"`
}

type AppState struct {
	Branch      string `yaml:"branch"`
	Deployed    bool   `yaml:"deployed"`
	RouteToHost bool   `yaml:"routeToHost"`
}

func (a *AppConfig) SetFromPath(path string) {
	a.FromPath = path
}

func LoadConfig(path string) (*RootConfig, error) {
	defaultPath := os.ExpandEnv("$HOME/.bosun/bosun.yaml")
	if path == "" {
		path = defaultPath
	} else {
		path = os.ExpandEnv(path)
	}

	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) && path == defaultPath {
			err = os.MkdirAll(filepath.Dir(defaultPath), 0600)
			if err != nil {
				return nil, errors.Errorf("could not create directory for default config file path: %s", err)
			}
			f, err := os.Open(defaultPath)
			if err != nil {
				return nil, errors.Errorf("could not create default config file: %s", err)
			}
			f.Close()
		} else {
			return nil, err
		}
	}

	c := &RootConfig{
		Path: path,
		AppStates: map[string]AppState{},
		ImportedConfigs: map[string]*Config{},
		MergedConfig: new(Config),
	}

	err = pkg.LoadYaml(path, &c)
	if err != nil {
		return nil, errors.Wrap(err, "loading root config")
	}

	for _, importPath := range c.Imports {

		err = c.importFromPath(importPath)
		if err != nil {
			pkg.Log.WithError(err).Error("Error importing config.")
		}
	}

	return c, nil
}

func (r *RootConfig) importFromPath(path string) error {
	log := pkg.Log.WithField("import_path", path)
	log.Debug("Importing config...")

	if r.ImportedConfigs[path] != nil {
		log.Info("Already imported.")
		return nil
	}

	c := &Config{
		Path: path,
	}

	err := pkg.LoadYaml(path, &c)

	if err != nil {
		return errors.Errorf("yaml error loading %q: %s", path, err)
	}

	for _, e := range c.Environments {
		e.SetFromPath(path)
	}

	for _, m := range c.Apps {
		m.SetFromPath(c.Path)
	}

	err = r.MergedConfig.Merge(c)

	if err != nil {
		return errors.Errorf("merge error loading %q: %s", path, err)
	}

	log.Debug("Import complete.")

	r.ImportedConfigs[path]=c

	for _, importPath := range c.Imports {
		err = r.importFromPath(importPath)
		if err != nil {
			return errors.Errorf("error with config imported to %q: %s", path)
		}
	}

	return nil
}


func (c *Config) Unmerge(toPath string) *Config {

	config := &Config{
		Path:               toPath,
		CurrentEnvironment: c.CurrentEnvironment,
		Imports:            c.Imports,
	}

	for _, e := range c.Environments {
		o := &EnvironmentConfig{
			Name:    e.Name,
			Domain:  e.Domain,
			Cluster: e.Cluster,
		}
		for _, x := range e.Scripts {
			if shouldMerge(x.FromPath, toPath) {
				o.Scripts = append(o.Scripts, x)

			}
		}
		for _, x := range e.Variables {
			if shouldMerge(x.FromPath, toPath) {
				o.Variables = append(o.Variables, x)
			}
		}
		for _, x := range e.Commands {
			if shouldMerge(x.FromPath, toPath) {
				o.Commands = append(o.Commands, x)
			}
		}
		config.Environments = append(config.Environments, o)
	}

	for _, m := range c.Apps {
		if shouldMerge(m.FromPath, toPath) {
			m2 := *m
			m2.FromPath = ""
			config.Apps = append(config.Apps, &m2)
		}
	}

	return config
}

func (c *Config) Merge(other *Config) error {

	for _, otherEnv := range other.Environments {
		c.mergeEnvironment(otherEnv)
	}

	for _, otherSvc := range other.Apps {
		c.mergeApp(otherSvc)
	}

	return nil
}

func (c *Config) mergeApp(svc *AppConfig) error {
	for _, e := range c.Apps {
		if e.Name == svc.Name {
			return errors.Errorf("duplicate microservice: %q is defined in %q and %q", svc.Name, svc.FromPath, e.FromPath)
		}
	}

	c.Apps = append(c.Apps, svc)

	return nil
}

func (c *Config) mergeEnvironment(env *EnvironmentConfig) error {
	for _, e := range c.Environments {
		if e.Name == env.Name {
			e.Merge(env)
			return nil
		}
	}

	c.Environments = append(c.Environments, env)

	return nil

}

func (c *Config) GetCurrentEnvironmentConfig() *EnvironmentConfig {
	for _, e := range c.Environments {
		if e.Name == c.CurrentEnvironment {
			return e
		}
	}

	panic(fmt.Sprintf("no environment named %q", c.CurrentEnvironment))
}

func shouldMerge(fromPath, toPath string) bool {
	return fromPath == "" || fromPath == toPath
}

func getStatePath(configPath string) string {
	statePath := filepath.Join(filepath.Dir(configPath), "state.yaml")
	return statePath
}
