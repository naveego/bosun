package ioc

import (
	"fmt"
	"github.com/pkg/errors"
	"reflect"
	"sort"
)

type Provider interface {
	Provide(instance interface{}, options ...Options) error
}

type Container struct {
	cache   map[reflect.Type][]binding
	summary []string
}

func NewContainer() *Container {
	return &Container{
		cache: map[reflect.Type][]binding{},
	}
}

func (c Container) String() string {
	sort.Strings(c.summary)
	return fmt.Sprint(c.summary)
}

func (c *Container) BindSingleton(instance interface{}, options ...Options) {
	opt := getOption(options)
	v := reflect.ValueOf(instance)
	if len(opt.asTypes) == 0 {
		opt.asTypes = []reflect.Type{reflect.TypeOf(instance)}
	}
	for _, t := range opt.asTypes {
		b := binding{
			name: opt.Name,
			typ:  t,
			fn: func() reflect.Value {
				return v
			},
		}

		c.cache[t] = append(c.cache[t], b)
		c.summary = append(c.summary, fmt.Sprintf("%s(%s)", t, opt))
	}
}

type binding struct {
	name string
	typ  reflect.Type
	fn   func() reflect.Value
}

func (c *Container) Provide(target interface{}, options ...Options) error {

	opt := getOption(options)
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr {
		return errors.Errorf("instance must be a pointer, got %T", target)
	}

	out := v.Elem()
	targetType := out.Type()

	bs, ok := c.cache[targetType]
	if !ok {
		return errors.Errorf("no bindings for type %q (bindings: %s)", targetType, c)
	}

	for _, b := range bs {
		if b.name != opt.Name {
			continue
		}

		instance := b.fn()
		out.Set(instance)
		return nil
	}

	return errors.Errorf("no binding matched type %s with options %s (bindings: %s)", targetType, opt, c)

}

func getOption(options []Options) Options {
	for _, o := range options {
		return o
	}
	return Options{}
}
