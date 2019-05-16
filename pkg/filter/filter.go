package filter

import (
	"fmt"
	"reflect"
)

type Filter interface {
	fmt.Stringer
	IsMatch(l Labels) bool
}

type Labelled interface {
	GetLabels() Labels
}

type filterFunc struct {
	label string
	fn    func(l Labels) bool
}

func FilterFunc(label string, fn func(l Labels) bool) Filter {
	return filterFunc{label, fn}
}

func (f filterFunc) String() string {
	return f.label
}

func (f filterFunc) IsMatch(l Labels) bool {
	return f.fn(l)
}

func FilterMatchAll() Filter {
	return FilterFunc("all", func(l Labels) bool { return true })
}

func Include(from interface{}, filters ...Filter) interface{} {
	return applyFilters(newFilterable(from), filters, true).val.Interface()
}

func Exclude(from interface{}, filters ...Filter) interface{} {
	return applyFilters(newFilterable(from), filters, false).val.Interface()
}

func applyFilters(f filterable, filters []Filter, include bool) filterable {
	var after filterable
	switch f.val.Kind() {
	case reflect.Map:
		after = f.cloneEmpty()
		keys := f.val.MapKeys()
		for _, key := range keys {
			value := f.val.MapIndex(key)
			labelled, ok := value.Interface().(Labelled)
			if !ok {
				panic(fmt.Sprintf("values must implement the filter.Labelled interface"))
			}
			var matched bool
			labels := labelled.GetLabels()
			for _, filter := range filters {
				if ok {
					matched = MatchFilter(labels, filter)
					if matched {
						break
					}
				}
			}
			if matched == include {
				after.val.SetMapIndex(key, value)
			}
		}
	case reflect.Slice:
		length := f.val.Len()
		after = f.cloneEmpty()
		for i := 0; i < length; i++ {
			value := f.val.Index(i)
			labelled, ok := value.Interface().(Labelled)
			if !ok {
				panic(fmt.Sprintf("values must implement the filter.Labelled interface"))
			}
			labels := labelled.GetLabels()
			var matched bool
			for _, filter := range filters {
				if ok {
					matched = MatchFilter(labels, filter)
					if matched {
						break
					}
				}
			}
			if matched == include {
				after.val = reflect.Append(after.val, value)
			}
		}
	}

	return after
}

func MatchFilter(labels Labels, filter Filter) bool {
	matched := filter.IsMatch(labels)
	return matched
}
