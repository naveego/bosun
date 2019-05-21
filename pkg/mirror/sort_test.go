package mirror_test

import (
	. "github.com/naveego/bosun/pkg/mirror"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Sort", func() {
	It("should call function", func() {
		sut := []string{"c", "a", "b"}

		Sort(sut, func(a, b string) bool {
			return a < b
		})

		Expect(sut).To(BeEquivalentTo([]string{"a", "b", "c"}))

	})
})
