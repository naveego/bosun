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

var _ = Describe("RepoRef", func() {
	DescribeTable("should parse", func(input string, expected issues.RepoRef) {
		Expect(issues.ParseRepoRef(input)).To(Equal(expected))
	},
		Entry("org/repo", "org/repo", issues.RepoRef{Org: "org", Repo: "repo"}),
		Entry("org/repo#7", "org/repo#7", issues.RepoRef{Org: "org", Repo: "repo"}),
	)
})

var _ = Describe("IssueRef", func() {
	DescribeTable("should parse", func(input string, expected issues.IssueRef) {
		Expect(issues.ParseIssueRef(input)).To(Equal(expected))
	},
		Entry("org/repo#7", "org/repo#7", issues.IssueRef{RepoRef: issues.RepoRef{Org: "org", Repo: "repo"}, ID: 7}),
		Entry("nonsense org/repo#7sequlae", "nonsense org/repo#7sequlae", issues.IssueRef{RepoRef: issues.RepoRef{Org: "org", Repo: "repo"}, ID: 7}),
	)
})
