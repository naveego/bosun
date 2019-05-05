package filter

import (
	"reflect"
)

type Filter interface {
	IsMatch(l Labels) bool
}

type Labelled interface {
	GetLabels() Labels
}

type FilterFunc func(l Labels) bool

func (f FilterFunc) IsMatch(l Labels) bool {
	return f(l)
}

func FilterMatchAll() Filter {
	return FilterFunc(func(l Labels) bool { return true })
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
			var matched bool
			for _, filter := range filters {
				if ok {
					matched = MatchFilter(labelled, filter)
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
			var matched bool
			for _, filter := range filters {
				if ok {
					matched = MatchFilter(labelled, filter)
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

func MatchFilter(labelled Labelled, filter Filter) bool {
	labels := labelled.GetLabels()
	matched := filter.IsMatch(labels)
	return matched
}
