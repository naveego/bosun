package bosun_test

import (
	. "github.com/naveego/bosun/pkg/bosun"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("AppActions", func() {

	Describe("HTTPAction", func() {
		It("should execute request", func() {

			sut := HTTPAction{
				Method: "GET",
				URL:    "https://google.com",
			}

			ctx := NewTestBosunContext()

			Expect(sut.Execute(ctx)).To(Succeed())
		})
	})
})
