package test_test

import (
	"github.com/naveego/bosun/pkg/bosun"
	. "github.com/naveego/bosun/test"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestTest(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Test Suite")
}

var _ = Describe("app commands", func() {
	var sut *bosun.Bosun
	BeforeEach(func() {
		sut = NewTestInstance()
	})

	Describe("list", func() {
		It("should list apps", func() {

			apps := sut.GetApps()
			Expect(apps).To(HaveLen(2))

		})
	})
})
