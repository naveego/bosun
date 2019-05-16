package bosun_test

import (
	"github.com/ghodss/yaml"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/semver"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Platform", func() {
	It("should round-trip version", func() {

		sut := bosun.ReleaseMetadata{
			Name:    "Deploy",
			Version: semver.New("0.1.4-alpha"),
		}
		y, err := yaml.Marshal(sut)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(y)).To(ContainSubstring("0.1.4-alpha"))
		var actual bosun.ReleaseMetadata
		Expect(yaml.Unmarshal(y, &actual)).To(Succeed())
		Expect(actual).To(BeEquivalentTo(sut))
	})
})
