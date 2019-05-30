package zenhub

import (
	"context"
	"encoding/json"
	"fmt"
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
	github *github.Client
	zenhub *API
	git git.GitWrapper
	log *logrus.Entry
}

func NewIssueService(githubToken, zenhubToken string, gitWrapper git.GitWrapper, log *logrus.Entry) (issues.IssueService, error) {
	s := IssueService{
		git:gitWrapper,
		log:log,
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(context.Background(), ts)

	s.github = github.NewClient(tc)

	s.zenhub = New(zenhubToken, s.github)

	return s, nil
}

func (s IssueService) ctx() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	return ctx
}

func (s IssueService) GetIssue(ref issues.IssueRef) (issues.Issue, error) {
	panic("implement me")

	/*var returnedIssue issues.Issue
	var issuePointer *github.Issue
	var returnedIssue1 github.Issue
	org, repo, num, err := s.SplitIssueRef(ref)
	if err != nil {
		return returnedIssue, errors.Wrap(err, "split IssueRef")
	}
	issuePointer, _, err =  s.github.Issues.Get(s.ctx(), org, repo, num)
	if err != nil {
		return returnedIssue, errors.Wrap(err, "split IssueRef")
	}
	returnedIssue1 = *issuePointer
	return returnedIssue, nil*/
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
		Title: github.String(issue.Title),
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

		issue.Org = parentOrg
		issue.Repo = parentRepo

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
			return errors.Wrap(err, "add dependency;" + newIssueRef.String() + ", parent " + (parent).String())
		}
	}

	// Move the task and issue to In Progress column
	column := "In Progress"
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

	fromString := from.String() // convert IssueRef to string
	toString := to.String()
	fromSplitted := strings.FieldsFunc(fromString, Split)
	toSplitted := strings.FieldsFunc(toString, Split)

	org := fromSplitted[0]

	blockingRepo := fromSplitted[1]
	blockingNum, err := strconv.Atoi(fromSplitted[2])
	if err != nil {
		return errors.Wrap(err, strings.Join(fromSplitted, " "))
	}

	blockedRepo := toSplitted[1]
	/*blockedNum, err := strconv.Atoi(toSplitted[2])
	if err != nil {
		return err
	} */

	blockingId, err := s.GetRepoIdbyName(org, blockingRepo)
	if err != nil {
		return err
	}
	blockedId, err := s.GetRepoIdbyName(org, blockedRepo)
	if err != nil {
		return err
	}

	blockingIssue := NewDependencyIssue(blockingId, blockingNum)
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

	//dumpJSON("repo from GetRepoIdbyName", repo)
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
		return err
	}
	issueNum, err := strconv.Atoi(issueSplitted[2])
	if err != nil {
		return err
	}

	err = s.zenhub.MovePipeline(repoId, issueNum, pipelineID)
	if err != nil {
		return err
	}
	//s.log.Warn("Setting progress not implemented yet.")
	return nil
}

func (s IssueService) SplitIssueRef(issue issues.IssueRef) (string, string, int, error) {

	issueString := issue.String()
	issueSplitted := strings.FieldsFunc(issueString, Split)
	org := issueSplitted[0]
	repoIdString := issueSplitted[1]
	/*repoId, err := s.GetRepoIdbyName(org, repoIdString)
	if err != nil {
		return "", "", 0,  err
	} */
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
	issuePointer, _, err =  s.github.Issues.Get(s.ctx(), org, repo, num)
	if err != nil {
		return state, errors.Wrap(err, "get a single issue with github api")
	}
	returnedIssue = *issuePointer
	state = *returnedIssue.State
	return state, nil
}

func (s IssueService) GetParents(issue issues.IssueRef) ([]issues.Issue, error) {

	var parentIssues []issues.Issue
	issueString := issue.String()
	issueSplitted := strings.FieldsFunc(issueString, Split)
	org := issueSplitted[0]
	repoIdString := issueSplitted[1]
	repoId, err := s.GetRepoIdbyName(org, repoIdString)
	if err != nil {
		return nil, err
	}
	issueNum, err := strconv.Atoi(issueSplitted[2])
	if err != nil {
		return nil, err
	}

	var parentIssueNums []int
	parentIssueNums, _, err = s.zenhub.GetDependencies(repoId, issueNum)
	if err != nil {
		return nil, errors.Wrap(err, "get dependencies")
	}

	i := 0
	if len(parentIssueNums) > 0 {
		for i < len(parentIssueNums) {
			// reconstruct issue with given org, repoId and issueNum
			var currIssue issues.Issue
			currIssue.Org = org
			currIssue.Repo = repoIdString
			currIssue.Number = parentIssueNums[i]
			parentIssues = append(parentIssues, currIssue)
			i++
		}
	}


	return parentIssues, nil

}

func (s IssueService) GetChildren(issue issues.IssueRef) ([]issues.Issue, error) {

	var childIssues []issues.Issue
	issueString := issue.String()
	issueSplitted := strings.FieldsFunc(issueString, Split)
	org := issueSplitted[0]
	repoIdString := issueSplitted[1]
	repoId, err := s.GetRepoIdbyName(org, repoIdString)
	if err != nil {
		return nil, err
	}
	issueNum, err := strconv.Atoi(issueSplitted[2])
	if err != nil {
		return nil, err
	}

	var childIssueNums []int
	_, childIssueNums, err = s.zenhub.GetDependencies(repoId, issueNum)
	if err != nil {
		return nil, err
	}

	if len(childIssueNums) < 1 {
		return nil, nil
	}

	i := 0
	if len(childIssueNums) > 0 {
		for i < len(childIssueNums) {
			// reconstruct issue with given org, repoId and issueNum
			var currIssue issues.Issue
			currIssue.Org = org
			currIssue.Repo = repoIdString
			currIssue.Number = childIssueNums[i]
			childIssues = append(childIssues, currIssue)
			i++
		}
	}


	return childIssues, nil
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
