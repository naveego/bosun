package values_test

import (
	"fmt"
	. "github.com/naveego/bosun/pkg/values"
	yaml "github.com/naveego/bosun/pkg/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sergi/go-diff/diffmatchpatch"
)

var _ = Describe("ValueSetCollection", func() {

	input := yaml.Yamlize(
		// language=yaml
		`
  green:
    static:
      green1: d
      redgreen1: c
      redgreenmap: 
        a: greenA
        d: greenD
        e: redgreenE
  red,green: 
    static:
      redgreen1: a
      redgreen2: f
      redgreenmap: 
        a: redgreenA
        b: redgreenB
        f: redgreenF
  red: 
    static:
      red1: b
      redgreenmap: 
        a: redA
        c: redC
        e: redgreenE
  blue:
    static:
      blue1: e
`)

	It("should unmarshall to slice pattern", func() {

		var actual *ValueSetCollection
		Expect(yaml.Unmarshal([]byte(input), &actual)).To(Succeed())

		expected := ValueSetCollection{
			ValueSets: ValueSets{
				{
					Roles: []string{"red", "green"},
					Static: Values{
						"redgreen1": "a",
						"redgreen2": "f",
						"redgreenmap": Values{
							"a": "redgreenA",
							"b": "redgreenB",
							"f": "redgreenF",
						},
					},
				},
				{
					Roles: []string{"blue"},
					Static: Values{
						"blue1": "e",
					},
				},
				{
					Roles: []string{"green"},
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
					Roles: []string{"red"},
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

		fmt.Println("\nactual")
		fmt.Println(actualYaml)
		fmt.Println("\nexpected")
		fmt.Println(expectedYaml)
		fmt.Println("\ndiff")

		diff := diffmatchpatch.New().DiffMain(actualYaml, expectedYaml, true)
		p := diffmatchpatch.New().DiffPrettyText(diff)

		fmt.Println(p)

		Expect(actualYaml).To(Equal(expectedYaml))
	})

	It("should extract values by name", func() {

		var sut *ValueSetCollection
		Expect(yaml.Unmarshal([]byte(input), &sut)).To(Succeed())
		redValues := sut.ExtractValueSetByName("green")
		Expect(redValues.Static).To(HaveKeyWithValue("green1", "d"), "it is in the green set")
		Expect(redValues.Static).To(HaveKeyWithValue("redgreen1", "c"), "the green key has a higher priority than the red,green key")
		Expect(redValues.Static).To(HaveKeyWithValue("redgreen2", "f"), "the red,green key should be integrated")
	})

	It("should extract multiple values by name", func() {
		var sut ValueSetCollection
		Expect(yaml.Unmarshal([]byte(input), &sut)).To(Succeed())
		redValues := sut.ExtractValueSetByNames("green", "red")
		Expect(redValues.Static).To(HaveKeyWithValue("green1", "d"), "it is in the green set")
		Expect(redValues.Static).To(HaveKeyWithValue("redgreen1", "a"), "the green key has a higher priority than the red,green key")
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

})