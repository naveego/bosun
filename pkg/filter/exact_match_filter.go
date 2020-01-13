package filter

import "github.com/naveego/bosun/pkg/util/stringsn"

type ExactMatchConfig map[string][]string

type ExactMatchArgs map[string]string

func (e ExactMatchConfig) Matches(args ExactMatchArgs) bool {
	if len(e) == 0 {
		return true
	}
	for key, matchers := range e {
		target := args[key]
		if !stringsn.Contains(target, matchers) {
			return false
		}
	}
	return true
}

type ExactMatchArgsContainer interface {
	GetExactMatchArgs() ExactMatchArgs
	WithExactMatchArgs(args ExactMatchArgs) interface{}
}
