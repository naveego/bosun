package filter_test

import (
	. "github.com/naveego/bosun/pkg/filter"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Filter", func() {
	ab := Item{
		Name: "ab",
		Labels: map[string]string{
			"a": "A",
			"b": "B",
		},
	}
	abc := Item{
		Name: "abc",
		Labels: map[string]string{
			"a": "A",
			"b": "B",
			"c": "C",
		},
	}
	c := Item{
		Name: "c",
		Labels: map[string]string{
			"c": "C",
		},
	}
	d := Item{
		Name: "d",
		Labels: map[string]string{
			"d": "D",
		},
	}
	items := []Item{
		ab,
		abc,
		c,
		d,
	}

	Describe("include", func() {
		It("should include matched", func() {
			Expect(Include(items, MustParse("a==A"))).To(ConsistOf(ab, abc))
		})
	})
	Describe("exclude", func() {
		It("should include unmatched", func() {
			Expect(Exclude(items, MustParse("c==C"))).To(ConsistOf(ab, d))
		})
	})
})
