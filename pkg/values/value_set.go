package values

import (
	"github.com/imdario/mergo"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/util/stringsn"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"go4.org/sort"
	"strings"
)

const ValueSetAll = "all"

type ValueSet struct {
	core.ConfigShared `yaml:",inline"`
	Roles             []string                         `yaml:"roles"`
	Dynamic           map[string]*command.CommandValue `yaml:"dynamic,omitempty" json:"dynamic,omitempty"`
	Files             []string                         `yaml:"files,omitempty" json:"files,omitempty"`
	Static            Values                           `yaml:"static,omitempty" json:"static,omitempty"`
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
	return out
}

// WithValues returns a new ValueSet with the values from
// other added after (and/or overwriting) the values from this instance)
func (a ValueSet) WithValues(other ValueSet) ValueSet {

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

// ValueSetCollection is a map of (possibly multiple) names
// to ValueSets. The the keys can be single names (like "red")
// or multiple, comma-delimited names (like "red,green").
// Use ExtractValueSetByName to get a merged ValueSet
// comprising the ValueSets under each key which contains that name.
type ValueSetCollection struct {
	DefaultValues ValueSet  `yaml:"defaults"`
	ValueSets     ValueSets `yaml:"custom"`
}

func (v ValueSetCollection) MarshalYAML() (interface{}, error) {

	m := append([]ValueSet{v.DefaultValues}, v.ValueSets...)
	return m, nil
}

func (v *ValueSetCollection) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var raw interface{}
	err := unmarshal(&raw)
	if err != nil {
		return err
	}
	switch raw.(type) {
	case map[interface{}]interface{}, map[string]interface{}:
		var m map[string]ValueSet
		err = unmarshal(&m)
		if err != nil {
			return errors.Wrap(err, "interpreting as map of ValueSet")
		}

		var out ValueSets

		for k, v := range m {
			keys := strings.Split(k, ",")
			v.Roles = keys
			out = append(out, v)
		}

		sort.Sort(out)

		*v = ValueSetCollection{
			ValueSets: out,
		}
		return nil
	case []interface{}:
		var s []ValueSet
		err = unmarshal(&s)
		if err != nil {
			return errors.Wrap(err, "interpreting as slice of ValueSetWithRoles")
		}
		*v = ValueSetCollection{}
		for _, vs := range s {
			if vs.Name == "default" {
				v.DefaultValues = vs
			} else {
				v.ValueSets = append(v.ValueSets, vs)
			}
		}
		return nil
	default:
		return errors.Errorf("unrecognized type %T", raw)
	}
}

// ExtractValueSetByName returns a merged ValueSet
// comprising the ValueSets under each key which contains the provided names.
// ValueSets with the same name are merged in order from least specific key
// to most specific, so values under the key "red" will overwrite values under "red,green",
// which will overwrite values under "red,green,blue", and so on. Then the
// ValueSets with each name are merged in the order the names were provided.
func (v ValueSetCollection) ExtractValueSetByName(name string) ValueSet {

	out := ValueSet{}

	for _, v := range v.ValueSets {
		for _, role := range v.Roles {
			if role == name {
				out = out.WithValues(v)
			}
		}
	}

	return out
}

func (v ValueSetCollection) FindValueSetForRole(role string) (ValueSet, error) {

	var out []ValueSet
	for _, vs := range v.ValueSets {
		for _, r := range vs.Roles {
			if r == role {
				out = append(out, vs)
			}
		}
	}

	switch len(out) {
	case 0:
		return ValueSet{}, errors.Errorf("no value set with role %q", role)
	case 1:
		return out[0], nil
	default:
		return ValueSet{}, errors.Errorf("found %d value sets with role %q (has this container be canonicalized?)", len(out), role)
	}

}

// ExtractValueSetByName returns a merged ValueSet
// comprising the ValueSets under each key which contains the provided names.
// The process starts with the values under the key "all", then
// ValueSets with the same name are merged in order from least specific key
// to most specific, so values under the key "red" will overwrite values under "red,green",
// which will overwrite values under "red,green,blue", and so on. Then the
// ValueSets with each name are merged in the order the names were provided.
func (v ValueSetCollection) ExtractValueSetByNames(names ...string) ValueSet {

	out := v.ExtractValueSetByName(ValueSetAll)

	for _, name := range names {
		vs := v.ExtractValueSetByName(name)
		out = out.WithValues(vs)
	}

	return out
}

// CanonicalizedCopy returns a copy of this ValueSetCollection with
// only single-role entries, by de-normalizing any multi-role entries.
// Each ValueSet will have its name set to the value of the name it's under.
func (v ValueSetCollection) CanonicalizedCopy() ValueSetCollection {

	out := ValueSetCollection{
		DefaultValues: v.DefaultValues.Clone(),
		ValueSets:     ValueSets{},
	}

	canonical := map[string]ValueSet{}

	var order []string

	for _, v := range v.ValueSets {
		for _, role := range v.Roles {
			order = stringsn.AppendIfNotPresent(order, role)
			var c ValueSet
			var ok bool
			if c, ok = canonical[role]; !ok {
				c = ValueSet{
					Roles:   []string{role},
					Static:  Values{},
					Dynamic: map[string]*command.CommandValue{},
				}
			}
			c = c.WithValues(v)
			canonical[role] = c
		}
	}

	for _, role := range order {
		out.ValueSets = append(out.ValueSets, canonical[role])
	}

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
