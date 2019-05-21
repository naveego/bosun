package mirror

import (
	"reflect"
	"sort"
)

// Sort sorts target by calling less.
// target must be a slice []T.
// less must be a function of the form func(a, b T) bool
func Sort(target interface{}, less interface{}) {

	lessVal := reflect.ValueOf(less)
	lessType := lessVal.Type()
	if lessVal.Kind() != reflect.Func {
		panic("fn must be a function")
	}
	if lessType.NumIn() != 2 {
		panic("fn must be a function with 2 parameters")
	}
	if lessType.NumOut() != 1 || lessType.Out(0).Kind() != reflect.Bool {
		panic("fn must be a function with 1 bool return")
	}

	val := reflect.ValueOf(target)
	if val.Kind() != reflect.Slice {
		panic("target must be a slice")
	}

	s := sorter{
		t:    val,
		less: lessVal,
	}

	sort.Sort(s)
}

type sorter struct {
	t    reflect.Value
	less reflect.Value
}

func (s sorter) Len() int {
	return s.t.Len()
}

func (s sorter) Less(i, j int) bool {
	ix := s.t.Index(i)
	jx := s.t.Index(j)
	rv := s.less.Call([]reflect.Value{ix, jx})
	bv := rv[0]
	return bv.Bool()
}

func (s sorter) Swap(i, j int) {
	ix := s.t.Index(i)
	jx := s.t.Index(j)
	iv := ix.Interface()
	jv := jx.Interface()
	ix.Set(reflect.ValueOf(jv))
	jx.Set(reflect.ValueOf(iv))
}
