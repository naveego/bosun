package bosun_test

import (
	. "github.com/naveego/bosun/pkg/bosun"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	"runtime"
	"strings"
)

type container struct {
	DV *DynamicValue `yaml:"dv"`
}

var _ = Describe("DynamicValue", func() {

	Describe("marshalling", func() {

		DescribeTable("should roundtrip", func(yml string) {
			var sut container
			Expect(yaml.Unmarshal([]byte(yml), &sut)).To(Succeed())
			out, err := yaml.Marshal(sut)
			Expect(err).ToNot(HaveOccurred())
			actual := strings.TrimSpace(string(out))
			Expect(actual).To(Equal(yml))
		},
			Entry("value", "dv: some-value"),
			Entry("command", `dv:
- some
- command`),
			Entry("script", `dv: |-
  some
  value`),
		)

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
				Command: []string{"some", "command"},
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

		Describe("execution", func() {
			It("should resolve value from script", func() {
				script := `
  testVar="test string"
  echo $testVar
`

				if runtime.GOOS == "windows" {
					script = `
set testVar=test string
echo %testVar%
`
				}

				sut := &DynamicValue{
					Script: script,
				}
				Expect(sut.Execute(BosunContext{})).To(Equal("test string"))
			})

			It("should include env values", func() {
				ctx := BosunContext{}.WithValues(Values{
					"test": Values{
						"nested": "value",
					},
					"APP_VERSION": "1.2.3",
				})
				sut := &DynamicValue{
					Command: []string{"env"},
				}
				result, err := sut.Execute(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(ContainSubstring("BOSUN_TEST_NESTED=value"))
				Expect(result).To(ContainSubstring("BOSUN_APP_VERSION=1.2.3"))
			})

		})
	})
})
