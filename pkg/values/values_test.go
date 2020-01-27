package values_test

import (
	"github.com/naveego/bosun/pkg/values"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Values", func() {

	Describe(".Distill", func() {

		It("should extract the shared values", func() {
			left := values.Values{
				"same-1": "same-1",
				"diff-1": "diff-1-left",
				"child-1": values.Values{
					"same-2": "same-2",
					"diff-2": "diff-2-left",
				},
				"arr-1": []values.Values{
					{
						"same-3": "same-3",
					},
					{
						"same-4": "same-4",
					},
				},
				"arr-2": []values.Values{
					{
						"same-3": "same-3",
						"diff-3": "diff-3-left",
					},
				},
			}
			right := values.Values{
				"same-1": "same-1",
				"diff-1": "diff-1-right",
				"child-1": values.Values{
					"same-2": "same-2",
					"diff-2": "diff-2-right",
				},
				"arr-1": []values.Values{
					{
						"same-3": "same-3",
					},
					{
						"same-4": "same-4",
					},
				},
				"arr-2": []values.Values{
					{
						"same-3": "same-3",
						"diff-3": "diff-3-right",
					},
				},
			}

			common, leftResidue, rightResidue := values.Distill(left, right)

			Expect(common).To(BeEquivalentTo(values.Values{
				"same-1": "same-1",
				"child-1": values.Values{
					"same-2": "same-2",
				},
				"arr-1": []values.Values{
					{
						"same-3": "same-3",
					},
					{
						"same-4": "same-4",
					},
				},
			}))
			Expect(leftResidue).To(BeEquivalentTo(values.Values{
				"diff-1": "diff-1-left",
				"child-1": values.Values{
					"diff-2": "diff-2-left",
				},
				"arr-2": []values.Values{
					{
						"same-3": "same-3",
						"diff-3": "diff-3-left",
					},
				},
			}))
			Expect(rightResidue).To(BeEquivalentTo(values.Values{
				"diff-1": "diff-1-right",
				"child-1": values.Values{
					"diff-2": "diff-2-right",
				},
				"arr-2": []values.Values{
					{
						"same-3": "same-3",
						"diff-3": "diff-3-right",
					},
				},
			}))
		})

	})

})
