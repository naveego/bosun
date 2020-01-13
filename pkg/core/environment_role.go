package core

import (
	"fmt"
	"strings"
)

type EnvironmentRole string

func (e EnvironmentRole) String() string {
	return string(e)
}

type EnvironmentRoles []EnvironmentRole

func (e EnvironmentRoles) MarshalYAML() (interface{}, error) {
	if e == nil {
		return nil, nil
	}
	var out []string
	for _, e := range e {
		out = append(out, string(e))
	}
	return out, nil
}

func (e *EnvironmentRoles) UnmarshalYAML(unmarshal func(interface{}) error) error {

	var p string
	var out []EnvironmentRole
	err := unmarshal(&p)

	if err == nil {
		segs := strings.Split(p, ",")
		for _, s := range segs {
			out = append(out, EnvironmentRole(s))
		}

		*e = out
		return nil
	}

	err = unmarshal(&out)

	*e = out

	return err
}

// Accepts returns true if the list of roles is empty or if it contains the role.
func (e EnvironmentRoles) Accepts(role EnvironmentRole) bool {
	if len(e) == 0 {
		return true
	}
	for _, r := range e {
		if r == role {
			return true
		}
	}
	return false
}

func (e EnvironmentRoles) Contains(role EnvironmentRole) bool {
	for _, r := range e {
		if r == role {
			return true
		}
	}
	return false
}

func (e EnvironmentRoles) Strings() []string {
	var out []string
	for _, x := range e {
		out = append(out, string(x))
	}
	return out
}

func (e EnvironmentRoles) String() string {
	return fmt.Sprintf("%#v", e)
}

type EnvironmentRoler interface {
	EnvironmentRole() EnvironmentRole
}

type EnvironmentRoleDefinition struct {
	Name        EnvironmentRole `yaml:"name"`
	Description string          `yaml:"description"`
}
