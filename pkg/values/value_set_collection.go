package values

import (
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/util/stringsn"
	"github.com/pkg/errors"
	"go4.org/sort"
	"strings"
)

// ValueSetCollection is a map of (possibly multiple) names
// to ValueSets. The the keys can be single names (like "red")
// or multiple, comma-delimited names (like "red,green").
// Use ExtractValueSetByName to get a merged ValueSet
// comprising the ValueSets under each key which contains that name.
type ValueSetCollection struct {
	DefaultValues ValueSet  `yaml:"defaults"`
	ValueSets     ValueSets `yaml:"custom"`
}

func (v *ValueSetCollection) SetFromPath(fromPath string) {
	v.DefaultValues.SetFromPath(fromPath)
	for i, vs := range v.ValueSets {
		vs.SetFromPath(fromPath)
		v.ValueSets[i] = vs
	}
}

func NewValueSetCollection() ValueSetCollection {
	return ValueSetCollection{}
}

func (v ValueSetCollection) MarshalYAML() (interface{}, error) {

	v.DefaultValues.Name = "default"
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
			for _, key := range keys {
				v.Roles = append(v.Roles, core.EnvironmentRole(key))
			}
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
		if v.Name == name {
			return v
		}
	}

	return out
}

func (v ValueSetCollection) ExtractValueSetByRole(role core.EnvironmentRole) ValueSet {

	out := ValueSet{}

	for _, v := range v.ValueSets {
		for _, r := range v.Roles {
			if r == role {
				out = out.WithValues(v)
			}
		}
	}

	return out
}

func (v ValueSetCollection) FindValueSetForRole(role core.EnvironmentRole) (ValueSet, error) {

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

type ExtractValueSetArgs struct {
	Names      []string
	Roles      []core.EnvironmentRole
	ExactMatch map[string]string
}

func (v ValueSetCollection) ExtractValueSetByNames(names ...string) ValueSet {

	return v.ExtractValueSet(ExtractValueSetArgs{
		Names: names,
	})
}

func (v ValueSetCollection) ExtractValueSetByRoles(roles ...core.EnvironmentRole) ValueSet {
	return v.ExtractValueSet(ExtractValueSetArgs{Roles: roles})
}

func (v ValueSetCollection) ExtractValueSet(args ExtractValueSetArgs) ValueSet {

	out := v.DefaultValues.Clone()

	var matched ValueSets

	if len(args.Roles) == 0 {
		if roleArgs, ok := args.ExactMatch[core.KeyEnvironmentRole]; ok {
			args.Roles = core.EnvironmentRoles{core.EnvironmentRole(roleArgs)}
		}
	}

	for _, candidate := range v.ValueSets {
		if len(args.Names) > 0 {
			if !stringsn.Contains(candidate.Name, args.Names) {
				pkg.Log.WithField("@value_set", candidate.Name).WithField("name", candidate.Name).WithField("requested_names", args.Names).Trace("ExtractValueSet: Skipping because name was not requested.")
				continue
			}
		}
		if len(args.Roles) > 0 {
			matchedRole := false
			for _, role := range args.Roles {
				for _, roleTest := range candidate.Roles {
					if role == roleTest {
						matchedRole = true
					}
				}
			}
			if !matchedRole {
			pkg.Log.WithField("@value_set", candidate.Name).WithField("roles", candidate.Roles).WithField("requested_roles", args.Roles).Trace("ExtractValueSet: Skipping because role was not requested.")
				continue
			}
		}

		if !candidate.ExactMatchFilters.Matches(args.ExactMatch) {
			pkg.Log.WithField("@value_set", candidate.Name).WithField("filter", candidate.ExactMatchFilters).WithField("filter_args", args.ExactMatch).Trace("ExtractValueSet: Skipping because filters did not match.")
			continue
		}

		matched = append(matched, candidate)
	}

	for _, m := range matched {
		out = out.WithValues(m)
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
			order = stringsn.AppendIfNotPresent(order, string(role))
			var c ValueSet
			var ok bool
			if c, ok = canonical[string(role)]; !ok {
				c = ValueSet{
					Roles:   []core.EnvironmentRole{role},
					Static:  Values{},
					Dynamic: map[string]*command.CommandValue{},
				}
			}
			c = c.WithValues(v)
			canonical[string(role)] = c
		}
	}

	for _, role := range order {
		out.ValueSets = append(out.ValueSets, canonical[role])
	}

	return out
}
