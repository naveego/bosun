package filter_test

import (
	. "github.com/naveego/bosun/pkg/filter"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("Simple filter", func() {

	labels := LabelsFromMap(map[string]string{
		"a":    "A",
		"long": "string-with-data",
		"key":  "",
	})

	DescribeTable("parsing", func(input string, expected bool) {
		f, err := Parse(input)
		Expect(err).ToNot(HaveOccurred())

		Expect(f.IsMatch(labels)).To(Equal(expected))
	},
		Entry("key match", "key", true),
		Entry("key mismatch", "crow", false),
		Entry("equality match", "a==A", true),
		Entry("equality mismatch", "a==B", false),
		Entry("inequality match", "a!=B", true),
		Entry("inequality mismatch", "a!=A", false),
		Entry("regex match", "long?=.*-with", true),
		Entry("regex mismatch", "long?=.*-wrong", false),
	)

})
