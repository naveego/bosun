package bosun_test

import (
	. "github.com/naveego/bosun/pkg/bosun"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	"strings"
)

func yamlize(y string) string {
	return strings.Replace(y, "\t", "  ", -1)
}

var _ = Describe("Config", func() {

	Describe("AppValuesMap", func(){
		It("should merge values when unmarshalled", func(){

			input := yamlize(`
red,green: 
	set:
		redgreen1: a
	files: 
		- redgreenfile
red: 
	set:
		red1: b
	files:
	 	- redfile
green:
	set:
		redgreen1: c
		green1: d
	files:
		- greenfile
`)
			var sut AppValuesMap
			Expect(yaml.Unmarshal([]byte(input), &sut)			).To(Succeed())

			Expect(sut).To(BeEquivalentTo(AppValuesMap{
				"red":{
					Set:map[string]string{
						"redgreen1":"a",
						"red1":"b",
					},
					Files:[]string{
						"redgreenfile",
						"redfile",
					},
				},
				"green":{
					Set:map[string]string{
						"redgreen1":"c",
						"green1":"d",
					},
					Files:[]string{
						"redgreenfile",
						"greenfile",
					},
				},
			}))
		})
	})
})
