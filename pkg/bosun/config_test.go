package bosun_test

import (
	"fmt"
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

			input := yamlize(
				`name: app
values:
  green:
  	set:
  		green1: d
  		redgreen1: c
  	files:
  		- greenfile
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

`)
			var sut AppConfig

			Expect(yaml.Unmarshal([]byte(input), &sut)).To(Succeed())

			redValues := sut.GetValuesConfig(BosunContext{Env:&EnvironmentConfig{Name:"red"}})

			Expect(redValues).To(BeEquivalentTo(AppValuesConfig{
				Set: map[string]*DynamicValue{
					"redgreen1": {Value: "a"},
					"red1":      {Value: "b"},
				},
				Files: []string{
					"redgreenfile",
					"redfile",
				},
			}))

			greenValues := sut.GetValuesConfig(BosunContext{Env:&EnvironmentConfig{Name:"green"}})

			Expect(greenValues).To(BeEquivalentTo(AppValuesConfig{
					Set:map[string]*DynamicValue{
						"redgreen1":{Value:"c"},
						"green1":{Value:"d"},
					},
					Files:[]string{
						"redgreenfile",
						"greenfile",
					},
				}))

			b, err := yaml.Marshal(sut)
			Expect(err).ToNot(HaveOccurred())
			fmt.Println(string(b))
			roundtripped := string(b)
			Expect(roundtripped).To(ContainSubstring("values"))

		})
	})
})
