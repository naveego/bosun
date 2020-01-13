package values

import (
	"github.com/imdario/mergo"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/templating"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
)

const ValueSetAll = "all"

type ValueSet struct {
	core.ConfigShared   `yaml:",inline"`
	Source              string                           `yaml:"source,omitempty"`
	StaticAttributions  Values                           `yaml:"staticAttributions,omitempty"`
	DynamicAttributions Values                           `yaml:"dynamicAttributions,omitempty"`
	Roles               []core.EnvironmentRole           `yaml:"roles,flow,omitempty"`
	ExactMatchFilters   filter.ExactMatchConfig          `yaml:"exactMatchFilters,omitempty"`
	Dynamic             map[string]*command.CommandValue `yaml:"dynamic,omitempty" json:"dynamic,omitempty"`
	Files               []string                         `yaml:"files,omitempty" json:"files,omitempty"`
	Static              Values                           `yaml:"static,omitempty" json:"static,omitempty"`
}

func (a *ValueSet) MarshalYAML() (interface{}, error) {
	type proxy ValueSet
	px := proxy(*a)

	if len(px.StaticAttributions) == 0 {
		px.StaticAttributions = nil
	}
	if len(px.DynamicAttributions) == 0 {
		px.DynamicAttributions = nil
	}

	return px, nil
}

func NewValueSet() ValueSet {
	return ValueSet{
		Dynamic: map[string]*command.CommandValue{},
		Static:  Values{},
	}
}

type ValueSets []ValueSet

func (v ValueSets) Len() int {
	return len(v)
}

func (v ValueSets) Less(i, j int) bool {
	li := len(v[i].Roles)
	lj := len(v[j].Roles)
	// secondary sort by role names
	if li == lj && li > 0 {
		ri := v[i].Roles[0]
		rj := v[j].Roles[0]
		return ri < rj
	}

	// sets with more roles go before sets with fewer roles
	return lj < li
}

func (v ValueSets) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

func (a *ValueSet) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var m map[string]interface{}
	err := unmarshal(&m)
	if err != nil {
		return errors.WithStack(err)
	}
	if _, ok := m["set"]; ok {
		// is v1
		var v1 appValuesConfigV1
		err = unmarshal(&v1)
		if err != nil {
			return errors.WithStack(err)
		}
		if a == nil {
			*a = ValueSet{}
		}
		if v1.Static == nil {
			v1.Static = Values{}
		}
		if v1.Set == nil {
			v1.Set = map[string]*command.CommandValue{}
		}
		a.Files = v1.Files
		a.Static = v1.Static
		a.Dynamic = v1.Set
		// handle case where set AND dynamic both have values
		if v1.Dynamic != nil {
			err = mergo.Map(a.Dynamic, v1.Dynamic)
		}
		return err
	}

	type proxy ValueSet
	var p proxy
	err = unmarshal(&p)
	if err != nil {
		return errors.WithStack(err)
	}

	*a = ValueSet(p)

	return nil
}

type appValuesConfigV1 struct {
	Set     map[string]*command.CommandValue `yaml:"set,omitempty" json:"set,omitempty"`
	Dynamic map[string]*command.CommandValue `yaml:"dynamic,omitempty" json:"dynamic,omitempty"`
	Files   []string                         `yaml:"files,omitempty" json:"files,omitempty"`
	Static  Values                           `yaml:"static,omitempty" json:"static,omitempty"`
}

func (a ValueSet) Clone() ValueSet {
	s, _ := yaml.Marshal(a)
	var out ValueSet
	_ = yaml.Unmarshal(s, &out)
	if out.DynamicAttributions == nil {
		out.DynamicAttributions = Values{}
	}
	if out.StaticAttributions == nil {
		out.StaticAttributions = Values{}
	}
	out.FileSaver = a.FileSaver
	out.FromPath = a.FromPath

	return out
}

func (a ValueSet) WithSource(source string) ValueSet {
	a.Source = source
	return a
}

// Returns a value set with the source set if it wasn't set before.
func (a ValueSet) WithDefaultSource(source string) ValueSet {
	if a.Source == "" {
		a.Source = source
	}
	return a
}

// WithValues returns a new ValueSet with the values from
// other added after (and/or overwriting) the values from this instance)
func (a ValueSet) WithValues(other ValueSet) ValueSet {

	// clone the valueSet to ensure we don't mutate `a`
	out := a.Clone()

	if out.StaticAttributions == nil {
		out.StaticAttributions = Values{}
	}
	if out.DynamicAttributions == nil {
		out.DynamicAttributions = Values{}
	}

	// clone the other valueSet to ensure we don't capture items from it
	other = other.Clone()

	if out.Dynamic == nil {
		out.Dynamic = map[string]*command.CommandValue{}
	}
	if out.Static == nil {
		out.Static = Values{}
	}

	attribution := other.Source
	if attribution == "" {
		attribution = other.Name
	}
	if attribution == "" {
		attribution = other.FromPath
	}

	out.Files = append(out.Files, other.Files...)

	out.StaticAttributions.Merge(other.StaticAttributions)
	out.DynamicAttributions.Merge(other.DynamicAttributions)

	out.Static.MergeWithAttribution(other.Static, attribution, out.StaticAttributions)

	for k, v := range other.Dynamic {
		out.Dynamic[k] = v
		out.DynamicAttributions[k] = attribution
	}

	return out

}

func (a ValueSet) WithValueSetAtPath(path string, value interface{}, attribution string) (ValueSet, error) {
	out := a.Clone()

	err := out.Static.SetAtPath(path, value)
	if err != nil {
		return out, err
	}

	_ = out.StaticAttributions.SetAtPath(path, attribution)

	return out, nil
}

// WithFilesLoaded resolves all file system dependencies into static values
// on this instance, then clears those dependencies.
func (a ValueSet) WithFilesLoaded(pathResolver core.PathResolver) (ValueSet, error) {

	out := a.Clone()

	mergedValues := Values{}

	// merge together values loaded from files
	for _, file := range a.Files {
		file = pathResolver.ResolvePath(file, "VALUE_SET", a.Name)
		valuesFromFile, err := ReadValuesFile(file)
		if err != nil {
			return out, errors.Errorf("reading values file %q: %s", file, err)
		}
		mergedValues.Merge(valuesFromFile)
	}

	// make sure any existing static values are merged OVER the values from the file
	mergedValues.Merge(out.Static)
	out.Static = mergedValues

	out.Dynamic = a.Dynamic

	return out, nil
}

// WithFilesLoaded resolves all file system dependencies into static values
// on this instance, then clears those dependencies.
func (a ValueSet) WithDynamicValuesResolved(ctx command.ExecutionContext) (ValueSet, error) {

	out := a.Clone()

	for k, v := range a.Dynamic {
		value, err := v.Resolve(ctx)
		if err != nil {
			return out, errors.Errorf("resolving dynamic values for app %q for key %q: %s", a.Name, k, err)
		}

		err = out.Static.SetAtPath(k, value)
		if err != nil {
			return out, errors.Errorf("merging dynamic values for app %q for key %q: %s", a.Name, k, err)
		}
	}

	return out, nil
}

// WithFilesLoaded resolves all file system dependencies into static values
// on this instance, then clears those dependencies.
func (a ValueSet) WithInternalTemplatesResolved() (ValueSet, error) {

	y, _ := yaml.MarshalString(a)

	rendered, err := templating.RenderTemplate(y, a.Static)

	if err != nil {
		return a, errors.Wrapf(err, "rendering internal templates")
	}

	var out ValueSet
	err = yaml.UnmarshalString(rendered, &out)

	return out, err
}
