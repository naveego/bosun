package ioc

import (
	"fmt"
	"reflect"
)

type Options struct {
	Name    string
	asTypes []reflect.Type
}

func (o Options) WithName(name string) Options {
	o.Name = name
	return o
}

func (o Options) ProvidingTypes(types ...interface{}) Options {
	for _, example := range types {
		t := reflect.TypeOf(example)
		if t.Kind() != reflect.Ptr {
			panic(fmt.Sprintf("providing types must be a list of pointers to the types provided, like ((*Interface)(nil))"))
		}
		o.asTypes = append(o.asTypes, reflect.TypeOf(example).Elem())
	}

	return o
}

func Option() Options {
	return Options{}
}

func (o Options) String() string {
	if o.Name != "" {
		return fmt.Sprintf("Name:%q", o.Name)
	}
	return "{}"
}
