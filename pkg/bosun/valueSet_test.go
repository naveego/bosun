package bosun_test

import (
	. "github.com/naveego/bosun/pkg/bosun"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
)

var _ = Describe("ValueSetMap", func() {

	input := yamlize(
		// language=yaml
		`
  green:
  	static:
  		green1: d
  		redgreen1: c
		redgreenmap: 
		  a: greenA
		  d: greenD
  red,green: 
    static:
      redgreen1: a
      redgreen2: f
      redgreenmap: 
        a: redgreenA
        b: redgreenB
  red: 
  	static:
  	  red1: b
  	  redgreenmap: 
        a: redA
        c: redC
  blue:
    static:
      blue1: e
`)

	It("should extract values by name", func() {

		var sut ValueSetMap
		Expect(yaml.Unmarshal([]byte(input), &sut)).To(Succeed())
		redValues := sut.ExtractValueSetByName("green")
		Expect(redValues.Static).To(HaveKeyWithValue("green1", "d"), "it is in the green set")
		Expect(redValues.Static).To(HaveKeyWithValue("redgreen1", "c"), "the green key has a higher priority than the red,green key")
		Expect(redValues.Static).To(HaveKeyWithValue("redgreen2", "f"), "the red,green key should be integrated")
	})

	It("should extract multiple values by name", func() {
		var sut ValueSetMap
		Expect(yaml.Unmarshal([]byte(input), &sut)).To(Succeed())
		redValues := sut.ExtractValueSetByNames("green", "red")
		Expect(redValues.Static).To(HaveKeyWithValue("green1", "d"), "it is in the green set")
		Expect(redValues.Static).To(HaveKeyWithValue("redgreen1", "a"), "the green key has a higher priority than the red,green key")
		Expect(redValues.Static).To(HaveKeyWithValue("redgreen2", "f"), "the red,green key should be integrated")
		Expect(redValues.Static).To(HaveKeyWithValue("red1", "b"), "the red,green key should be integrated")
	})
	It("should canonicalize correctly", func() {
		var sut ValueSetMap
		Expect(yaml.Unmarshal([]byte(input), &sut)).To(Succeed())
		actual := sut.CanonicalizedCopy()
		Expect(actual["red"].Static).To(
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
		Expect(actual["green"].Static).To(
			And(
				HaveKeyWithValue("green1", "d"),
				HaveKeyWithValue("redgreen1", "c"),
				HaveKeyWithValue("redgreen2", "f"),
			))
		Expect(actual["blue"].Static).To(HaveKeyWithValue("blue1", "e"))

	})

})
