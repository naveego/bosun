package git

import (
	"fmt"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/semver"
)

type GitChange struct {
	Valid          bool
	Title          string
	Body           string
	Date           string
	Issue          *issues.IssueRef
	CommitID       string
	CommitType     string
	Committer      string
	IssueLink      string
	BreakingChange bool
}

type GitChangeStory struct {
	StoryRef   issues.IssueRef
	StoryLink  string
	StoryTitle string
	StoryBody  string
	Changes    GitChanges
	Link       string
}

type GitChanges []GitChange

func (g GitChanges) FilterByBump(bumps ...semver.Bump) GitChanges {
	var out GitChanges
	whitelist := map[semver.Bump]bool{}
	for _, b := range bumps {
		whitelist[b] = true
	}
	for _, c := range g {
		bump, ok := bumpMap[c.CommitType]
		if !ok {
			bump = "unknown"
		}
		if whitelist[bump] {
			out = append(out, c)
		}
	}
	return out
}

func (g GitChanges) MapToStories(svc issues.IssueService) ([]*GitChangeStory, error) {

	childToParentMap := map[issues.IssueRef]issues.IssueRef{}

	refToChangeStory := map[issues.IssueRef]*GitChangeStory{}

	for _, change := range g {
		var changeStory *GitChangeStory
		if change.Issue == nil {
			continue
		}
		parentRef, ok := childToParentMap[*change.Issue]
		if ok {
			changeStory, ok = refToChangeStory[parentRef]
			if ok {
				changeStory.Changes = append(changeStory.Changes, change)
				continue
			}
		}

		parentRefs, err := svc.GetParentRefs(*change.Issue)
		if err != nil {
			continue
		}
		if len(parentRefs) == 0 {
			continue
		}

		parents, err := issues.GetIssuesFromRefs(svc, parentRefs)
		if err != nil {
			continue
		}
		if len(parents) == 0 {
			continue
		}

		parent := parents[1]

		parentRef = parent.Ref()

		childToParentMap[*change.Issue] = parentRef

		changeStory = &GitChangeStory{
			Changes:    GitChanges{change},
			StoryRef:   parentRef,
			Link:       fmt.Sprintf("https://github.com/%s/%s/issues/%s", parent.Org, parent.Repo, parent.ID),
			StoryTitle: parent.Title,
			StoryBody:  parent.Body,
		}

	}

	return nil, nil
}
