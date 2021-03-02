package stories

import (
	"github.com/naveego/bosun/pkg/issues"
	"github.com/pkg/errors"
)

type EventBranchCreated struct {
	Branch string
}
type EventPRCreated struct {
	FromBranch string
	ToBranch   string
}
type EventPRApproved struct {
	FromBranch string
	ToBranch   string
}
type EventPRMerged struct {
	FromBranch string
	ToBranch   string
}

type Event struct {
	Payload interface{}
	StoryID string
	Story   *Story
	URL     string
	Issue   *issues.IssueRef
}

func (e Event) Validated() (*ValidatedEvent, error) {
	v := &ValidatedEvent{e: e}
	if e.Payload == nil {
		return v, errors.New("payload is required")
	}
	if e.Story != nil {
		e.StoryID = e.Story.ID
	}
	if e.Issue == nil {
		return v, errors.New("issue is required")
	}
	return v, nil
}

type ValidatedEvent struct {
	e Event
}

func (v *ValidatedEvent) SetStory(story *Story) {
	v.e.Story = story
}
func (v ValidatedEvent) Payload() interface{} {
	return v.e.Payload
}
func (v ValidatedEvent) StoryID() string {
	return v.e.StoryID
}
func (v ValidatedEvent) Story() *Story {
	return v.e.Story
}
func (v ValidatedEvent) URL() string {
	return v.e.URL
}
func (v ValidatedEvent) Issue() issues.IssueRef {
	return *v.e.Issue
}
