package jira

import (
	"fmt"
	jira "github.com/andygrunwald/go-jira"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/stories"
	"github.com/pkg/errors"
	"io/ioutil"
	"regexp"
)

type Client struct {
	Config
	jira        *jira.Client
	username    string
	transitions CompiledTransitions
}

var _ stories.StoryHandler = &Client{}

func NewClient(ctx command.ExecutionContext, config Config) (*Client, error) {

	if config.AccountID == "" {
		return nil, errors.New("AccountID must be set")
	}

	compiledTransitions, err := config.Transitions.Compiled()
	if err != nil {
		return nil, errors.Wrap(err, "invalid transitions")
	}

	token, err := config.JiraToken.Resolve(ctx)
	if err != nil {
		return nil, err
	}

	tp := jira.BasicAuthTransport{
		Username: config.JiraUsername,
		Password: token,
	}

	jiraClient, err := jira.NewClient(tp.Client(), config.JiraUrl)

	if err != nil {
		return nil, err
	}

	if config.IDPattern == "" {
		config.IDPattern = ".*"
	}

	return &Client{
		jira:        jiraClient,
		Config:      config,
		transitions: compiledTransitions,
	}, nil
}

func (c *Client) GetStory(id string) (*stories.Story, error) {
	jiraStory, _, err := c.jira.Issue.Get(id, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get story with ID %q", id)
	}

	fields := jiraStory.Fields

	story := &stories.Story{
		ID:            id,
		URL:           jiraStory.Self,
		Reference:     id,
		Body:          fields.Description,
		Estimate:      fmt.Sprint(fields.TimeEstimate),
		Title:         fields.Summary,
		Labels:        fields.Labels,
		IsClosed:      false,
		ProviderState: jiraStory,
	}

	if fields.Status != nil {
		story.ProgressState = fields.Status.Name
	}
	if fields.Assignee != nil {
		story.Assignee = fields.Assignee.Name
	}
	if fields.Epic != nil {
		story.Epics = []string{fields.Epic.Name}
	}

	return story, nil
}

func (c *Client) HandleEvent(event *stories.ValidatedEvent) error {

	var err error
	var res *jira.Response
	if event.Story() == nil {
		s, storyErr := c.GetStory(event.StoryID())
		if storyErr != nil {
			return storyErr
		}
		event.SetStory(s)
	}

	_, err = c.jira.Issue.UpdateAssignee(event.StoryID(), &jira.User{AccountID: c.AccountID})
	if err != nil {
		return errors.Wrapf(detailedErr(res, err), "set assignee on %q to %q", event.StoryID(), c.username)
	}

	err = c.doTransition(event.StoryID(), c.transitions.InDevelopment)
	if err != nil {
		return err
	}

	payload := event.Payload()

	story := event.Story().ProviderState.(*jira.Issue)

	switch p := payload.(type) {
	case stories.EventBranchCreated:

		err2, done := c.handleBranchCreated(event, p, story)
		if done {
			return err2
		}
	}

	return nil
}

func (c *Client) handleBranchCreated(event *stories.ValidatedEvent, payload stories.EventBranchCreated, story *jira.Issue) (error, bool) {
	subtaskName := SubtaskName(event.Issue(), payload.Branch)
	var subtask = &jira.Issue{
		Fields: &jira.IssueFields{
			Parent:   &jira.Parent{Key: story.Key, ID: story.ID},
			Project:  story.Fields.Project,
			Summary:  subtaskName.String(),
			Type:     jira.IssueType{ID: "5"},
			Assignee: &jira.User{AccountID: c.AccountID},
			Description: fmt.Sprintf(`This subtask tracks development in the %s repo.

The branch is %s

Link: %s
`, event.Issue().RepoRef.String(), payload.Branch, event.URL()),
		},
	}
	var err error
	var res *jira.Response

	subtask, res, err = c.jira.Issue.Create(subtask)
	if err != nil {
		return errors.Wrapf(detailedErr(res, err), "create subtask documenting branch %q for story with key %q, id %q", subtaskName, story.Key, story.ID), true
	}

	err = c.doTransition(subtask.ID, c.transitions.InDevelopment)
	return nil, false
}

type RepoBranchComment string

func SubtaskName(repoRef issues.IssueRef, branch string) RepoBranchComment {
	return RepoBranchComment(fmt.Sprintf("%s|%s", repoRef, branch))
}
func (r RepoBranchComment) String() string { return string(r) }

func (c *Client) doTransition(storyID string, re *regexp.Regexp) error {

	var transitionID string
	possibleTransitions, _, _ := c.jira.Issue.GetTransitions(storyID)
	for _, v := range possibleTransitions {
		if re.MatchString(v.Name) {
			transitionID = v.ID
			break
		}
	}

	if transitionID == "" {
		return errors.Errorf("no transition matched %s", re)
	}

	res, err := c.jira.Issue.DoTransition(storyID, transitionID)
	return detailedErr(res, err)
}

func detailedErr(res *jira.Response, err error) error {
	if err == nil {
		return nil
	}

	body, _ := ioutil.ReadAll(res.Body)

	return errors.Errorf("jira: %s; response body: %s", err.Error(), string(body))
}
