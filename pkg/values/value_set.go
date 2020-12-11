package values

import (
	"fmt"
	"github.com/imdario/mergo"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/filter"
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
	ExactMatchFilters   filter.MatchMapConfig            `yaml:"exactMatchFilters,omitempty"`
	Dynamic             map[string]*command.CommandValue `yaml:"dynamic,omitempty" json:"dynamic,omitempty"`
	Files               []string                         `yaml:"files,omitempty" json:"files,omitempty"`
	Static              Values                           `yaml:"static,omitempty" json:"static,omitempty"`
}

func (v *ValueSet) MarshalYAML() (interface{}, error) {
	type proxy ValueSet
	px := proxy(*v)

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

func (v *ValueSet) UnmarshalYAML(unmarshal func(interface{}) error) error {
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
		if v == nil {
			*v = ValueSet{}
		}
		if v1.Static == nil {
			v1.Static = Values{}
		}
		if v1.Set == nil {
			v1.Set = map[string]*command.CommandValue{}
		}
		v.Files = v1.Files
		v.Static = v1.Static
		v.Dynamic = v1.Set
		// handle case where set AND dynamic both have values
		if v1.Dynamic != nil {
			err = mergo.Map(v.Dynamic, v1.Dynamic)
		}
		return err
	}

	type proxy ValueSet
	var p proxy
	err = unmarshal(&p)
	if err != nil {
		return errors.WithStack(err)
	}

	*v = ValueSet(p)

	return nil
}

type appValuesConfigV1 struct {
	Set     map[string]*command.CommandValue `yaml:"set,omitempty" json:"set,omitempty"`
	Dynamic map[string]*command.CommandValue `yaml:"dynamic,omitempty" json:"dynamic,omitempty"`
	Files   []string                         `yaml:"files,omitempty" json:"files,omitempty"`
	Static  Values                           `yaml:"static,omitempty" json:"static,omitempty"`
}

func (v ValueSet) Clone() ValueSet {
	s, _ := yaml.Marshal(v)
	var out ValueSet
	_ = yaml.Unmarshal(s, &out)

	out = out.withTriviaFrom(v)
	return out
}

func (v ValueSet) withTriviaFrom(other ValueSet) ValueSet {
	if v.DynamicAttributions == nil {
		v.DynamicAttributions = Values{}
	}
	if v.StaticAttributions == nil {
		v.StaticAttributions = Values{}
	}
	v.FileSaver = other.FileSaver
	v.FromPath = other.FromPath

	return v
}

func (v ValueSet) WithSource(source string) ValueSet {
	v.Source = source

	return v.WithDefaultSource(source)
}

// Returns a value set with the source set if it wasn't set before.
func (v ValueSet) WithDefaultSource(source string) ValueSet {
	if v.Source == "" {
		v.Source = source
	}
	if v.StaticAttributions == nil {
		v.StaticAttributions = Values{}
	}
	v.StaticAttributions.Merge(v.Static.MakeAttributionValues(source))

	if v.DynamicAttributions == nil {
		v.DynamicAttributions = map[string]interface{}{}
	}
	for k := range v.Dynamic {
		v.DynamicAttributions[k] = source
	}
	return v
}

// WithValues returns a new ValueSet with the values from
// other added after (and/or overwriting) the values from this instance)
func (v ValueSet) WithValues(other ValueSet) ValueSet {

	// clone the valueSet to ensure we don't mutate `a`
	out := v.Clone()

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

	out.Static.Merge(other.Static)

	for k, cmd := range other.Dynamic {
		out.Dynamic[k] = cmd
		attr := other.DynamicAttributions[k]
		if attr == "" {
			attr = attribution
		}
		out.DynamicAttributions[k] = attr
	}

	return out

}

func (v ValueSet) WithValueSetAtPath(path string, value interface{}, attribution string) (ValueSet, error) {
	out := v.Clone()

	err := out.Static.SetAtPath(path, value)
	if err != nil {
		return out, err
	}

	_ = out.StaticAttributions.SetAtPath(path, attribution)

	return out, nil
}

// WithFilesLoaded returns a new ValueSet based on this one, but with all files loaded
// and merged UNDER existing static values.
func (v ValueSet) WithFilesLoaded(pathResolver core.PathResolver) (ValueSet, error) {

	out := v.Clone()

	// merge together values loaded from files
	for _, file := range v.Files {
		file = pathResolver.ResolvePath(file, "VALUE_SET", v.Name)
		valuesFromFile, err := ReadValuesFile(file)
		if err != nil {
			return out, errors.Errorf("reading values file %q: %s", file, err)
		}
		valueSetFromFile := ValueSet{
			Static: valuesFromFile,
		}
		out = out.WithValues(valueSetFromFile.WithSource(fmt.Sprintf("file imported by app: %s", file)))
	}

	// make sure any existing static values are merged OVER the values from the file
	out = out.WithValues(v)

	return out, nil
}

// WithDynamicValuesResolved returns a ValueSet based on this instance, but with
// all dynamic values resolved and merged into the static values, and with all values
// which contain go templates rendered into their final values using the static values
// as of when this method was called.
func (v ValueSet) WithDynamicValuesResolved(ctx command.ExecutionContext) (ValueSet, error) {

	y, _ := yaml.MarshalString(v)

	// Get the template values from the context
	templateValues := ctx.TemplateValues()
	// Merge the existing values into it so they can be used to format themselves
	templateValues.Values = Values(templateValues.Values).Merge(v.Static).ToMapStringInterface()

	rendered, err := templateValues.RenderInto(y)
	if err != nil {
		return v, err
	}

	var out ValueSet
	err = yaml.UnmarshalString(rendered, &out)

	if err != nil {
		return v, err
	}

	out = out.withTriviaFrom(v)

	for k, value := range out.Dynamic {

		if value.Script != "" {
			ctx.Log().Debugf("Resolving dynamic value %q using script:\n %s", k, value.Script)
		}

		resolved, err := value.Resolve(ctx)
		if err != nil {
			return out, errors.Errorf("resolving dynamic values for key %q: %s", k, err)
		}

		err = out.Static.SetAtPath(k, resolved)
		if err != nil {
			return out, errors.Errorf("merging dynamic values for key %q: %s", k, err)
		}
	}

	return out, nil
}
