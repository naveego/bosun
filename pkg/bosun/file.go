package bosun

import (
	"fmt"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"regexp"
)

// File represents a loaded bosun.yaml file.
type File struct {
	Imports      []string               `yaml:"imports,omitempty"`
	Environments []*EnvironmentConfig   `yaml:"environments"`
	AppRefs      map[string]*Dependency `yaml:"appRefs"`
	Apps         []*AppRepoConfig       `yaml:"apps"`
	FromPath     string                 `yaml:"fromPath"`
	Config       *Workspace             `yaml:"-"`
	Releases     []*ReleaseConfig       `yaml:"releases,omitempty"`
	// merged indicates that this File has had File instances merged into it and cannot be saved.
	merged bool `yaml:"-"`
}


func (c *File) Merge(other *File) error {

	c.merged = true

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

func (c *File) Save() error {
	if c.merged {
		panic("a merged File cannot be saved")
	}

	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	b = stripFromPath.ReplaceAll(b, []byte{})

	return err
}

var stripFromPath = regexp.MustCompile(`\s*fromPath:.*`)

func (c *File) mergeApp(incoming *AppRepoConfig) error {
	for _, app := range c.Apps {
		if app.Name == incoming.Name {
			return errors.Errorf("app %q imported from %q, but it was already imported from %q", incoming.Name, incoming.FromPath, app.FromPath)
		}
	}

	c.Apps = append(c.Apps, incoming)

	return nil
}

func (c *File) mergeEnvironment(env *EnvironmentConfig) error {

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

func (c *File) GetEnvironmentConfig(name string) *EnvironmentConfig {
	for _, e := range c.Environments {
		if e.Name == name {
			return e
		}
	}

	panic(fmt.Sprintf("no environment named %q", name))
}

func (c *File) mergeRelease(release *ReleaseConfig) error {
	for _, e := range c.Releases {
		if e.Name == release.Name {
			return errors.Errorf("already have a release named %q, from %q", release.Name, e.FromPath)

		}
	}

	c.Releases = append(c.Releases, release)
	return nil
}
