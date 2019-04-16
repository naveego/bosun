package bosun_test

import (
	"fmt"
	. "github.com/naveego/bosun/pkg/bosun"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
)

var _ = Describe("E2E", func() {

	It("should round trip to yaml", func() {
		sut := E2ETestConfig{
			ConfigShared: ConfigShared{
				Name: "test-e2e-test",
			},
		}

		y, err := yaml.Marshal(sut)
		Expect(err).ToNot(HaveOccurred())
		var actual E2ETestConfig
		Expect(yaml.Unmarshal(y, &actual)).To(Succeed())

		fmt.Println(string(y))

		Expect(actual).To(BeEquivalentTo(sut))
	})
})
