package stories

import (
	"fmt"
	"github.com/pkg/errors"
	"regexp"
	"strconv"
)

type Story struct {
	Number    int
	Repo      string
	Body      string
	Assignees []string
	Milestone string
	Estimate  int
	Epics     []string
	Releases  []string
}

var storyRefRE = regexp.MustCompile(`(.+)/(.+)#(\d+)`)

type StoryRef string

func NewStoryRef(org, repo string, number int) StoryRef {
	return StoryRef(fmt.Sprintf("%s/%s#%d", org, repo, number))
}

func (s StoryRef) String() string { return string(s) }
func (s StoryRef) Valid() error {
	if string(s) == "" {
		return errors.New("story ref is empty")
	}
	if storyRefRE.MatchString(string(s)) {
		return nil
	}
	return errors.Errorf(`invalid story ref (want 'org/repo#number', got %q)`, s)
}

func (s StoryRef) Parts() (org string, repo string, number int, err error) {
	matches := storyRefRE.FindStringSubmatch(string(s))
	if len(matches) == 0 {
		return "", "", 0, errors.Errorf(`invalid story ref (want 'org/repo#number', got %q)`, s)
	}

	number, err = strconv.Atoi(matches[3])
	if err != nil {
		return "", "", 0, errors.Errorf(`invalid story number (want 'org/repo#number', got %q): %s`, s, err)
	}
	return matches[1], matches[2], number, nil
}

type StoryRelationship string

type StoryService interface {
	Create(story Story) error
	AddDependency(from, to StoryRef) error
	RemoveDependency(from, to StoryRef) error
	SetProgress(story StoryRef, column string) error
	GetParents(story StoryRef) ([]Story, error)
	GetChildren(story StoryRef) ([]Story, error)
}
