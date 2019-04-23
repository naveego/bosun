package bosun

import (
	"fmt"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"regexp"
)

// File represents a loaded bosun.yaml file.
type File struct {
	Imports      []string               `yaml:"imports,omitempty" json:"imports"`
	Environments []*EnvironmentConfig   `yaml:"environments" json:"environments"`
	AppRefs      map[string]*Dependency `yaml:"appRefs" json:"appRefs"`
	Apps         []*AppRepoConfig       `yaml:"apps" json:"apps"`
	FromPath     string                 `yaml:"fromPath" json:"fromPath"`
	Config       *Workspace             `yaml:"-" json:"-"`
	Releases     []*ReleaseConfig       `yaml:"releases,omitempty" json:"releases"`
	Tools        []ToolDef              `yaml:"tools,omitempty" json:"tools"`
	TestSuites   []*E2ESuiteConfig      `yaml:"testSuites,omitempty" json:"testSuites,omitempty"`
	Scripts      []*Script              `yaml:"scripts,omitempty" json:"scripts,omitempty"`

	// merged indicates that this File has had File instances merged into it and cannot be saved.
	merged bool `yaml:"-" json:"-"`
}

func (c *File) SetFromPath(path string) {

	c.FromPath = path

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
		m.SetParent(c)
	}

	for _, s := range c.Scripts {
		s.SetFromPath(path)
	}

	for i := range c.Tools {
		c.Tools[i].FromPath = c.FromPath
	}

	for i := range c.TestSuites {
		c.TestSuites[i].SetFromPath(c.FromPath)
	}
}

func (c *File) Merge(other *File) error {

	c.merged = true

	for _, otherEnv := range other.Environments {
		err := c.mergeEnvironment(otherEnv)
		if err != nil {
			return errors.Wrap(err, "merge environment")
		}
	}

	if c.AppRefs == nil {
		c.AppRefs = make(map[string]*Dependency)
	}

	for k, other := range other.AppRefs {
		other.Name = k
		c.AppRefs[k] = other
	}

	for _, otherApp := range other.Apps {
		if err := c.mergeApp(otherApp); err != nil {
			return errors.Wrapf(err, "merge app %q", otherApp.Name)
		}
	}

	for _, release := range other.Releases {
		if err := c.mergeRelease(release); err != nil {
			return errors.Wrapf(err, "merge release %q", release.Name)
		}
	}

	for _, other := range other.Scripts {
		c.Scripts = append(c.Scripts, other)
	}

	c.TestSuites = append(c.TestSuites, other.TestSuites...)
	c.Tools = append(c.Tools, other.Tools...)

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

	err = ioutil.WriteFile(c.FromPath, b, 0600)
	if err != nil {
		return err
	}

	for _, release := range c.Releases {
		err = release.SaveBundle()
		if err != nil {
			return errors.Wrapf(err, "saving bundle for release %q", release.Name)
		}
	}

	return nil
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
