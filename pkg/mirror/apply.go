package mirror

import (
	"reflect"
)

func ApplyFuncRecursively(target interface{}, fn interface{}) {

	fnVal := reflect.ValueOf(fn)
	if fnVal.Kind() != reflect.Func {
		panic("fn must be a function")
	}
	if fnVal.Type().NumIn() != 1 {
		panic("fn must be a function with one parameter")
	}

	argType := fnVal.Type().In(0)

	val := reflect.ValueOf(target)

	for val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Type().AssignableTo(argType) || val.Type().Implements(argType) {
		fnVal.Call([]reflect.Value{val})
	}

	numFields := val.NumField()
	for i := 0; i < numFields; i++ {
		field := val.Field(i)
		if field.Kind() != reflect.Slice {
			continue
		}
		entryCount := field.Len()
		for j := 0; j < entryCount; j++ {
			entry := field.Index(j)
			if entry.Type().AssignableTo(argType) || entry.Type().Implements(argType) {
				fnVal.Call([]reflect.Value{entry})
			}
		}
	}
}
