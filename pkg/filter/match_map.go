package filter

import (
	"fmt"
	"strings"
)

type MatchMapConfig map[string]MatchMapConfigValues

type MatchMapArgs map[string]string

type MatchMapConfigValue string

func (m MatchMapConfigValue) Matches(arg string) bool {
	s := string(m)
	if strings.HasPrefix(s, "!"){
		return arg != s[1:]
	}
	return arg == s
}

type MatchMapConfigValues []MatchMapConfigValue

func (e MatchMapArgs) String() string {
	return fmt.Sprintf("%#v", map[string]string(e))
}


func (e MatchMapConfig) Matches(args MatchMapArgs) bool {
	if len(e) == 0 {
		return true
	}
	for key, matchers := range e {
		target := args[key]
		matched := false
		for _, matcher := range matchers {
			matched = matcher.Matches(target)
			if matched {
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func MatchMapConfigValuesFromStrings(ss []string) MatchMapConfigValues {
	var out MatchMapConfigValues
	for _, s := range ss {
		out = append(out, MatchMapConfigValue(s))
	}
	return out
}

type MatchMapArgContainer interface {
	GetMatchMapArgs() MatchMapArgs
	WithMatchMapArgs(args MatchMapArgs) interface{}
}
