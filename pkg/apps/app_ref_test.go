package apps_test

import (
	. "github.com/naveego/bosun/pkg/apps"
	"github.com/naveego/bosun/pkg/semver"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

var _ = Describe("AppRef", func() {

	It("should round trip app ref", func(){

		input := AppRef{
			Name: "test",
			Provider: "provider",
			Version: semver.MustParse("3.2.1"),
			ProviderMetadata: map[string]interface{}{
				"value": "test",
				"int": 42,
			},
		}

		y, err := yaml.Marshal(input)

		Expect(err).ToNot(HaveOccurred())

		var actual AppRef
		Expect(yaml.Unmarshal(y, &actual)).To(Succeed())

		Expect(actual).To(BeEquivalentTo(input))

	})
})
