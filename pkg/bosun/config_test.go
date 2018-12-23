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

var _ = Describe("ConfigFragment", func() {

	Describe("AppValuesByEnvironment", func(){
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
			var sut AppValuesByEnvironment
			Expect(yaml.Unmarshal([]byte(input), &sut)			).To(Succeed())

			Expect(sut).To(BeEquivalentTo(AppValuesByEnvironment{
				"red":{
					Set:map[string]*DynamicValue{
						"redgreen1": {Value: "a"},
						"red1":     {Value: "b"},
					},
					Files:[]string{
						"redgreenfile",
						"redfile",
					},
				},
				"green":{
					Set:map[string]*DynamicValue{
						"redgreen1":{Value:"c"},
						"green1":{Value:"d"},
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
