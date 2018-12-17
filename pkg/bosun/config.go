package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
)

type RootConfig struct {
	Path               string              `yaml:"-"`
	CurrentEnvironment string              `yaml:"currentEnvironment"`
	Imports            []string            `yaml:"imports,omitempty"`
	AppStates          map[string]AppState `yaml:"appStates"`
	MergedConfig       *Config             `yaml:"-"`
	ImportedConfigs    map[string]*Config  `yaml:"-"`
}

type Config struct {
	Repo         string               `yaml:"repo,omitempty"`
	Imports      []string             `yaml:"imports,omitempty"`
	Environments []*EnvironmentConfig `yaml:"environments"`
	Apps         []*AppConfig         `yaml:"apps"`
	Path         string               `yaml:"-"`
	RootConfig   *Config              `yaml:"-"`
}

type State struct {
	Microservices map[string]AppState
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
		Path:            path,
		AppStates:       map[string]AppState{},
		ImportedConfigs: map[string]*Config{},
		MergedConfig:    new(Config),
	}

	err = pkg.LoadYaml(path, &c)
	if err != nil {
		return nil, errors.Wrap(err, "loading root config")
	}

	err = c.importFromPaths(path, c.Imports)

	return c, err
}

func (r *RootConfig) importFromPaths(relativeTo string, paths []string) error {
	for _, importPath := range paths {
		for _, importPath = range expandPath(relativeTo, importPath) {
			err := r.importFromPath(importPath)
			if err != nil {
				return errors.Errorf("error importing config relative to %q: %s", relativeTo, err)
			}
		}
	}
	return nil
}

func (r *RootConfig) importFromPath(path string) error {
	log := pkg.Log.WithField("import_path", path)
	log.Debug("Importing config...")

	if r.ImportedConfigs[path] != nil {
		// log.Info("Already imported.")
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
		m.SetFromPath(path)
	}

	err = r.MergedConfig.Merge(c)

	if err != nil {
		return errors.Errorf("merge error loading %q: %s", path, err)
	}

	log.Debug("Import complete.")

	r.ImportedConfigs[path] = c

	err = r.importFromPaths(c.Path, c.Imports)

	return nil
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

func (c *Config) GetEnvironmentConfig(name string) *EnvironmentConfig {
	for _, e := range c.Environments {
		if e.Name == name {
			return e
		}
	}

	panic(fmt.Sprintf("no environment named %q", name))
}

// expandPath resolves a path relative to another file's path, including expanding env variables and globs.
func expandPath(relativeToFile, path string) []string {

	path = resolvePath(relativeToFile, path)

	paths, _ := filepath.Glob(path)

	return paths
}

func resolvePath(relativeToFile, path string) string {
	path = os.ExpandEnv(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(relativeToFile), path)
	}
	return path
}
