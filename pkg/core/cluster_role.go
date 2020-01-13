package core

import (
	"fmt"
	"strings"
)

type ClusterRole string

const ClusterRoleDefault ClusterRole = "default"

type ClusterRoles []ClusterRole

func ClusterRolesFromStrings(ss []string) ClusterRoles {
	var out ClusterRoles
	for _, s := range ss {
		out = append(out, ClusterRole(s))
	}
	return out
}

func (e ClusterRoles) MarshalYAML() (interface{}, error) {
	if e == nil {
		return nil, nil
	}
	var out []string
	for _, e := range e {
		out = append(out, string(e))
	}
	return out, nil
}

func (e *ClusterRoles) UnmarshalYAML(unmarshal func(interface{}) error) error {

	var p string
	var out []ClusterRole
	err := unmarshal(&p)

	if err == nil {
		segs := strings.Split(p, ",")
		for _, s := range segs {
			out = append(out, ClusterRole(s))
		}

		*e = out
		return nil
	}

	err = unmarshal(&out)

	*e = out

	return err
}

// Accepts returns true if the list of roles is empty or if it contains the role.
func (e ClusterRoles) Accepts(role ClusterRole) bool {
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

func (e ClusterRoles) Contains(role ClusterRole) bool {
	for _, r := range e {
		if r == role {
			return true
		}
	}
	return false
}

func (e ClusterRoles) String() string {
	return fmt.Sprintf("%#v", e)
}

func (e ClusterRoles) Strings() []string {
	var out []string
	for _, x := range e {
		out = append(out, string(x))
	}
	return out
}

type ClusterRoler interface {
	ClusterRole() ClusterRole
}

type ClusterRoleDefinition struct {
	Name        ClusterRole `yaml:"name"`
	Description string      `yaml:"description"`
}
