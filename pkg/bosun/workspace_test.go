package bosun_test

import (
	. "github.com/naveego/bosun/pkg/bosun"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Workspace", func() {
	It("should set defaults", func() {

		sut, err := LoadWorkspace("__test_data/empty_workspace.yaml")
		Expect(err).ToNot(HaveOccurred())

		Expect(sut.Minikube.DiskSize).To(Equal("40g"))
	})
})
