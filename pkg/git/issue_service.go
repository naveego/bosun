package git

import (
	"context"
	"encoding/json"
	"fmt"
	// "github.com/naveego/bosun/pkg/git"

	// "github.com/coreos/etcd/client"
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type IssueService struct {
	Config
	github *github.Client
	//git    GitWrapper
	log *logrus.Entry
}

func (s IssueService) ChangeLabels(ref issues.IssueRef, add []string, remove []string) error {

	org, repo, id, _ := ref.Parts()
	number, err := strconv.Atoi(id)
	if err != nil {
		return err
	}
	if len(add) > 0 {
		_, _, addError := s.github.Issues.AddLabelsToIssue(stdctx(), org, repo, number, add)
		if addError != nil {
			return addError
		}
	}

	for _, label := range remove {
		_, removeErr := s.github.Issues.RemoveLabelForIssue(stdctx(), org, repo, number, label)
		if removeErr != nil {
			return removeErr
		}
	}

	return nil
}

func NewIssueService(config Config, log *logrus.Entry) (IssueService, error) {

	s := IssueService{
		Config: config,
		log:    log,
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: config.GithubToken},
	)
	tc := oauth2.NewClient(context.Background(), ts)

	s.github = github.NewClient(tc)

	return s, nil
}

func (s IssueService) ctx() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	return ctx
}

func (s IssueService) SetProgress(issue issues.IssueRef, column string) error {
	return nil
}

func (s IssueService) Create(issue issues.Issue) (string, error) {

	log := s.log.WithField("title", issue.Title)

	user, _, err := s.github.Users.Get(s.ctx(), "")
	if err != nil {
		return "unknown", err
	}

	issueRequest := &github.IssueRequest{
		Title:    github.String(issue.Title),
		Assignee: user.Login,
	}

	s.log.WithField("title", issue.Title).Info("Setting assignee")

	issueRequest.Body = &issue.Body

	issueResponse, _, err := s.github.Issues.Create(s.ctx(), issue.Org, issue.Repo, issueRequest)
	if err != nil {
		return "unknown", errors.Wrap(err, "creating issue")
	}

	issueResponse.Repository.GetID()

	issue.ID = fmt.Sprint(issueResponse.GetNumber())
	log = log.WithField("issue", issue.ID)

	log.Info("Created issue.")

	return issue.ID, nil
}

// Helper function to split IssueRef
func Split(r rune) bool {
	return r == '#' || r == '/'
}

func (s IssueService) AddDependency(from, to issues.IssueRef, parentIssueNum string) error {

	panic("implement me")
}

func (s IssueService) GetRepoIdbyName(org, repoName string) (int, error) {

	repo, _, err := s.github.Repositories.Get(s.ctx(), org, repoName)
	if err != nil {
		return 0, errors.Wrap(err, "getting repo id")
	}

	repoId := int(repo.GetID())
	return repoId, nil
}

func (s IssueService) RemoveDependency(from, to issues.IssueRef) error {
	panic("implement me")
}

func (s IssueService) SplitIssueRef(issue issues.IssueRef) (string, string, int, error) {

	issueString := issue.String()
	issueSplitted := strings.FieldsFunc(issueString, Split)
	org := issueSplitted[0]
	repoIdString := issueSplitted[1]

	issueNum, err := strconv.Atoi(issueSplitted[2])
	if err != nil {
		return "", "", 0, err
	}
	return org, repoIdString, issueNum, nil
}

func (s IssueService) GetIssueState(issue issues.IssueRef) (string, error) {

	var issuePointer *github.Issue
	var returnedIssue github.Issue
	var state string
	org, repo, num, err := s.SplitIssueRef(issue)
	if err != nil {
		return state, errors.Wrap(err, "split IssueRef")
	}
	issuePointer, _, err = s.github.Issues.Get(s.ctx(), org, repo, num)
	if err != nil {
		return state, errors.Wrap(err, "get a single issue with github api")
	}
	returnedIssue = *issuePointer
	state = *returnedIssue.State
	return state, nil
}

func (s IssueService) GetParentRefs(issue issues.IssueRef) ([]issues.IssueRef, error) {

	org, repo, id, err := issue.Parts()
	//repoId, err := s.getRepoID(org, repo)
	if err != nil {
		return nil, err
	}

	number, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}

	githubIssue, _, err := s.github.Issues.Get(s.ctx(), org, repo, number)
	if err != nil {
		return nil, err
	}
	body := githubIssue.GetBody()

	out, err := extractIssueRefsFromString(body, "required by")
	return out, err

}

func (s IssueService) GetChildRefs(issue issues.IssueRef) ([]issues.IssueRef, error) {

	org, repo, id, err := issue.Parts()
	if err != nil {
		return nil, errors.Wrap(err, "parts of issue")
	}
	number, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}
	// find children from an issue's body
	githubIssuePointer, _, err := s.github.Issues.Get(s.ctx(), org, repo, number)
	if err != nil {
		return nil, err
	}
	githubIssue := *githubIssuePointer
	issueBody := githubIssue.GetBody()
	out, err := extractIssueRefsFromString(issueBody, "requires")
	return out, err
}

func (s IssueService) GetIssue(ref issues.IssueRef) (issues.Issue, error) {

	org, repo, id, err := ref.Parts()
	number, err := strconv.Atoi(id)
	if err != nil {
		return issues.Issue{}, err
	}

	issue, _, err := s.github.Issues.Get(s.ctx(), org, repo, number)
	if err != nil {
		return issues.Issue{}, errors.Wrap(err, "parts of issue")
	}

	out := issues.Issue{
		Repo:  repo,
		Org:   org,
		ID:    id,
		Title: issue.GetTitle(),
		Body:  issue.GetBody(),
	}

	for _, label := range issue.Labels {
		out.Labels = append(out.Labels, label.GetName())
	}

	if issue.Assignee != nil {
		out.Assignee = issue.Assignee.GetLogin()
	}
	for _, user := range issue.Assignees {
		if user != nil {
			out.Assignees = append(out.Assignees, user.GetLogin())
		}
	}

	return out, nil
}

func extractIssueRefsFromString(body string, prefix string) ([]issues.IssueRef, error) {
	var out []issues.IssueRef
	var err error
	regForParent := regexp.MustCompile(prefix + `\s?(([A-z\-]+)/(.*)+#([0-9]+))`)
	parents := regForParent.FindAllStringSubmatch(body, -1)
	if len(parents) < 1 {
		return out, nil
	}

	for _, parent := range parents {
		var parentRef issues.IssueRef
		parentRef, err = issues.ParseIssueRef(parent[1])
		if err != nil {
			return nil, err
		}
		out = append(out, parentRef)
	}

	return out, nil
}

func (s IssueService) GetClosedIssue(org, repoName string) ([]int, error) {

	opt := github.IssueListByRepoOptions{
		State: "closed",
	}

	closedIssues, _, err := s.github.Issues.ListByRepo(s.ctx(), org, repoName, &opt)
	if err != nil {
		return nil, errors.Wrap(err, "get closed issues by repo")
	}
	//dumpJSON("closed issues", closedIssues)

	i := 0
	var closedIssueNumbers []int
	for i < len(closedIssues) {
		closedIssueNumbers = append(closedIssueNumbers, closedIssues[i].GetNumber())
		i++
	}

	/*pipelines, err := s.zenhub.GetPipelines(repoID)
	if err != nil {
		return nil, errors.New("get pipelines")
	}
	dumpJSON("get pipelines", pipelines)

	var closedIssues []issues.Issue
	for _, pipeline := range pipelines.List{
		if pipeline.Name != "Closed" {
			continue
		}
		closedIssues = pipeline.Issues
		break
	} */
	return closedIssueNumbers, nil
}

//type Issue issues.Issue
type IssueRef issues.IssueRef

func dumpJSON(label string, data interface{}) {
	if pkg.Log != nil && pkg.Log.Logger.IsLevelEnabled(logrus.DebugLevel) {
		j, _ := json.MarshalIndent(data, "", "  ")
		fmt.Fprintf(os.Stderr, "%s:\n%s\n\n", label, string(j))
	}
}
