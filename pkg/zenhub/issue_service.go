package zenhub

import (
	"context"
	"encoding/json"
	"fmt"
	//"github.com/coreos/etcd/client"
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/git"
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
	zenhub *API
	git    git.GitWrapper
	log    *logrus.Entry
}

func NewIssueService(config Config, gitWrapper git.GitWrapper, log *logrus.Entry) (IssueService, error) {

	s := IssueService{
		Config: config,
		git:    gitWrapper,
		log:    log,
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: config.GithubToken},
	)
	tc := oauth2.NewClient(context.Background(), ts)

	s.github = github.NewClient(tc)

	s.zenhub = New(config.ZenhubToken, s.github)

	return s, nil
}

func (s IssueService) ctx() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	return ctx
}



func (s IssueService) Create(issue issues.Issue, parent *issues.IssueRef) error {

	log := s.log.WithField("title", issue.Title)

	var parentOrg, parentRepo string
	var parentIssueNumber int

	user, _, err := s.github.Users.Get(s.ctx(), "")
	if err != nil {
		return err
	}

	issueRequest := &github.IssueRequest{
		Title:    github.String(issue.Title),
		Assignee: user.Login,
	}
	dumpJSON("assignee", issueRequest.Assignee)

	if parent != nil {

		issue.Body = fmt.Sprintf("%s\n\n required by %s", issue.Body, parent.String())

		// Need to figure out where to get the title
		//issue.Title = "tempTitle"

		dumpJSON("issue", issue)

		pOrg, pRepo, pIssueNumber, _ := parent.Parts()
		parentOrg = pOrg
		parentRepo = pRepo
		parentIssueNumber = pIssueNumber

		parentIssue, _, err := s.github.Issues.Get(s.ctx(), parentOrg, parentRepo, parentIssueNumber)
		if err != nil {
			return errors.Wrap(err, "get issue")
		}

		//issue.Org = parentOrg
		//issue.Repo = parentRepo

		if parentIssue.Milestone != nil {
			milestones, _, err := s.github.Issues.ListMilestones(s.ctx(), issue.Org, issue.Repo, nil)
			//dumpJSON("milestones", milestones)

			if err != nil {
				return errors.Wrap(err, "get milestones for new issue")
			}
			for _, m := range milestones {
				if m.GetTitle() == parentIssue.Milestone.GetTitle() {
					log.WithField("milestone", m.GetTitle()).Info("Attaching milestone.")
					issueRequest.Milestone = m.Number
					break
				}
			}
		}
	}

	/*if &issue.Assignee != nil {
		issueRequest.Assignee = &issue.Assignee
		s.log.WithField("title", issue.Title).Info("Setting assignee")
	} */
	s.log.WithField("title", issue.Title).Info("Setting assignee")

	issueRequest.Body = &issue.Body

	dumpJSON("creating issue", issueRequest)
	issueResponse, _, err := s.github.Issues.Create(s.ctx(), issue.Org, issue.Repo, issueRequest)
	if err != nil {
		return errors.Wrap(err, "creating issue")
	}

	issueResponse.Repository.GetID()

	issueNumber := issueResponse.GetNumber()
	log = log.WithField("issue", issueNumber)

	log.Info("Created issue.")

	slug := regexp.MustCompile(`\W+`).ReplaceAllString(strings.ToLower(issue.Title), "-")
	branchName := fmt.Sprintf("issue/#%d/%s", issueNumber, slug)
	s.log.WithField("branch", branchName).Info("Creating branch.")
	err = pkg.NewCommand("git", "checkout", "-b", branchName).RunE()
	if err != nil {
		return err
	}

	// Maybe figure out a better way than just commenting it out
	//s.log.Infof("Creating branch %q.", branchName)
	_, err = s.git.Exec("checkout", "-B", branchName)
	if err != nil {
		return errors.Wrap(err, "create branch")
	}

	s.log.WithField("branch", branchName).Info("Pushing branch.")
	err = pkg.NewCommand("git", "push", "-u", "origin", branchName).RunE()
	if err != nil {
		return errors.Wrap(err, "push branch")
	}

	//issueResponse, _, err := s.github.Issues.Create(s.ctx(), issue.Org, issue.Repo, issueRequest)

	// the below issueNumber used to be issue.Number
	newIssueRef := issues.NewIssueRef(issue.Org, issue.Repo, issueNumber)
	if parent != nil {
		err = s.AddDependency(newIssueRef, *parent, parentIssueNumber)
		if err != nil {
			return errors.Wrap(err, "add dependency;"+newIssueRef.String()+", parent "+(parent).String())
		}
	}

	// Move the task and issue to In Progress column
	column := issues.ColumnInDevelopment
	err = s.SetProgress(newIssueRef, column)
	if err != nil {
		return errors.Wrap(err, "set task progress")
	}
	if parent != nil {
		err = s.SetProgress(*parent, column)
		if err != nil {
			return errors.Wrap(err, "set parent progress")
		}
	}

	return nil

}

// Helper function to split IssueRef
func Split(r rune) bool {
	return r == '#' || r == '/'
}

func (s IssueService) AddDependency(from, to issues.IssueRef, parentIssueNum int) error {

	/*fromString := from.String() // convert IssueRef to string
	toString := to.String()
	fromSplitted := strings.FieldsFunc(fromString, Split)
	toSplitted := strings.FieldsFunc(toString, Split)*/

	orgFrom, repoFrom, numFrom, err := from.Parts()
	if err != nil {
		return errors.Wrap(err, "split 'from' IssueRef")
	}

	/*blockingRepo := fromSplitted[1]
	blockingNum, err := strconv.Atoi(fromSplitted[2])
	if err != nil {
		return errors.Wrap(err, strings.Join(fromSplitted, " "))
	}*/

	orgTo, repoTo, _, err := to.Parts()
	if err != nil {
		return errors.Wrap(err, "split 'to' IssueRef")
	}
	//blockedRepo := toSplitted[1]
	/*blockedNum, err := strconv.Atoi(toSplitted[2])
	if err != nil {
		return err
	} */

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

func (s IssueService) SetProgress(issue issues.IssueRef, column string) error {

	issueString := issue.String()
	issueSplitted := strings.FieldsFunc(issueString, Split)
	org := issueSplitted[0]
	repoName := issueSplitted[1]

	repoId, err := s.GetRepoIdbyName(org, repoName)
	if err != nil {
		return errors.Wrap(err, "set progress - get repo id")
	}

	pipelineID, err := s.zenhub.GetPipelineID(repoId, column)
	if err != nil {
		return errors.Wrap(err, "set progress - get pipeline id")
	}
	issueNum, err := strconv.Atoi(issueSplitted[2])
	if err != nil {
		return errors.Wrap(err, "set progress - issue number to int")
	}

	err = s.zenhub.MovePipeline(repoId, issueNum, pipelineID)
	if err != nil {
		return errors.Wrap(err, "set progress - move issue between pipelines")
	}

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
	issuePointer, _, err = s.github.Issues.Get(s.ctx(), org, repo, num)
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

	//dumpJSON("depsInStories", depsInStories)

	if len(deps) < 1 && len(depsInStories) < 1{ // no parent found
		return nil, nil
	}

	for _, dep := range deps{
		if dep.Blocking.RepoID != repoId || dep.Blocking.IssueNumber != number {
			continue
		}

		data, err := s.zenhub.GetIssueData(dep.Blocking.RepoID, dep.Blocking.IssueNumber)
		if err != nil {
			return nil, errors.Wrap(err, "add parent to result")
		}

		// TODO: get repo name from github?

		currIssue := issues.Issue{
			Org:org,
			Repo:repo,
			Number: dep.Blocked.IssueNumber,
			GithubRepoID: &dep.Blocked.RepoID,
			ProgressState: data.Pipeline.Name,
			MappedProgressState: s.Config.StoryColumnMapping.ReverseLookup(data.Pipeline.Name),
			IsClosed: data.Pipeline.Name == s.Config.StoryColumnMapping[issues.ColumnClosed],
		}

		parentIssues = append(parentIssues, currIssue)
	}

	for _, depis := range depsInStories{
		if depis.Blocking.RepoID != repoId || depis.Blocking.IssueNumber != number {
			continue
		}

		data, err := s.zenhub.GetIssueData(depis.Blocking.RepoID, depis.Blocking.IssueNumber)
		if err != nil {
			return nil, errors.Wrap(err, "add parent to result")
		}

		// TODO: get repo name from github?

		currIssue := issues.Issue{
			Org:org,
			Repo:repo,
			Number: depis.Blocked.IssueNumber,
			GithubRepoID: &depis.Blocked.RepoID,
			ProgressState: data.Pipeline.Name,
			MappedProgressState: s.Config.StoryColumnMapping.ReverseLookup(data.Pipeline.Name),
			IsClosed: data.Pipeline.Name == s.Config.StoryColumnMapping[issues.ColumnClosed],
		}

		parentIssues = append(parentIssues, currIssue)
	}

	dumpJSON("parentIssues", parentIssues)

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

		for _, dep := range deps{
			if dep.Blocked.RepoID != repoId || dep.Blocked.IssueNumber != number {
				continue
			}

			data, err := s.zenhub.GetIssueData(dep.Blocking.RepoID, dep.Blocking.IssueNumber)
			if err != nil {
				return nil, errors.Wrap(err, "add child to result")
			}

			// TODO: get repo name from github?

			currIssue := issues.Issue{
				Org:org,
				Repo:repo,
				Number: dep.Blocking.IssueNumber,
				GithubRepoID: &dep.Blocking.RepoID,
				ProgressState: data.Pipeline.Name,
				MappedProgressState: s.Config.StoryColumnMapping.ReverseLookup(data.Pipeline.Name),
				IsClosed: data.Pipeline.Name == s.Config.StoryColumnMapping[issues.ColumnClosed],
			}

			childIssues = append(childIssues, currIssue)
	}

	return childIssues, nil
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
	for i<len(closedIssues){
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
