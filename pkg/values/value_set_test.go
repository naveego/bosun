package values_test

import (
	"context"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/templating"
	"github.com/naveego/bosun/pkg/util"
	. "github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"time"
)

var _ = Describe("ValueSetCollection", func() {

	input := yaml.Yamlize(
		// language=yaml
		`
  green:
    name: green
    static:
      green1: d
      redgreen1: c
      redgreenmap: 
        a: greenA
        d: greenD
        e: redgreenE
  red,green: 
    name: redgreen
    static:
      redgreen1: a
      redgreen2: f
      redgreenmap: 
        a: redgreenA
        b: redgreenB
        f: redgreenF
  red: 
    name: red
    static:
      red1: b
      redgreenmap: 
        a: redA
        c: redC
        e: redgreenE
  red,redfiltered: 
    name: redfiltered
    exactMatchFilters: 
      multi: [a,b]
      single: [c]
    static:
      filtered: not-present-without-filter
  blue:
    name: blue
    static:
      blue1: e
`)

	It("should unmarshall to slice pattern", func() {

		var actual *ValueSetCollection
		Expect(yaml.Unmarshal([]byte(input), &actual)).To(Succeed())

		expected := ValueSetCollection{
			ValueSets: ValueSets{
				{
					ConfigShared: core.ConfigShared{
						Name: "redgreen",
					},
					Roles: []core.EnvironmentRole{"red", "green"},
					Static: Values{
						"redgreen1": "a",
						"redgreen2": "f",
						"redgreenmap": Values{
							"a": "redgreenA",
							"b": "redgreenB",
							"f": "redgreenF",
						},
					},
				}, {
					ConfigShared: core.ConfigShared{
						Name: "redfiltered",
					},
					Roles: []core.EnvironmentRole{"red", "redfiltered"},
					ExactMatchFilters: map[string][]string{
						"multi":  []string{"a", "b"},
						"single": []string{"c"},
					},
					Static: Values{
						"filtered": "not-present-without-filter",
					},
				},
				{
					ConfigShared: core.ConfigShared{
						Name: "blue",
					}, Roles: []core.EnvironmentRole{"blue"},
					Static: Values{
						"blue1": "e",
					},
				},
				{

					ConfigShared: core.ConfigShared{
						Name: "green",
					},
					Roles: []core.EnvironmentRole{"green"},
					Static: Values{
						"green1":    "d",
						"redgreen1": "c",
						"redgreenmap": Values{
							"a": "greenA",
							"d": "greenD",
							"e": "redgreenE",
						},
					},
				},
				{
					ConfigShared: core.ConfigShared{
						Name: "red",
					},
					Roles: []core.EnvironmentRole{"red"},
					Static: Values{
						"red1": "b",
						"redgreenmap": Values{
							"a": "redA",
							"c": "redC",
							"e": "redgreenE",
						},
					},
				},
			},
		}

		actualYaml, _ := yaml.MarshalString(actual)
		expectedYaml, _ := yaml.MarshalString(expected)

		// fmt.Println("\nactual")
		// fmt.Println(actualYaml)
		// fmt.Println("\nexpected")
		// fmt.Println(expectedYaml)
		// fmt.Println("\ndiff")

		// diff := diffmatchpatch.New().DiffMain(actualYaml, expectedYaml, true)
		// p := diffmatchpatch.New().DiffPrettyText(diff)

		// fmt.Println(p)

		Expect(actualYaml).To(Equal(expectedYaml))
	})

	It("should extract values by name", func() {

		var sut *ValueSetCollection
		Expect(yaml.Unmarshal([]byte(input), &sut)).To(Succeed())
		redValues := sut.ExtractValueSetByRole("green")
		Expect(redValues.Static).To(HaveKeyWithValue("green1", "d"), "it is in the green set")
		Expect(redValues.Static).To(HaveKeyWithValue("redgreen1", "c"), "the green key has a higher priority than the red,green key")
		Expect(redValues.Static).To(HaveKeyWithValue("redgreen2", "f"), "the red,green key should be integrated")
	})

	It("should extract multiple values by name", func() {
		var sut ValueSetCollection
		Expect(yaml.Unmarshal([]byte(input), &sut)).To(Succeed())
		redValues := sut.ExtractValueSetByRoles("green", "red")
		Expect(redValues.Static).To(HaveKeyWithValue("green1", "d"), "it is in the green set")
		Expect(redValues.Static).To(HaveKeyWithValue("redgreen1", "c"), "the green key has a higher priority than the red,green key")
		Expect(redValues.Static).To(HaveKeyWithValue("redgreen2", "f"), "the red,green key should be integrated")
		Expect(redValues.Static).To(HaveKeyWithValue("red1", "b"), "the red,green key should be integrated")
	})
	It("should canonicalize correctly", func() {
		var sut ValueSetCollection
		Expect(yaml.Unmarshal([]byte(input), &sut)).To(Succeed())
		actual := sut.CanonicalizedCopy()
		red, err := actual.FindValueSetForRole("red")
		Expect(err).ToNot(HaveOccurred())
		Expect(red.Static).To(
			And(
				HaveKeyWithValue("redgreen1", "a"),
				HaveKeyWithValue("redgreen2", "f"),
				HaveKeyWithValue("red1", "b"),
				HaveKeyWithValue("redgreenmap", And(
					HaveKeyWithValue("a", "redA"),
					HaveKeyWithValue("b", "redgreenB"),
					HaveKeyWithValue("c", "redC"),
				)),
			))
		green, err := actual.FindValueSetForRole("green")
		Expect(err).ToNot(HaveOccurred())
		Expect(green.Static).To(
			And(
				HaveKeyWithValue("green1", "d"),
				HaveKeyWithValue("redgreen1", "c"),
				HaveKeyWithValue("redgreen2", "f"),
			))
		blue, err := actual.FindValueSetForRole("blue")
		Expect(err).ToNot(HaveOccurred())
		Expect(blue.Static).To(HaveKeyWithValue("blue1", "e"))

	})

	It("should match with exact match labels", func() {
		var sut ValueSetCollection
		Expect(yaml.Unmarshal([]byte(input), &sut)).To(Succeed())
		redValues := sut.ExtractValueSet(ExtractValueSetArgs{
			Roles: []core.EnvironmentRole{"red"},
			ExactMatch: map[string]string{
				"multi":  "b",
				"single": "c",
			},
		})
		Expect(redValues.Static).To(HaveKeyWithValue("filtered", "not-present-without-filter"), "it is in the redfiltered set")
		Expect(redValues.Static).To(HaveKeyWithValue("red1", "b"), "it is in the red set")

	})

	It("should render internal templates safely", func() {
		sut := ValueSet{

			ConfigShared: core.ConfigShared{
				Name: "redgreen",
			},
			Roles: []core.EnvironmentRole{"red", "green"},
			Static: Values{
				"static":   "static-value",
				"template": "rendered-{{.Values.static}}",
				"preserve": "{{.username}}",
			},
		}

		actual, err := sut.WithDynamicValuesResolved(mockExecutionContext{})
		Expect(err).ToNot(HaveOccurred())
		Expect(actual.Static).To(HaveKeyWithValue("static", "static-value"))
		Expect(actual.Static).To(HaveKeyWithValue("template", "rendered-static-value"))
		Expect(actual.Static).To(HaveKeyWithValue("preserve", "{{.username}}"))

	})
})

type mockExecutionContext struct {
}

func (mockExecutionContext) GetParameters() cli.Parameters {
	panic("implement me")
}

func (mockExecutionContext) Pwd() string {
	panic("implement me")
}

func (mockExecutionContext) WithPwd(pwd string) cli.WithPwder {
	panic("implement me")
}

func (mockExecutionContext) GetEnvironmentVariables() map[string]string {
	panic("implement me")
}

func (mockExecutionContext) TemplateValues() templating.TemplateValues {
	panic("implement me")
}

func (mockExecutionContext) Log() *logrus.Entry {
	panic("implement me")
}

func (mockExecutionContext) WithLogField(name string, value interface{}) util.WithLogFielder {
	panic("implement me")
}

func (mockExecutionContext) Ctx() context.Context {
	panic("implement me")
}

func (mockExecutionContext) WithTimeout(timeout time.Duration) core.Ctxer {
	panic("implement me")
}
