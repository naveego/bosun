package issues

import (
	"github.com/naveego/bosun/pkg/util/multierr"
	"regexp"
	"strings"
)

type Issue struct {
	Number          int      `yaml:"number,omitempty"`
	Org             string   `yaml:"org,omitempty"`
	Repo            string   `yaml:"repo,omitempty"`
	Body            string   `yaml:"body,omitempty"`
	Assignee        string   `yaml:"assignee,omitempty"`
	Assignees       []string `yaml:"assignees,omitempty"`
	Milestone       *int     `yaml:"milestone,omitempty"`
	Estimate        int      `yaml:"estimate,omitempty"`
	Epics           []string `yaml:"epics,omitempty"`
	Releases        []string `yaml:"releases,omitempty"`
	Title           string   `yaml:"title,omitempty"`
	ProgressState   string   `yaml:"progressState,omitempty"`
	ProgressStateID int      `yaml:"progressStateId,omitempty"`
	BranchPattern   string   `yaml:"branchPattern,omitempty"`
	Labels          []string `yaml:"labels,omitempty"`

	IsClosed bool `yaml:"isClosed,omitempty"`

	GithubRepoID        *int64 `yaml:"githubRepoId,omitempty"`
	MappedProgressState string `yaml:"mappedProgressState,omitempty"`
}

var slugRE = regexp.MustCompile(`\W+`)

func (i Issue) Slug() string {
	slug := slugRE.ReplaceAllString(strings.ToLower(i.Title), "-")
	for len(slug) > 30 {
		cutoff := strings.LastIndex(slug, "-")
		if cutoff < 0 || cutoff > 30 {
			return slug
		}
		slug = slug[:cutoff]
	}
	return slug
}

func (i Issue) Ref() IssueRef {
	return NewIssueRef(i.Org, i.Repo, i.Number)
}

func (i Issue) RefPtr() *IssueRef {
	r := NewIssueRef(i.Org, i.Repo, i.Number)
	return &r
}

type Estimate struct {
	Value int
}

type IssueService interface {
	// Assign the user who created the task, attach body and milestone
	Create(issue Issue) (int, error)
	// Add dependency relationship : the newly created task should be a dependency of the issue issue on the ZenHub board
	AddDependency(from, to IssueRef, parentIssueNum int) error
	RemoveDependency(from, to IssueRef) error
	// Put the task and depending issue into In Progress column on the ZenHub board
	SetProgress(issue IssueRef, column string) error
	ChangeLabels(ref IssueRef, add []string, remove []string) error
	GetParentRefs(issue IssueRef) ([]IssueRef, error)
	GetChildRefs(issue IssueRef) ([]IssueRef, error)
	GetIssue(issue IssueRef) (Issue, error)
	GetClosedIssue(org, repoName string) ([]int, error)
	// Check if a story's children are all closed before moving it to Waiting for Merge
}

const (
	ColumnInDevelopment    = "In Development"
	ColumnWaitingForMerge  = "Ready for Merge"
	ColumnWaitingForDeploy = ""
	ColumnInProgress       = "In Progress"
	ColumnWaitingForUAT    = "UAT"
	ColumnDone             = "Done"
	ColumnClosed           = "Closed"
)

type ColumnMapping map[string]string

func (c ColumnMapping) ReverseLookup(name string) string {
	for k, v := range c {
		if v == name {
			return k
		}
	}
	return "NotFound"
}

func GetIssuesFromRefsErr(svc IssueService, refs []IssueRef, err error) ([]Issue, error) {
	if err != nil {
		return nil, err
	}

	return GetIssuesFromRefs(svc, refs)
}

func GetIssuesFromRefs(svc IssueService, refs []IssueRef) ([]Issue, error) {
	outCh := make(chan interface{}, len(refs))
	for _, ref := range refs {
		go func(ref IssueRef) {
			issue, err := svc.GetIssue(ref)
			if err != nil {
				outCh <- err
			} else {
				outCh <- issue
			}
		}(ref)
	}

	var out []Issue
	errs := multierr.New()
	for range refs {
		x := <-outCh
		switch t := x.(type) {
		case Issue:
			out = append(out, t)
		case error:
			errs.Collect(t)
		}
	}
	return out, errs.ToError()
}
