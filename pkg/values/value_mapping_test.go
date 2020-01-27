package values_test

import (
	"fmt"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ValueMapping", func() {

	It("should map value", func() {
		target := values.Values{
			"a": values.Values{
				"b": "B",
			},
		}
		mappings := values.ValueMappings{
			"a.b": "c.d",
		}

		actual := target.Clone()
		Expect(mappings.ApplyToValues(actual)).To(Succeed())

		y, err := yaml.MarshalString(actual)
		Expect(err).ToNot(HaveOccurred())
		fmt.Println(y)

		Expect(actual).To(HaveKeyWithValue("c", HaveKeyWithValue("d", Equal("B"))))
	})
})
