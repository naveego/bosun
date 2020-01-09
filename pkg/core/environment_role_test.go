package core_test

import (
	"github.com/naveego/bosun/pkg/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/naveego/bosun/pkg/core"
)

var _ = Describe("EnvironmentRole", func() {

	It("should marshal correctly", func() {
		type Example struct {
			Roles EnvironmentRoles `yaml:"roles"`
		}

		sut := &Example{
			Roles: EnvironmentRoles{"role1", "role2"},
		}

		y, err := yaml.MarshalString(sut)
		Expect(err).ToNot(HaveOccurred())

		var actual Example
		Expect(yaml.Unmarshal([]byte(y), &actual)).To(Succeed())

		Expect(actual.Roles).To(ConsistOf(EnvironmentRole("role1"), EnvironmentRole("role2")))

	})

})
