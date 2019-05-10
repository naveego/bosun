package bosun

import (
	"github.com/imdario/mergo"
	"github.com/pkg/errors"
)

type ValueSet struct {
	ConfigShared `yaml:",inline"`
	Dynamic      map[string]*CommandValue `yaml:"dynamic,omitempty" json:"dynamic,omitempty"`
	Files        []string                 `yaml:"files,omitempty" json:"files,omitempty"`
	Static       Values                   `yaml:"static,omitempty" json:"static,omitempty"`
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
			v1.Set = map[string]*CommandValue{}
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
