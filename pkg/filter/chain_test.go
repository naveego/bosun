package filter_test

import (
	. "github.com/naveego/bosun/pkg/filter"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Chain", func() {

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

	Describe("when one step is provided", func() {
		It("include should whitelist", func() {
			Expect(Try().Including(MustParse("a==A")).From(items)).To(ConsistOf(ab, abc))
		})

		It("include exclude blacklist", func() {
			Expect(Try().Excluding(MustParse("c==C")).From(items)).To(ConsistOf(ab, d))
		})

		It("apply include and exclude", func() {
			Expect(Try().
				Including(MustParse("a==A")).
				Excluding(MustParse("c==C")).
				From(items)).To(ConsistOf(ab))
		})
	})

	Describe("when two steps are provided", func() {
		It("should return first step results if not empty", func() {
			Expect(Try().
				Including(MustParse("a==A")).
				Then().
				Including(MustParse("b==B")).
				From(items)).
				To(ConsistOf(ab, abc))
		})
		It("should return second step results if first is empty but second is not empty", func() {
			Expect(Try().
				Including(MustParse("a==C")).
				Then().
				Including(MustParse("c==C")).
				From(items)).
				To(ConsistOf(abc, c))
		})
		It("should return first step where result falls within desired count at low end", func() {
			Expect(Try().
				Including(MustParse("d==D")).
				Then().
				Including(MustParse("a==A")).
				ToGet(2, 3).
				From(items)).
				To(ConsistOf(ab, abc))
		})
		It("should return first step where result falls within desired count at high end", func() {
			Expect(Try().
				Including(MustParse("d==D")).
				Then().
				Including(MustParse("a==A"), MustParse("c==C")).
				ToGet(2, 3).
				From(items)).
				To(ConsistOf(ab, abc, c))
		})

		It("should return first step which meets exact count", func() {
			Expect(Try().
				Including(MustParse("a==A")).
				Then().
				Including(MustParse("d==D")).
				ToGetExactly(1).
				From(items)).
				To(ConsistOf(d))
		})

		It("should error if no step which meets requested count", func() {
			_, err := Try().Including(MustParse("d==D")).
				ToGetExactly(2).
				FromErr(items)
			Expect(err).To(HaveOccurred())
		})
	})

})
