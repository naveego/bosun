package issues

import (
	"fmt"
	"github.com/pkg/errors"
	"regexp"
	"strconv"
)

type Issue struct {
	Number          int
	Org             string
	Repo            string
	Body            string
	Assignee        string
	Assignees       []string
	Milestone       *int
	Estimate        int
	Epics           []string
	Releases        []string
	Title           string
	ProgressState   string
	ProgressStateID int

	GithubRepoID *int
}

var issueRefRE = regexp.MustCompile(`(.+)/(.+)#(\d+)`)

type IssueRef string

func NewIssueRef(org, repo string, number int) IssueRef {
	return IssueRef(fmt.Sprintf("%s/%s#%d", org, repo, number))
}

func (s IssueRef) String() string { return string(s) }
func (s IssueRef) Valid() error {
	if string(s) == "" {
		return errors.New("issue ref is empty")
	}
	if issueRefRE.MatchString(string(s)) {
		return nil
	}
	return errors.Errorf(`invalid issue ref (want 'org/repo#number', got %q)`, s)
}

func (s IssueRef) Parts() (org string, repo string, number int, err error) {
	matches := issueRefRE.FindStringSubmatch(string(s))
	if len(matches) == 0 {
		return "", "", 0, errors.Errorf(`invalid issue ref (want 'org/repo#number', got %q)`, s)
	}

	number, err = strconv.Atoi(matches[3])
	if err != nil {
		return "", "", 0, errors.Errorf(`invalid issue number (want 'org/repo#number', got %q): %s`, s, err)
	}
	return matches[1], matches[2], number, nil
}

type IssueService interface {
	// Assign the user who created the task, attach body and milestone
	Create(issue Issue, parent *IssueRef) error
	// Add dependency relationship : the newly created task should be a dependency of the issue issue on the ZenHub board
	AddDependency(from, to IssueRef, parentIssueNum int) error
	RemoveDependency(from, to IssueRef) error
	// Put the task and depending issue into In Progress column on the ZenHub board
	SetProgress(issue IssueRef, column string) error
	GetParents(issue IssueRef) ([]Issue, error)
	GetChildren(issue IssueRef) ([]Issue, error)
	GetIssue(ref IssueRef) (Issue, error)
	// Check if a story's children are all closed before moving it to Waiting for Merge
	ChildrenAllClosed(children []Issue) (bool, error)
}
