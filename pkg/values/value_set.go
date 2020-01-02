package values

import (
	"github.com/imdario/mergo"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"strings"
)

const ValueSetAll = "all"

type ValueSet struct {
	core.ConfigShared `yaml:",inline"`
	Dynamic           map[string]*command.CommandValue `yaml:"dynamic,omitempty" json:"dynamic,omitempty"`
	Files             []string                         `yaml:"files,omitempty" json:"files,omitempty"`
	Static            Values                           `yaml:"static,omitempty" json:"static,omitempty"`
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

// Combine returns a new ValueSet with the values from
// other added after (and/or overwriting) the values from this instance)
func (a ValueSet) Combine(other ValueSet) ValueSet {

	// clone the valueSet to ensure we don't mutate `a`
	y, _ := yaml.Marshal(a)
	var out ValueSet
	_ = yaml.Unmarshal(y, &out)

	// clone the other valueSet to ensure we don't capture items from it
	y, _ = yaml.Marshal(other)
	_ = yaml.Unmarshal(y, &other)

	if out.Dynamic == nil {
		out.Dynamic = map[string]*command.CommandValue{}
	}
	if out.Static == nil {
		out.Static = Values{}
	}

	out.Files = append(out.Files, other.Files...)

	out.Static.Merge(other.Static)

	for k, v := range other.Dynamic {
		out.Dynamic[k] = v
	}

	return out
}

// ValueSetMap is a map of (possibly multiple) names
// to ValueSets. The the keys can be single names (like "red")
// or multiple, comma-delimited names (like "red,green").
// Use ExtractValueSetByName to get a merged ValueSet
// comprising the ValueSets under each key which contains that name.
type ValueSetMap map[string]ValueSet

// ExtractValueSetByName returns a merged ValueSet
// comprising the ValueSets under each key which contains the provided names.
// ValueSets with the same name are merged in order from least specific key
// to most specific, so values under the key "red" will overwrite values under "red,green",
// which will overwrite values under "red,green,blue", and so on. Then the
// ValueSets with each name are merged in the order the names were provided.
func (a ValueSetMap) ExtractValueSetByName(name string) ValueSet {

	out := ValueSet{}

	// More precise values should override less precise values
	// We assume no ValueSetMap will ever have more than 10
	// named keys in it.
	priorities := make([][]ValueSet, 10, 10)

	for k, v := range a {
		keys := strings.Split(k, ",")
		for _, k2 := range keys {
			if k2 == name {
				priorities[len(keys)] = append(priorities[len(keys)], v)
			}
		}
	}

	for i := len(priorities) - 1; i >= 0; i-- {
		for _, v := range priorities[i] {
			out = out.Combine(v)
		}
	}

	return out
}

// ExtractValueSetByName returns a merged ValueSet
// comprising the ValueSets under each key which contains the provided names.
// The process starts with the values under the key "all", then
// ValueSets with the same name are merged in order from least specific key
// to most specific, so values under the key "red" will overwrite values under "red,green",
// which will overwrite values under "red,green,blue", and so on. Then the
// ValueSets with each name are merged in the order the names were provided.
func (a ValueSetMap) ExtractValueSetByNames(names ...string) ValueSet {

	out := a.ExtractValueSetByName(ValueSetAll)

	for _, name := range names {
		vs := a.ExtractValueSetByName(name)
		out = out.Combine(vs)
	}

	return out
}

// CanonicalizedCopy returns a copy of this ValueSetMap with
// only single-name keys, by de-normalizing any multi-name keys.
// Each ValueSet will have its name set to the value of the name it's under.
func (a ValueSetMap) CanonicalizedCopy() ValueSetMap {

	out := ValueSetMap{
		ValueSetAll: ValueSet{},
	}

	for k := range a {
		names := strings.Split(k, ",")
		for _, name := range names {
			out[name] = ValueSet{}
		}
	}

	for name := range out {
		vs := a.ExtractValueSetByName(name)
		vs.Name = name
		out[name] = vs
	}
	// don't write out the "all" value set, it's integrated into the others
	delete(out, ValueSetAll)

	return out
}

// WithFilesLoaded resolves all file system dependencies into static values
// on this instance, then clears those dependencies.
func (a ValueSet) WithFilesLoaded(pathResolver core.PathResolver) (ValueSet, error) {

	out := ValueSet{
		Static: a.Static.Clone(),
	}

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
