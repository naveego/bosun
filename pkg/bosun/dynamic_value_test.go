package bosun_test

import (
	. "github.com/naveego/bosun/pkg/bosun"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
)

type container struct {
	DV *DynamicValue `yaml:"dv"`
}

var _ = Describe("DynamicValue", func() {

	It("should assign string to Value", func() {

		input := yamlize(`
dv: some-value
`)
		var sut container
		Expect(yaml.Unmarshal([]byte(input), &sut)).To(Succeed())

		Expect(*sut.DV).To(BeEquivalentTo(DynamicValue{
			Value: "some-value",
		}))
	})

	It("should assign array to Command", func() {
		input := yamlize(`
dv: [some,command]
`)
		var sut container
		Expect(yaml.Unmarshal([]byte(input), &sut)).To(Succeed())

		Expect(*sut.DV).To(BeEquivalentTo(DynamicValue{
			Command: []string{"some","command"},
		}))
	})

	It("should assign multiline to script", func() {
		input := yamlize(`
dv: |
  some
  script
`)
		var sut container
		Expect(yaml.Unmarshal([]byte(input), &sut)).To(Succeed())

		Expect(*sut.DV).To(BeEquivalentTo(DynamicValue{
			Script: `some
script
`,
		}))
	})
})
