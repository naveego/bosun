package issues_test

import (
	"github.com/naveego/bosun/pkg/issues"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

func TestIssues(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Issues Suite")
}

var _ = Describe("Issue", func() {
	DescribeTable("should create slug", func(input, expected string) {
		Expect(issues.Issue{Title: input}.Slug()).To(Equal(expected))
	},
		Entry("simple", "A basic title", "a-basic-title"),
		Entry("simple", "A long title with too many words", "a-long-title-with-too-many"),
		Entry("simple", "smoooooooooshedwordswithnogapsthatistoolong", "smoooooooooshedwordswithnogapsthatistoolong"),
	)
})
