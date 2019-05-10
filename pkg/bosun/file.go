package bosun

import (
	"fmt"
	"github.com/imdario/mergo"
	"github.com/naveego/bosun/pkg/mirror"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"regexp"
)

// File represents a loaded bosun.yaml file.
type File struct {
	Imports      []string               `yaml:"imports,omitempty" json:"imports"`
	Environments []*EnvironmentConfig   `yaml:"environments,omitempty" json:"environments"`
	AppRefs      map[string]*Dependency `yaml:"appRefs,omitempty" json:"appRefs"`
	Apps         []*AppConfig           `yaml:"apps,omitempty" json:"apps"`
	Repos        []*RepoConfig          `yaml:"repos,omitempty" json:"repos"`
	FromPath     string                 `yaml:"fromPath" json:"fromPath"`
	Config       *Workspace             `yaml:"-" json:"-"`
	Tools        []*ToolDef             `yaml:"tools,omitempty" json:"tools"`
	TestSuites   []*E2ESuiteConfig      `yaml:"testSuites,omitempty" json:"testSuites,omitempty"`
	Scripts      []*Script              `yaml:"scripts,omitempty" json:"scripts,omitempty"`
	ValueSets    []*ValueSet            `yaml:"valueSets,omitempty" json:"valueSets,omitempty"`
	Platforms    []*Platform            `yaml:"platforms,omitempty" json:"platforms,omitempty"`

	// merged indicates that this File has had File instances merged into it and cannot be saved.
	merged bool `yaml:"-" json:"-"`
}

func (f *File) MarshalYAML() (interface{}, error) {
	if f == nil {
		return nil, nil
	}
	type proxy File
	p := proxy(*f)

	return &p, nil
}

func (f *File) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type proxy File
	var p proxy
	if f != nil {
		p = proxy(*f)
	}

	err := unmarshal(&p)

	if err == nil {
		*f = File(p)
	}

	return err
}

type ParentSetter interface {
	SetParent(*File)
}

type FromPathSetter interface {
	SetFromPath(string)
}

func (f *File) SetFromPath(path string) {

	f.FromPath = path

	mirror.ApplyFuncRecursively(f, func(x ParentSetter) {
		x.SetParent(f)
	})

	mirror.ApplyFuncRecursively(f, func(x FromPathSetter) {
		x.SetFromPath(f.FromPath)
	})
}

func (f *File) Merge(other *File) error {

	f.merged = true

	for _, otherEnv := range other.Environments {
		err := f.mergeEnvironment(otherEnv)
		if err != nil {
			return errors.Wrap(err, "merge environment")
		}
	}

	for _, otherApp := range other.Apps {
		if err := f.mergeApp(otherApp); err != nil {
			return errors.Wrapf(err, "merge app %q", otherApp.Name)
		}
	}

	err := mergo.Merge(f, other, mergo.WithAppendSlice)
	return err

	//
	// f.Scripts = append(f.Scripts, other.Scripts...)
	// f.Repos = append(f.Repos, other.Repos...)
	// f.TestSuites = append(f.TestSuites, other.TestSuites...)
	// f.Tools = append(f.Tools, other.Tools...)
	//
	// return nil
}

func (f *File) Save() error {
	if f.merged {
		panic("a merged File cannot be saved")
	}

	b, err := yaml.Marshal(f)
	if err != nil {
		return err
	}

	b = stripFromPath.ReplaceAll(b, []byte{})

	err = ioutil.WriteFile(f.FromPath, b, 0600)
	if err != nil {
		return err
	}

	return nil
}

var stripFromPath = regexp.MustCompile(`\s*fromPath:.*`)

func (f *File) mergeApp(incoming *AppConfig) error {
	for _, app := range f.Apps {
		if app.Name == incoming.Name {
			return errors.Errorf("app %q imported from %q, but it was already imported from %q", incoming.Name, incoming.FromPath, app.FromPath)
		}
	}

	f.Apps = append(f.Apps, incoming)

	return nil
}

func (f *File) mergeEnvironment(env *EnvironmentConfig) error {

	if env.Name == "all" {
		for _, e := range f.Environments {
			e.Merge(env)
		}
		return nil
	}

	for _, e := range f.Environments {
		if e.Name == env.Name {
			e.Merge(env)
			return nil
		}
	}

	f.Environments = append(f.Environments, env)

	return nil
}

func (f *File) GetEnvironmentConfig(name string) *EnvironmentConfig {
	for _, e := range f.Environments {
		if e.Name == name {
			return e
		}
	}

	panic(fmt.Sprintf("no environment named %q", name))
}
