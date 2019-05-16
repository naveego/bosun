package mirror_test

import (
	. "github.com/naveego/bosun/pkg/mirror"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type Target struct {
	Values []ValueString
}

type ValueString string

func (v ValueString) Value() string {
	return string(v)
}

type Valuer interface {
	Value() string
}

var _ = Describe("Reflect", func() {
	It("should call function", func() {
		target := Target{
			Values: []ValueString{"x", "y"},
		}
		var out []string

		ApplyFuncRecursively(target, func(x Valuer) {
			out = append(out, x.Value())
		})

		Expect(out).To(ConsistOf("x", "y"))

	})
})
