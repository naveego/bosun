package zenhub

import (
	"context"
	"encoding/json"
	"fmt"
	//"github.com/coreos/etcd/client"
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"os"
	"strconv"
	"strings"
	"time"
)

type issueSvc struct {
	Config
	//gitisvc git.issueSvc
	wrapped      issues.IssueService
	zenhub       *API
	git          git.GitWrapper
	log          *logrus.Entry
	githubClient *github.Client
	knownRepoIDs map[string]int
}

func (s issueSvc) ChangeLabels(ref issues.IssueRef, add []string, remove []string) error {
	return s.wrapped.ChangeLabels(ref, add, remove)
}

// TODO: take an injected issueSvc parameter, delegate all non zenhub things to it.
func NewIssueService(wrapped issues.IssueService, config Config, log *logrus.Entry) (issues.IssueService, error) {

	s := issueSvc{
		Config:       config,
		log:          log,
		wrapped:      wrapped,
		knownRepoIDs: map[string]int{},
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: config.GithubToken},
	)
	tc := oauth2.NewClient(context.Background(), ts)

	s.githubClient = github.NewClient(tc)

	s.zenhub = New(config.ZenhubToken, s.githubClient)

	return s, nil
}

func (s issueSvc) ctx() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	return ctx
}

func (s issueSvc) GetIssuesFromCommitsSince(org, repo, since string) ([]issues.Issue, error) {
	return nil, nil
}

func (s issueSvc) Create(issue issues.Issue, parent *issues.IssueRef) (int, error) {

	issueNumber, err := s.wrapped.Create(issue, parent)
	dumpJSON("issueNumber", issueNumber)

	// the below issueNumber used to be issue.Number
	newIssueRef := issues.NewIssueRef(issue.Org, issue.Repo, issueNumber)
	if parent != nil {
		_, _, parentIssueNumber, err := parent.Parts()
		if err != nil {
			return -1, errors.Wrap(err, "invalid parent")
		}
		err = s.AddDependency(newIssueRef, *parent, parentIssueNumber)
		if err != nil {
			return -1, errors.Wrap(err, "add dependency;"+newIssueRef.String()+", parent "+(parent).String())
		}
	}

	// Move the task and issue to In Progress column
	column := issues.ColumnInProgress
	err = s.SetProgress(newIssueRef, column)
	if err != nil {
		return -1, errors.Wrap(err, "set task progress")
	}
	parentColumn := issues.ColumnInDevelopment
	if parent != nil {
		err = s.SetProgress(*parent, parentColumn)
		if err != nil {
			return -1, errors.Wrap(err, "set parent progress")
		}
	}

	return -1, nil

}

func (s issueSvc) getRepoID(org, name string) (int, error) {

	key := org + name
	var id int
	var ok bool
	if id, ok = s.knownRepoIDs[key]; !ok {
		repo, _, err := s.githubClient.Repositories.Get(s.ctx(), org, name)
		if err != nil {
			return 0, errors.Wrap(err, "getting repo id")
		}
		id = int(repo.GetID())
		s.knownRepoIDs[key] = id
	}
	return id, nil
}

// Helper function to split IssueRef
func Split(r rune) bool {
	return r == '#' || r == '/'
}

func (s issueSvc) AddDependency(from, to issues.IssueRef, parentIssueNum int) error {

	orgFrom, repoFrom, numFrom, err := from.Parts()
	if err != nil {
		return errors.Wrap(err, "split 'from' IssueRef")
	}

	orgTo, repoTo, _, err := to.Parts()
	if err != nil {
		return errors.Wrap(err, "split 'to' IssueRef")
	}

	blockingId, err := s.getRepoID(orgFrom, repoFrom)
	if err != nil {
		return errors.Wrap(err, "getting blocking issue id")
	}
	blockedId, err := s.getRepoID(orgTo, repoTo)
	if err != nil {
		return errors.Wrap(err, "getting blocked issue id")
	}

	blockingIssue := NewDependencyIssue(blockingId, numFrom)
	//fmt.Printf("blocking di:%v, %v", blockingId, blockingNum)
	blockedIssue := NewDependencyIssue(blockedId, parentIssueNum)

	depToAdd := NewDependency(blockingIssue, blockedIssue)
	//dumpJSON("dependency", depToAdd)
	err = s.zenhub.AddDependency(&depToAdd)
	if err != nil {
		return err
	}

	//s.log.Warn("Adding dependencies not implemented yet.")
	return nil
}

func (s issueSvc) RemoveDependency(from, to issues.IssueRef) error {
	panic("implement me")
}

func (s issueSvc) SetProgress(issue issues.IssueRef, column string) error {

	org, repoName, issueNum, err := issue.Parts()

	repoId, err := s.getRepoID(org, repoName)
	if err != nil {
		return errors.Wrap(err, "set progress - get repo id")
	}

	//dumpJSON("repoId", repoId)

	issueData, err := s.zenhub.GetIssueData(repoId, issueNum)
	if err != nil {
		return errors.Wrap(err, "get issue data")
	}

	//dumpJSON("issueData", issueData)

	workspaceId := issueData.Pipeline.WorkspaceID

	// Change the workspace id if it was still in "Team One"
	if workspaceId == "5c00a1ba4b5806bc2bf951e1" {
		workspaceId = "5cee878e76309a690b06a240" // This is the workspace id of "Tasks" in naveegoinc
	}
	pipelineID, err := s.zenhub.GetPipelineID(workspaceId, repoId, column)
	if err != nil {
		return errors.Wrap(err, "set progress - get pipeline id")
	}

	err = s.zenhub.MovePipeline(workspaceId, repoId, issueNum, pipelineID)
	if err != nil {
		return errors.Wrap(err, "set progress - move issue between pipelines")
	}

	//dumpJSON("moved", pipelineID)

	return nil
}

func (s issueSvc) SplitIssueRef(issue issues.IssueRef) (string, string, int, error) {

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

func (s issueSvc) GetIssueState(issue issues.IssueRef) (string, error) {

	var issuePointer *github.Issue
	var returnedIssue github.Issue
	var state string
	org, repo, num, err := s.SplitIssueRef(issue)
	if err != nil {
		return state, errors.Wrap(err, "split IssueRef")
	}
	issuePointer, _, err = s.githubClient.Issues.Get(s.ctx(), org, repo, num)
	if err != nil {
		return state, errors.Wrap(err, "get a single issue with github api")
	}
	returnedIssue = *issuePointer
	state = *returnedIssue.State
	return state, nil
}

func (s issueSvc) GetParentRefs(issue issues.IssueRef) ([]issues.IssueRef, error) {
	return s.wrapped.GetParentRefs(issue)

}

func (s issueSvc) GetChildRefs(issue issues.IssueRef) ([]issues.IssueRef, error) {
	return s.wrapped.GetChildRefs(issue)
}

func (s issueSvc) GetIssue(ref issues.IssueRef) (issues.Issue, error) {

	issue, err := s.wrapped.GetIssue(ref)
	if err != nil {
		return issue, err
	}

	var repoID int
	if issue.GithubRepoID == nil {
		org, repo, _, _ := ref.Parts()
		repoID, err = s.getRepoID(org, repo)
		if err != nil {
			return issue, err
		}
	} else {
		repoID = int(*issue.GithubRepoID)
	}

	issueData, err := s.zenhub.GetIssueData(repoID, issue.Number)

	if err != nil {
		return issue, errors.Wrapf(err, "get zenhub issue data for %s using repoID %d and issue number %d", ref, repoID, issue.Number)
	}

	issue.Estimate = issueData.Estimate.Value
	issue.ProgressState = issueData.Pipeline.Name
	issue.MappedProgressState = s.TaskColumnMapping.ReverseLookup(issueData.Pipeline.Name)

	return issue, nil
}

func (s issueSvc) GetClosedIssue(org, repoName string) ([]int, error) {

	return nil, nil
	//
	//opt := github.IssueListByRepoOptions{
	//	State: "closed",
	//}
	//
	//closedIssues, _, err := s.github.Issues.ListByRepo(s.ctx(), org, repoName, &opt)
	//if err != nil {
	//	return nil, errors.Wrap(err, "get closed issues by repo")
	//}
	////dumpJSON("closed issues", closedIssues)
	//
	//i := 0
	//var closedIssueNumbers []int
	//for i<len(closedIssues){
	//	closedIssueNumbers = append(closedIssueNumbers, closedIssues[i].GetNumber())
	//	i++
	//}
	//
	///*pipelines, err := s.zenhub.GetPipelines(repoID)
	//if err != nil {
	//	return nil, errors.New("get pipelines")
	//}
	//dumpJSON("get pipelines", pipelines)
	//
	//var closedIssues []issues.Issue
	//for _, pipeline := range pipelines.List{
	//	if pipeline.Name != "Closed" {
	//		continue
	//	}
	//	closedIssues = pipeline.Issues
	//	break
	//} */
	//return closedIssueNumbers, nil
}

func (s issueSvc) ChildrenAllClosed(children []issues.Issue) (bool, error) {

	allClosed := true
	var issuerf issues.IssueRef

	i := 0
	for i < len(children) {
		issuerf = issues.NewIssueRef(children[i].Org, children[i].Repo, children[i].Number)
		childState, err := s.GetIssueState(issuerf)
		if err != nil {
			return allClosed, err
		}
		if childState == "open" {
			allClosed = false
		}
		i++
	}
	return allClosed, nil
}

//type Issue issues.Issue
type IssueRef issues.IssueRef

func dumpJSON(label string, data interface{}) {
	j, _ := json.MarshalIndent(data, "", "  ")
	fmt.Fprintf(os.Stderr, "%s:\n%s\n\n", label, string(j))
}
