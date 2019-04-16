package bosun

import (
	"github.com/fatih/color"
	"reflect"
	"regexp"
	"strings"
)

const (
	FilterKeyName    = "name"
	FilterKeyBranch  = "branch"
	FilterKeyCommit  = "commit"
	FilterKeyVersion = "version"
	FilterKeyPath    = "path"
)

type Filter struct {
	Key      string
	Value    string
	Operator string
}

type LabelValue interface {
	Value() string
}

type LabelThunk func() string
func (l LabelThunk) Value() string { return l() }

type Labels map[string]LabelValue

type LabelString string
func (l LabelString) Value() string { return string(l) }

func (l *Labels) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var arr []string
	err := unmarshal(&arr)
	proxy := map[string]string{}
	if err == nil {
		for _, name := range arr {
			proxy[name] = "true"
		}
	} else {
		err = unmarshal(proxy)
	}
	out := Labels{}

	for k, v := range proxy {
		out[k] = LabelString(v)
	}
	*l = out
	return err
}

func FilterMatchAll() []Filter {
	return []Filter{{Key: FilterKeyName, Value: ".*", Operator: "?="}}
}

func FiltersFromNames(names ...string) []Filter {
	var out []Filter
	for _, name := range names {
		out = append(out,
			Filter{Key: FilterKeyName, Value: name, Operator: "="},
			Filter{Key: FilterKeyPath, Value: name, Operator: "="},
		)
	}
	return out
}

func FiltersFromArgs(args ...string) []Filter {
	var out []Filter
	for _, arg := range args {
		matches := parseFilterRE.FindStringSubmatch(arg)
		if len(matches) != 4 {
			color.Red("Invalid filter: %s", arg)
			continue
		}
		out = append(out, Filter{Key: matches[1], Value: matches[3], Operator: matches[2]})
	}
	return out
}

func FiltersFromAppLabels(args ...string) []Filter {
	var out []Filter
	for _, arg := range args {
		segs := strings.Split(arg, "=")
		switch len(segs) {
		case 1:
			out = append(out, Filter{Key: arg, Value: "true", Operator: "="})
		case 2:
			out = append(out, Filter{Key: segs[0], Value: segs[1], Operator: "="})
		}
	}
	return out
}

var parseFilterRE = regexp.MustCompile(`(\w+)(\W+)(\w+)`)

func ApplyFilter(from interface{}, includeMatched bool, filters []Filter) interface{} {
	fromValue := reflect.ValueOf(from)
	var out reflect.Value

	switch fromValue.Kind() {
	case reflect.Map:
		out = reflect.MakeMap(fromValue.Type())
		keys := fromValue.MapKeys()
		for _, key := range keys {
			value := fromValue.MapIndex(key)
			labelled, ok := value.Interface().(Labelled)
			var matched bool
			for _, filter := range filters {
				if ok {
					matched := MatchFilter(labelled, filter)
					if matched {
						break
					}
				}
			}
			if matched == includeMatched {
				out.SetMapIndex(key, value)
			}
		}
	case reflect.Slice:
		length := fromValue.Len()
		out = reflect.MakeSlice(fromValue.Type(), 0, fromValue.Len())
		for i := 0; i < length; i++ {
			value := fromValue.Index(i)
			labelled, ok := value.Interface().(Labelled)
			var matched bool
			for _, filter := range filters {
				if ok {
					matched = MatchFilter(labelled, filter)
					if matched {
						break
					}
				}
			}
			if matched == includeMatched {
				out = reflect.Append(out, value)
			}
		}
	}

	return out.Interface()
}

func ExcludeMatched(from []interface{}, filters []Filter) []interface{} {
	var out []interface{}
	for _, item := range from {
		labelled, ok := item.(Labelled)
		for _, filter := range filters {
			if ok {
				matched := MatchFilter(labelled, filter)
				if !matched {
					out = append(out, item)
					break
				}
			}
		}
	}
	return out
}

func MatchFilter(labelled Labelled, filter Filter) bool {
	labels := labelled.Labels()

	switch filter.Operator {
	case "=", "==", "":
		value, ok := labels[filter.Key]
		if ok {
			return value.Value() == filter.Value
		}
	case "?=":
		re, err := regexp.Compile(filter.Value)
		if err != nil {
			color.Red("Invalid regex in filter %s?=%s: %s", filter.Key, filter.Value, err)
			return false
		}
		value, ok := labels[filter.Key]
		if ok {
			return re.MatchString(value.Value())
		} else {
			return false
		}
	}

	return false
}

type Labelled interface {
	Labels() Labels
}
