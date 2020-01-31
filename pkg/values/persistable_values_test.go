package values_test

import (
	"fmt"
	. "github.com/naveego/bosun/pkg/values"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	yml "gopkg.in/yaml.v3"
)

var _ = Describe("PersistableValues", func() {


	Describe("yaml nodes", func() {
		It("should marshal as expected", func() {
			input := Values{

				"from-a": "a-value",
				"nested": Values{
					"from-b1": "b1-value",
					"from-b2": "b2-value",
				},
			}
			y, _ := yml.Marshal(input)
			var node yml.Node

			Expect(yml.Unmarshal(y, &node)).To(Succeed())

			node.Content[0].HeadComment = "head-comment"
			node.FootComment = "foot-comment"
			node.Content[0].Content[0].LineComment = "line-comment"

			out, _ := yml.Marshal(&node)

			fmt.Println(string(out))

			Expect(string(out)).To(ContainSubstring("b1-value"))
			Expect(string(out)).To(ContainSubstring("# head-comment"))
			Expect(string(out)).To(ContainSubstring("# foot-comment"))
			Expect(string(out)).To(ContainSubstring("# line-comment"))

		})
	})

	FDescribe("reporting", func() {
		It("should report with comments", func() {
			sut := PersistableValues{
				Attribution: Values{

					"from-a": "from source a",
					"nested": Values{
						"from-b1": "from source b",
						"from-a2": "from source a",
					},
				},
				Values: Values{

					"from-a": "a-value",
					"nested": Values{
						"from-b1": "b1-value",
						"from-b2": "b2-value",
					},
				},
			}
			actual := sut.Report()
			Expect(actual).To(ContainSubstring("# from source a"))
			Expect(actual).To(ContainSubstring("from-a: a-value"))
		})
	})
})
