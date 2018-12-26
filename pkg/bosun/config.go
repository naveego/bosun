package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
)

type Config struct {
	Path               string                     `yaml:"-"`
	CurrentEnvironment string                     `yaml:"currentEnvironment"`
	Imports            []string                   `yaml:"imports,omitempty"`
	GitRoots           []string                   `yaml:"gitRoots"`
	Release            string                     `yaml:"release"`
	AppStates          AppStatesByEnvironment     `yaml:"appStates"`
	MergedFragments    *ConfigFragment            `yaml:"-"`
	ImportedFragments  map[string]*ConfigFragment `yaml:"-"`
}

type ConfigFragment struct {
	Repo         string                 `yaml:"repo,omitempty"`
	Imports      []string               `yaml:"imports,omitempty"`
	Environments []*EnvironmentConfig   `yaml:"environments"`
	AppRefs      map[string]*Dependency `yaml:"appRefs"`
	Apps         []*AppConfig           `yaml:"apps"`
	FromPath     string                 `yaml:"-"`
	Config       *Config                `yaml:"-"`
	Releases     []*Release             `yaml:"releases,omitempty"`
}

type State struct {
	Microservices map[string]AppState
}

func LoadConfig(path string) (*Config, error) {
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
				return nil, errors.Errorf("could not create directory for default mergedFragments file path: %s", err)
			}
			f, err := os.Open(defaultPath)
			if err != nil {
				return nil, errors.Errorf("could not create default mergedFragments file: %s", err)
			}
			f.Close()
		} else {
			return nil, err
		}
	}

	c := &Config{
		Path:              path,
		AppStates:         AppStatesByEnvironment{},
		ImportedFragments: map[string]*ConfigFragment{},
		MergedFragments:   new(ConfigFragment),
	}

	err = pkg.LoadYaml(path, &c)
	if err != nil {
		return nil, errors.Wrap(err, "loading root config")
	}

	err = c.importFromPaths(path, c.Imports)

	if err != nil {
		return nil, errors.Wrap(err, "loading imports")
	}

	var syntheticPaths []string
	for _, app := range c.MergedFragments.AppRefs {
		if app.Repo != "" {
			for _, root := range c.GitRoots {
				dir := filepath.Join(root, app.Repo)
				bosunFile := filepath.Join(dir, "bosun.yaml")
				if _, err := os.Stat(bosunFile); err == nil {
					syntheticPaths = append(syntheticPaths, bosunFile)
				}
			}
		}
	}

	err = c.importFromPaths(path, syntheticPaths)
	if err != nil {
		return nil, errors.Errorf("error importing from synthetic paths based on %q: %s", path, err)
	}

	return c, err
}

func (r *Config) importFromPaths(relativeTo string, paths []string) error {
	for _, importPath := range paths {
		for _, importPath = range expandPath(relativeTo, importPath) {
			err := r.importFragmentFromPath(importPath)
			if err != nil {
				return errors.Errorf("error importing fragment relative to %q: %s", relativeTo, err)
			}
		}
	}

	return nil
}

func (r *Config) importFragmentFromPath(path string) error {
	//log := pkg.Log.WithField("import_path", path)
	//log.Debug("Importing mergedFragments...")

	if r.ImportedFragments[path] != nil {
	//	log.Debugf("Already imported.")
		return nil
	}

	c := &ConfigFragment{
		FromPath: path,
		AppRefs:  map[string]*Dependency{},
	}

	err := pkg.LoadYaml(path, &c)

	if err != nil {
		return errors.Errorf("yaml error loading %q: %s", path, err)
	}

	for _, e := range c.Environments {
		e.SetFromPath(path)
	}

	for _, m := range c.Apps {
		m.SetFragment(c)
	}

	for _, m := range c.AppRefs {
		m.FromPath = path
	}

	for _, m := range c.Releases {
		m.SetFragment(c)
	}

	err = r.MergedFragments.Merge(c)

	if err != nil {
		return errors.Errorf("merge error loading %q: %s", path, err)
	}

	//log.Debug("Import complete.")

	r.ImportedFragments[path] = c

	err = r.importFromPaths(c.FromPath, c.Imports)

	return err
}

func (c *ConfigFragment) Merge(other *ConfigFragment) error {

	for _, otherEnv := range other.Environments {
		c.mergeEnvironment(otherEnv)
	}

	if c.AppRefs == nil {
		c.AppRefs = make(map[string]*Dependency)
	}

	for k, other := range other.AppRefs {
		other.Name = k
		c.AppRefs[k] = other
	}

	for _, otherApp := range other.Apps {
		c.mergeApp(otherApp)
	}

	for _, other := range other.Releases {
		c.mergeRelease(other)
	}

	return nil
}

func (c *ConfigFragment) Save() error {
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(c.FromPath, b, 0600)
	return err
}

func (c *ConfigFragment) mergeApp(incoming *AppConfig) error {
	for _, app := range c.Apps {
		if app.Name == incoming.Name {
			return errors.Errorf("app %q imported from %q, but it was already imported frome %q", incoming.Name, incoming.FromPath, app.FromPath)
		}
	}

	c.Apps = append(c.Apps, incoming)

	return nil
}

func (c *ConfigFragment) mergeEnvironment(env *EnvironmentConfig) error {

	if env.Name == "all" {
		for _, e := range c.Environments {
			e.Merge(env)
		}
		return nil
	}

	for _, e := range c.Environments {
		if e.Name == env.Name {
			e.Merge(env)
			return nil
		}
	}

	c.Environments = append(c.Environments, env)

	return nil
}

func (c *ConfigFragment) GetEnvironmentConfig(name string) *EnvironmentConfig {
	for _, e := range c.Environments {
		if e.Name == name {
			return e
		}
	}

	panic(fmt.Sprintf("no environment named %q", name))
}

func (c *ConfigFragment) mergeRelease(release *Release) error {
	for _, e := range c.Releases {
		if e.Name == release.Name {
			return errors.Errorf("already have a release named %q, from %q", release.Name, e.FromPath)

		}
	}

	c.Releases = append(c.Releases, release)
	return nil
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
