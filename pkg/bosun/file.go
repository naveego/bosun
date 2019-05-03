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
	Repos        []RepoConfig           `yaml:"repos" json:"repos"`
	FromPath     string                 `yaml:"fromPath" json:"fromPath"`
	Config       *Workspace             `yaml:"-" json:"-"`
	Releases     []*ReleaseConfig       `yaml:"releases,omitempty" json:"releases"`
	Tools        []ToolDef              `yaml:"tools,omitempty" json:"tools"`
	TestSuites   []*E2ESuiteConfig      `yaml:"testSuites,omitempty" json:"testSuites,omitempty"`
	Scripts      []*Script              `yaml:"scripts,omitempty" json:"scripts,omitempty"`

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

func (f *File) SetFromPath(path string) {

	f.FromPath = path

	for _, e := range f.Environments {
		e.SetFromPath(path)
	}

	for _, m := range f.Apps {
		m.SetFragment(f)
	}

	for _, m := range f.AppRefs {
		m.FromPath = path
	}

	for _, m := range f.Releases {
		m.SetParent(f)
	}

	for _, s := range f.Scripts {
		s.SetFromPath(path)
	}

	for i := range f.Tools {
		f.Tools[i].FromPath = f.FromPath
	}

	for i := range f.TestSuites {
		f.TestSuites[i].SetFromPath(f.FromPath)
	}
}

func (f *File) Merge(other *File) error {

	f.merged = true

	for _, otherEnv := range other.Environments {
		err := f.mergeEnvironment(otherEnv)
		if err != nil {
			return errors.Wrap(err, "merge environment")
		}
	}

	if f.AppRefs == nil {
		f.AppRefs = make(map[string]*Dependency)
	}

	for k, other := range other.AppRefs {
		other.Name = k
		f.AppRefs[k] = other
	}

	for _, otherApp := range other.Apps {
		if err := f.mergeApp(otherApp); err != nil {
			return errors.Wrapf(err, "merge app %q", otherApp.Name)
		}
	}

	for _, release := range other.Releases {
		if err := f.mergeRelease(release); err != nil {
			return errors.Wrapf(err, "merge release %q", release.Name)
		}
	}

	for _, other := range other.Scripts {
		f.Scripts = append(f.Scripts, other)
	}

	for _, repo := range other.Repos {
		f.Repos = append(f.Repos, repo)
	}

	f.TestSuites = append(f.TestSuites, other.TestSuites...)
	f.Tools = append(f.Tools, other.Tools...)

	return nil
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

	for _, release := range f.Releases {
		err = release.SaveBundle()
		if err != nil {
			return errors.Wrapf(err, "saving bundle for release %q", release.Name)
		}
	}

	return nil
}

var stripFromPath = regexp.MustCompile(`\s*fromPath:.*`)

func (f *File) mergeApp(incoming *AppRepoConfig) error {
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

func (f *File) mergeRelease(release *ReleaseConfig) error {
	for _, e := range f.Releases {
		if e.Name == release.Name {
			return errors.Errorf("already have a release named %q, from %q", release.Name, e.FromPath)

		}
	}

	f.Releases = append(f.Releases, release)
	return nil
}
