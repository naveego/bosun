package command_test

import (
	"fmt"
	. "github.com/naveego/bosun/pkg/bosun"
	. "github.com/naveego/bosun/pkg/command"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"runtime"
	"strings"
)

func yamlize(y string) string {
	return strings.Replace(y, "\t", "  ", -1)
}

type container struct {
	DV *CommandValue `yaml:"dv" json:"dv"`
}

var _ = Describe("CommandValue", func() {

	var ctx BosunContext

	BeforeEach(func() {
		ctx = BosunContext{}.WithLog(logrus.NewEntry(logrus.StandardLogger()))
	})

	Describe("marshalling", func() {

		DescribeTable("should roundtrip", func(in string, expectedValue CommandValue, expectedYaml string) {
			var sut container
			Expect(yaml.Unmarshal([]byte(in), &sut)).To(Succeed())
			out, err := yaml.Marshal(sut)
			Expect(err).ToNot(HaveOccurred())
			actual := strings.TrimSpace(string(out))
			Expect(*sut.DV).To(BeEquivalentTo(expectedValue))
			Expect(fmt.Sprintf("%q", actual)).To(Equal(fmt.Sprintf("%q", expectedYaml)))
		},
			Entry("value",
				"dv: some-value",
				CommandValue{Value: "some-value"},
				`dv: some-value`),
			Entry("value explicit",
				`dv:
  value: some-value`,
				CommandValue{Value: "some-value"},
				`dv: some-value`),

			Entry("command implicit",
				`dv:
  command: 
  - some
  - command`,
				CommandValue{Command: Command{Command: []string{"some", "command"}}},
				`dv:
  command: [some, command]`),
			Entry("command explicit",
				`dv:
  - some
  - command`,
				CommandValue{Command: Command{Command: []string{"some", "command"}}},
				`dv:
  command: [some, command]`),
			Entry("command explicit with tools",
				`dv:
  command: [some, command]
  tools: 
    - xyz`,
				CommandValue{Command: Command{Command: []string{"some", "command"}, Tools: []string{"xyz"}}},
				`dv:
  command: [some, command]
  tools:
  - xyz`),
			Entry("script",
				`dv: 
  script: |-
    some
    value`, CommandValue{Command: Command{Script: `some
value`}}, `dv:
  script: |-
    some
    value`),
		)

		It("should assign string to Value", func() {
			input := yamlize(`
dv: some-value
`)
			var sut container
			Expect(yaml.Unmarshal([]byte(input), &sut)).To(Succeed())

			Expect(*sut.DV).To(BeEquivalentTo(CommandValue{
				Value: "some-value",
			}))
		})

		It("should assign array to Command", func() {
			input := yamlize(`
dv: [some,command]
`)
			var sut container
			Expect(yaml.Unmarshal([]byte(input), &sut)).To(Succeed())

			Expect(*sut.DV).To(BeEquivalentTo(CommandValue{
				Command: Command{
					Command: []string{"some", "command"},
				},
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

			Expect(*sut.DV).To(BeEquivalentTo(CommandValue{
				Command: Command{Script: `some
script
`,
				},
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

				sut := &CommandValue{
					Command: Command{
						Script: script,
					}}
				Expect(sut.Execute(ctx)).To(Equal("test string"))
			})

			It("should include env values", func() {
				ctx = ctx.WithPersistableValues(&PersistableValues{
					Values: Values{
						"test": Values{
							"nested": "value",
						},
						"APP_VERSION": "1.2.3",
					},
				})
				sut := &CommandValue{
					Command: Command{Command: []string{"env"}},
				}
				result, err := sut.Execute(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(ContainSubstring("BOSUN_TEST_NESTED=value"))
				Expect(result).To(ContainSubstring("BOSUN_APP_VERSION=1.2.3"))
			})

		})
	})
})
