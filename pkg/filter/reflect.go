package filter

import (
	"fmt"
	"reflect"
)

type filterable struct {
	val reflect.Value
}

func newFilterable(mapOrSlice interface{}) filterable {
	fromValue := reflect.ValueOf(mapOrSlice)
	switch fromValue.Kind() {
	case reflect.Map,
		reflect.Slice:
		return filterable{val: fromValue}
	}
	panic(fmt.Sprintf("invalid type %T, must be a map or slice", mapOrSlice))
}

func (f filterable) len() int {
	return f.val.Len()
}

func (f filterable) cloneEmpty() filterable {
	switch f.val.Kind() {
	case reflect.Map:
		return filterable{val: reflect.MakeMap(f.val.Type())}
	case reflect.Slice:
		return filterable{val: reflect.MakeSlice(f.val.Type(), 0, f.val.Len())}
	default:
		panic(fmt.Sprintf("invalid type, must be a map or slice"))
	}
}
