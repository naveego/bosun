package issues

import (
	"fmt"
	"github.com/pkg/errors"
	"regexp"
	"strconv"
)

var issueRefRE = regexp.MustCompile(`([A-z0-9\-._]+)/([A-z0-9\-._]+)#(\d+)`)
var repoRefRE = regexp.MustCompile(`([A-z0-9\-._]+)/([A-z0-9\-._]+)`)

type RepoRef struct {
	Org  string
	Repo string
}

func (r RepoRef) MarshalYAML() (interface{}, error) {
	return r.String(), nil
}

func (r *RepoRef) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var p string

	err := unmarshal(&p)

	if err == nil {
		out, err := ParseRepoRef(p)
		if err == nil {
			*r = out
		}
	}

	return err
}

func (r RepoRef) String() string {
	return fmt.Sprintf("%s/%s", r.Org, r.Repo)
}

func ParseRepoRef(raw string) (RepoRef, error) {
	parts := repoRefRE.FindStringSubmatch(raw)
	if parts == nil {
		return RepoRef{}, errors.Errorf("expected org/repo, got %q", raw)
	}
	return RepoRef{
		Org:  parts[1],
		Repo: parts[2],
	}, nil
}

type IssueRef struct {
	RepoRef
	Number int
}

func NewIssueRef(org, repo string, number int) IssueRef {
	return IssueRef{
		RepoRef: RepoRef{
			Org:  org,
			Repo: repo,
		},
		Number: number,
	}
}

func ParseIssueRef(raw string) (IssueRef, error) {

	matches := issueRefRE.FindStringSubmatch(string(raw))
	if len(matches) == 0 {
		return IssueRef{}, errors.Errorf(`invalid issue ref (want 'org/repo#number', got %q)`, raw)
	}

	number, err := strconv.Atoi(matches[3])
	if err != nil {
		return IssueRef{}, errors.Errorf(`invalid issue number (want 'org/repo#number', got %q): %s`, raw, err)
	}
	return NewIssueRef(matches[1], matches[2], number), nil
}

func (s IssueRef) String() string { return fmt.Sprintf("%s/%s#%d", s.Org, s.Repo, s.Number) }

func (s IssueRef) Parts() (org string, repo string, number int, err error) {
	return s.Org, s.Repo, s.Number, nil
}
