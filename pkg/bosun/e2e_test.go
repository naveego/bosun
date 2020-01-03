package bosun_test

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	. "github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"path/filepath"
)

var _ = Describe("E2E", func() {

	It("should round trip to yaml", func() {
		sut := E2ETestConfig{
			ConfigShared: core.ConfigShared{
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

	It("should load tests from yaml", func() {
		type container struct {
			TestSuites []*E2ESuiteConfig `yaml:"testSuites"`
		}
		var c container
		suitePath := filepath.Join(IntegrationTestDir, "testdata/e2e/simple-http-test/suite.yaml")
		Expect(pkg.LoadYaml(suitePath, &c)).To(Succeed())
		Expect(c.TestSuites).ToNot(BeEmpty())

		config := c.TestSuites[0]
		config.FromPath = suitePath

		ctx := NewTestBosunContext()

		sut, err := NewE2ESuite(ctx, config)
		Expect(err).ToNot(HaveOccurred())

		Expect(sut.LoadTests(ctx)).ToNot(HaveOccurred())

		Expect(sut.Tests).ToNot(BeEmpty())

		test := sut.Tests[0]
		Expect(test.Variables).To(HaveKeyWithValue("url", "https://google.com"))
	})
})
