package filter

import (
	"fmt"
	"github.com/pkg/errors"
	"math"
	"strings"
)

type Chain struct {
	steps   []step
	current *step
	min     *int
	max     *int
}

type step struct {
	include []Filter
	exclude []Filter
}

func (s step) String() string {
	var include, exclude []string
	for _, i := range s.include {
		include = append(include, i.String())
	}
	for _, i := range s.exclude {
		exclude = append(exclude, i.String())
	}
	out := ""
	if len(include) > 0 {
		out += fmt.Sprintf("include(%s)", strings.Join(include, ","))
	}
	if len(exclude) > 0 {
		out += fmt.Sprintf("exclude(%s)", strings.Join(include, ","))
	}
	return out
}

func Try() Chain {
	return Chain{}
}

func (c Chain) Including(f ...Filter) Chain {
	if c.current == nil {
		c.current = &step{}
	}
	c.current.include = append(c.current.include, f...)
	return c
}

func (c Chain) Excluding(f ...Filter) Chain {
	if c.current == nil {
		c.current = &step{}
	}
	c.current.exclude = append(c.current.exclude, f...)
	return c
}

func (c Chain) Then() Chain {
	if c.current == nil {
		panic("no current step (should call Including or Excluding first")
	}
	c.steps = append(c.steps, *c.current)
	c.current = nil
	return c
}

func (c Chain) ToGet(min, max int) Chain {
	c.max = &max
	c.min = &min
	return c
}

func (c Chain) ToGetExactly(want int) Chain {
	c.max = &want
	c.min = &want
	return c
}

func (c Chain) ToGetAtLeast(want int) Chain {
	m := math.MaxInt64
	c.max = &m
	c.min = &want
	return c
}

// From returns the filtered set, or an empty value of the same type as from if an expectation failed.
func (c Chain) From(from interface{}) interface{} {
	out, _ := c.FromErr(from)
	return out
}

func (c Chain) String() string {
	var steps []string
	for _, s := range c.steps {
		steps = append(steps, s.String())
	}
	return strings.Join(steps, "\n")
}

// FromErr returns the filtered set, or an error if all steps failed expectations.
func (c Chain) FromErr(from interface{}) (interface{}, error) {

	f := newFilterable(from)

	if f.len() == 0 {
		return from, nil
	}

	steps := c.steps
	if c.current != nil {
		steps = append(steps, *c.current)
	}
	min := 1
	max := math.MaxInt64
	if c.min != nil {
		min = *c.min
	}
	if c.max != nil {
		max = *c.max
	}

	var after filterable
	for _, s := range steps {
		if len(s.include) > 0 && len(s.exclude) > 0 {
			after = applyFilters(f, s.include, true)
			after = applyFilters(after, s.exclude, false)
		} else if len(s.include) > 0 {
			after = applyFilters(f, s.include, true)
		} else if len(s.exclude) > 0 {
			after = applyFilters(f, s.exclude, false)
		} else {
			after = f.cloneEmpty()
		}
		if after.len() >= min && after.len() <= max {
			return after.val.Interface(), nil
		}
	}

	maxString := "∞"
	if max < math.MaxInt64 {
		maxString = fmt.Sprint(max)
	}

	return f.cloneEmpty().val.Interface(), errors.Errorf("no steps in chain could reduce initial set of %d items to requested size of [%d,%s]\nsteps:\n%s", f.len(), min, maxString, c)
}
