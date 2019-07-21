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

type IssueService struct {
	Config
	//gitisvc git.IssueService
	wrapped      issues.IssueService
	zenhub       *API
	git          git.GitWrapper
	log          *logrus.Entry
	githubClient *github.Client
}

// TODO: take an injected IssueService parameter, delegate all non zenhub things to it.
func NewIssueService(wrapped issues.IssueService, config Config, log *logrus.Entry) (IssueService, error) {

	s := IssueService{
		Config:  config,
		log:     log,
		wrapped: wrapped,
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: config.GithubToken},
	)
	tc := oauth2.NewClient(context.Background(), ts)

	s.githubClient = github.NewClient(tc)

	s.zenhub = New(config.ZenhubToken, s.githubClient)

	return s, nil
}

func (s IssueService) ctx() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	return ctx
}

func (s IssueService) GetIssuesFromCommitsSince(org, repo, since string) ([]issues.Issue, error) {
	return nil, nil
}

func (s IssueService) Create(issue issues.Issue, parent *issues.IssueRef) (int, error) {

	issueNumber, err := s.wrapped.Create(issue, parent)
	dumpJSON("issueNumber", issueNumber)

	// the below issueNumber used to be issue.Number
	newIssueRef := issues.NewIssueRef(issue.Org, issue.Repo, issueNumber)
	if parent != nil {
		_, _, parentIssueNumber, err := parent.Parts()
		if err != nil {
			return -1, errors.Wrapf(err, "invalid parent")
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

func (s IssueService) GetRepoIdbyName(org, repoName string) (int, error) {

	repo, _, err := s.githubClient.Repositories.Get(s.ctx(), org, repoName)
	if err != nil {
		return 0, errors.Wrap(err, "getting repo id")
	}

	repoId := int(repo.GetID())
	return repoId, nil
}

// Helper function to split IssueRef
func Split(r rune) bool {
	return r == '#' || r == '/'
}

func (s IssueService) AddDependency(from, to issues.IssueRef, parentIssueNum int) error {

	orgFrom, repoFrom, numFrom, err := from.Parts()
	if err != nil {
		return errors.Wrap(err, "split 'from' IssueRef")
	}

	orgTo, repoTo, _, err := to.Parts()
	if err != nil {
		return errors.Wrap(err, "split 'to' IssueRef")
	}

	blockingId, err := s.GetRepoIdbyName(orgFrom, repoFrom)
	if err != nil {
		return errors.Wrap(err, "getting blocking issue id")
	}
	blockedId, err := s.GetRepoIdbyName(orgTo, repoTo)
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

func (s IssueService) RemoveDependency(from, to issues.IssueRef) error {
	panic("implement me")
}

func (s IssueService) SetProgress(issue issues.IssueRef, column string) error {

	org, repoName, issueNum, err := issue.Parts()

	repoId, err := s.GetRepoIdbyName(org, repoName)
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
	issuePointer, _, err = s.githubClient.Issues.Get(s.ctx(), org, repo, num)
	if err != nil {
		return state, errors.Wrap(err, "get a single issue with github api")
	}
	returnedIssue = *issuePointer
	state = *returnedIssue.State
	return state, nil
}

func (s IssueService) GetParents(issue issues.IssueRef) ([]issues.Issue, error) {

	var parentIssues []issues.Issue
	org, repo, number, err := issue.Parts()
	repoId, err := s.GetRepoIdbyName(org, repo)
	if err != nil {
		return nil, err
	}

	deps, err := s.zenhub.GetDependencies(repoId, number)
	if err != nil {
		return nil, err
	}
	//dumpJSON("deps", deps)

	// Make sure we find parent story in "stories" repo if the task is not in that repo
	storiesOrg := "naveegoinc"
	storiesRepo := "stories"
	storiesId, err := s.GetRepoIdbyName(storiesOrg, storiesRepo)
	if err != nil {
		return nil, errors.Wrap(err, "find parent in naveegoinc/stories")
	}
	depsInStories, err := s.zenhub.GetDependencies(storiesId, number)
	if err != nil {
		return nil, err
	}

	if len(deps) < 1 && len(depsInStories) < 1 { // no parent found
		return nil, nil
	}

	for _, dep := range deps {
		if dep.Blocking.RepoID != repoId || dep.Blocking.IssueNumber != number {
			continue
		}

		parentData, err := s.zenhub.GetIssueData(dep.Blocked.RepoID, dep.Blocked.IssueNumber)
		if err != nil {
			return nil, errors.Wrap(err, "get parent data")
		}

		// TODO: get repo name from github?

		currIssue := issues.Issue{
			Org:                 org,
			Repo:                repo,
			Number:              dep.Blocked.IssueNumber,
			GithubRepoID:        &dep.Blocked.RepoID,
			ProgressState:       parentData.Pipeline.Name,
			MappedProgressState: s.Config.StoryColumnMapping.ReverseLookup(parentData.Pipeline.Name),
			IsClosed:            parentData.Pipeline.Name == s.Config.StoryColumnMapping[issues.ColumnClosed],
		}

		if currIssue.Number != 0 {
			parentIssues = append(parentIssues, currIssue)
		}

	}

	//dumpJSON("parentIssues 0", parentIssues)

	for _, depis := range depsInStories {
		if depis.Blocking.RepoID != repoId || depis.Blocking.IssueNumber != number {
			continue
		}

		data, err := s.zenhub.GetIssueData(depis.Blocked.RepoID, depis.Blocked.IssueNumber)
		if err != nil {
			return nil, errors.Wrap(err, "add parent to result")
		}

		// TODO: get repo name from github?

		currIssue := issues.Issue{
			Org:                 storiesOrg,
			Repo:                storiesRepo,
			Number:              depis.Blocked.IssueNumber,
			GithubRepoID:        &storiesId,
			ProgressState:       data.Pipeline.Name,
			MappedProgressState: s.Config.StoryColumnMapping.ReverseLookup(data.Pipeline.Name),
			IsClosed:            data.Pipeline.Name == s.Config.StoryColumnMapping[issues.ColumnClosed],
		}

		if currIssue.Number != 0 {
			parentIssues = append(parentIssues, currIssue)
		}
	}

	//dumpJSON("parentIssues 1", parentIssues)

	return parentIssues, nil

}

func (s IssueService) GetChildren(issue issues.IssueRef) ([]issues.Issue, error) {

	var childIssues []issues.Issue
	org, repo, number, err := issue.Parts()
	if err != nil {
		return nil, err
	}

	repoId, err := s.GetRepoIdbyName(org, repo)
	if err != nil {
		return nil, err
	}

	deps, err := s.zenhub.GetDependencies(repoId, number)
	if err != nil {
		return nil, err
	}

	if len(deps) < 1 {
		return nil, nil
	}

	for _, dep := range deps {
		if dep.Blocked.RepoID != repoId || dep.Blocked.IssueNumber != number {
			continue
		}

		//dumpJSON("dep", dep)

		data, err := s.zenhub.GetIssueData(dep.Blocking.RepoID, dep.Blocking.IssueNumber)
		if err != nil {
			return nil, errors.Wrap(err, "add child to result")
		}
		//dumpJSON("issue data", data)

		// TODO: get repo name from github?

		currIssue := issues.Issue{
			Org:                 org,
			Repo:                repo,
			Number:              dep.Blocking.IssueNumber,
			GithubRepoID:        &dep.Blocking.RepoID,
			ProgressState:       data.Pipeline.Name,
			MappedProgressState: s.Config.StoryColumnMapping.ReverseLookup(data.Pipeline.Name),
			IsClosed:            data.Pipeline.Name == "",
			//s.Config.StoryColumnMapping[issues.ColumnClosed],
		}

		//dumpJSON("child issue pipeline name", data.Pipeline.Name)

		childIssues = append(childIssues, currIssue)
	}
	//dumpJSON("childIssues", childIssues)

	return childIssues, nil
}

func (s IssueService) GetClosedIssue(org, repoName string) ([]int, error) {

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

func (s IssueService) ChildrenAllClosed(children []issues.Issue) (bool, error) {

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
