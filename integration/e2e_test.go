// +build integration

// This in an integration test which has dependencies on mongodb and vault, among other things.
// It needs to be run with a bosun context available and a valid environment selected.

package integration_test

import (
	. "github.com/naveego/bosun/pkg/bosun"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("E2E Integration", func() {

	It("should run mongo-based test successfully", func() {
		b := GetBosun()

		suite, err := b.GetTestSuite("MongoTestSuite")
		Expect(err).ToNot(HaveOccurred())

		ctx := b.NewContext()

		doneCh := make(chan struct{})
		var results []*E2EResult
		go func() {
			GinkgoRecover()
			defer close(doneCh)
			results, err = suite.Run(ctx)
			Expect(err).ToNot(HaveOccurred())
		}()

		Eventually(doneCh, 10*time.Second).Should(BeClosed())

		Expect(results).ToNot(BeEmpty())

		result := results[0]
		Expect(result.Passed).To(BeTrue())
	})

	Describe("when running tests", func() {

		It("should pass test when it passes", func() {
			b := GetBosun()

			suite, err := b.GetTestSuite("SimpleHttpTest")
			Expect(err).ToNot(HaveOccurred())

			ctx := b.NewContext()

			doneCh := make(chan struct{})
			var results []*E2EResult
			go func() {
				GinkgoRecover()
				defer close(doneCh)
				results, err = suite.Run(ctx, "pass-test")
				Expect(err).ToNot(HaveOccurred())
			}()

			Eventually(doneCh, 10*time.Second).Should(BeClosed())

			Expect(results).ToNot(BeEmpty())

			result := results[0]
			Expect(result.Passed).To(BeTrue())
		})

		It("should fail test when it fails", func() {
			b := GetBosun()

			suite, err := b.GetTestSuite("SimpleHttpTest")
			Expect(err).ToNot(HaveOccurred())

			ctx := b.NewContext()

			doneCh := make(chan struct{})
			var results []*E2EResult
			go func() {
				GinkgoRecover()
				defer close(doneCh)
				results, err = suite.Run(ctx, "fail-test")
				Expect(err).ToNot(HaveOccurred())
			}()

			Eventually(doneCh, 10*time.Second).Should(BeClosed())

			Expect(results).ToNot(BeEmpty())

			result := results[0]
			Expect(result.Passed).To(BeFalse())
			Expect(result.Error).To(ContainSubstring("after 3 attempts"))
		})
	})

})
