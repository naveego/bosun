package core

import (
	"fmt"
	"strings"
)

type NamespaceRole string

const NamespaceRoleDefault NamespaceRole = "default"

type NamespaceRoles []NamespaceRole

func NamespaceRolesFromStrings(ss []string) NamespaceRoles {
	var out NamespaceRoles
	for _, s := range ss {
		out = append(out, NamespaceRole(s))
	}
	return out
}

func (e NamespaceRoles) MarshalYAML() (interface{}, error) {
	if e == nil {
		return nil, nil
	}
	var out []string
	for _, e := range e {
		out = append(out, string(e))
	}
	return out, nil
}

func (e *NamespaceRoles) UnmarshalYAML(unmarshal func(interface{}) error) error {

	var p string
	var out []NamespaceRole
	err := unmarshal(&p)

	if err == nil {
		segs := strings.Split(p, ",")
		for _, s := range segs {
			out = append(out, NamespaceRole(s))
		}

		*e = out
		return nil
	}

	err = unmarshal(&out)

	*e = out

	return err
}

// Accepts returns true if the list of roles is empty or if it contains the role.
func (e NamespaceRoles) Accepts(role NamespaceRole) bool {
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

func (e NamespaceRoles) Contains(role NamespaceRole) bool {
	for _, r := range e {
		if r == role {
			return true
		}
	}
	return false
}

func (e NamespaceRoles) String() string {
	return fmt.Sprintf("%#v", e)
}

type NamespaceRoler interface {
	NamespaceRole() NamespaceRole
}

type NamespaceRoleDefinition struct {
	Name        NamespaceRole `yaml:"name"`
	Description string        `yaml:"description"`
}
