package filter

import (
	"fmt"
	"github.com/pkg/errors"
	"regexp"
)

type Operator func(v string) bool

const (
	OperatorKey      = ""
	OperatorEqual    = "=="
	OperatorNotEqual = "!="
	OperatorRegex    = "?="
)

type OperatorFactory func(key, value string) (Operator, error)

var Operators = map[string]OperatorFactory{
	"==": func(key, value string) (Operator, error) {
		return func(v string) bool {
			return value == v
		}, nil
	},
	"!=": func(key, value string) (Operator, error) {
		return func(v string) bool {
			return value != v
		}, nil
	},
	"?=": func(key, value string) (operator Operator, e error) {
		re, err := regexp.Compile(value)
		if err != nil {
			return nil, errors.Errorf("bad regex in %s?=%s: %s", key, value, err)
		}
		return func(value string) bool {
			return re.MatchString(value)
		}, nil
	},
}

type SimpleFilter struct {
	Raw      string
	Key      string
	Value    string
	Operator Operator
}

func (s SimpleFilter) IsMatch(l Labels) bool {
	if label, ok := l[s.Key]; ok {
		labelValue := label.Value()
		return s.Operator(labelValue)
	}
	return false
}

// FilterFromOperator creates a new filter with the given operator.
func FilterFromOperator(label, key string, operator Operator) Filter {
	return SimpleFilter{
		Raw:      label,
		Key:      key,
		Operator: operator,
	}
}

// MustParse is like Parse but panics if parse fails.
func MustParse(parts ...string) Filter {
	f, err := Parse(parts...)
	if err != nil {
		panic(err)
	}
	return f
}

// Parse returns a filter based on key, value, and operator
// Argument patterns:
// `"key"` - Will match if the key is found
// `"keyOPvalue"` - Where OP is one or more none-word characters, will check the value of key against the provided value using the operator
// `"key", "op", "value"` - Will check the value at key against the value using the operator
func Parse(parts ...string) (Filter, error) {
	switch len(parts) {
	case 0:
		return nil, errors.New("at least one part required")
	case 1:
		matches := simpleFilterParseRE.FindStringSubmatch(parts[0])
		if len(matches) == 4 {
			return newFilterFromOperator(matches[1], matches[2], matches[3])
		}
		return SimpleFilter{
			Raw: parts[0],
			Key: parts[0],
			Operator: func(value string) bool {
				return true
			},
		}, nil

	case 3:
		return newFilterFromOperator(parts[0], parts[1], parts[2])
	default:
		return nil, errors.Errorf("invalid parts %#v", parts)
	}
}

func newFilterFromOperator(k, op, v string) (Filter, error) {

	if factory, ok := Operators[op]; ok {
		fn, err := factory(k, v)
		if err != nil {
			return nil, err
		}
		return SimpleFilter{
			Raw:      fmt.Sprintf("%s%s%s", k, op, v),
			Key:      k,
			Value:    v,
			Operator: fn,
		}, nil
	}

	return nil, errors.Errorf("no operator factory registered for operator %q", op)

}

var simpleFilterParseRE = regexp.MustCompile(`(\w+)(\W{1,2})(.*)`)
