package issues

import (
	"fmt"
	"github.com/pkg/errors"
	"regexp"
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
		out, parseErr := ParseRepoRef(p)
		if parseErr == nil {
			*r = out
		}
	}

	return err
}

func (r RepoRef) String() string {
	return fmt.Sprintf("%s/%s", r.Org, r.Repo)
}

func (r RepoRef) OrgAndRepo() (string, string) {
	return r.Org, r.Repo
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
	ID string
}

func (f *IssueRef) MarshalYAML() (interface{}, error) {
	if f == nil {
		return nil, nil
	}
	return f.String(), nil
}

func (f *IssueRef) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var raw string

	err := unmarshal(&raw)
	if err != nil {
		return err
	}

	*f, err = ParseIssueRef(raw)
	return err
}

func NewIssueRef(org, repo string, id string) IssueRef {
	return IssueRef{
		RepoRef: RepoRef{
			Org:  org,
			Repo: repo,
		},
		ID: id,
	}
}

func ParseIssueRef(raw string) (IssueRef, error) {

	matches := issueRefRE.FindStringSubmatch(string(raw))
	if len(matches) == 0 {
		return IssueRef{}, errors.Errorf(`invalid issue ref (want 'org/repo#number', got %q)`, raw)
	}

	return NewIssueRef(matches[1], matches[2], matches[3]), nil
}

func (s IssueRef) String() string { return fmt.Sprintf("%s/%s#%d", s.Org, s.Repo, s.ID) }

func (s IssueRef) Parts() (org string, repo string, number int, err error) {
	return s.Org, s.Repo, s.ID, nil
}
