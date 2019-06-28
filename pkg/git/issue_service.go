package git

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	//"github.com/naveego/bosun/pkg/git"

	//"github.com/coreos/etcd/client"
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
	git    GitWrapper
	log    *logrus.Entry
}


func NewIssueService(config Config, gitWrapper GitWrapper, log *logrus.Entry) (issues.IssueService, error) {

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

	return s, nil
}

func (s IssueService) ctx() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	return ctx
}

func (s IssueService) SetProgress(issue issues.IssueRef, column string) error {
	return nil
}

func (s IssueService) GetIssuesFromCommitsSince(org, repo, since string) ([]issues.Issue, error) {

	currentHash := s.git.Commit()
	changesBlob, err := s.git.Exec("log", "--left-right", "--cherry-pick", "--no-merges", "--no-color", fmt.Sprintf("%s...%s", since, currentHash))
	if err != nil {
		return nil, errors.Wrap(err, "get changes between commits")
	}
	/*changes := strings.Split(changesBlob, "Closes")
	if err != nil {
		return nil, err
	}*/
	reg := regexp.MustCompile(`#[0-9]+`)
	closedIssueStrings := reg.FindAllString(changesBlob,-1)
	regNum := regexp.MustCompile(`[0-9]+`)
	var closedIssueNums []int
	for _, closedIssueString := range closedIssueStrings {
		numString := regNum.FindAllString(closedIssueString, 1)
		nString := numString[0]
		num, err := strconv.Atoi(nString)
		if err != nil {
			return nil, errors.Wrap(err, "convert issue num to int")
		}
		closedIssueNums = append(closedIssueNums, num)
	}

	var closedIssues []issues.Issue
	for _, closedIssueNum := range closedIssueNums {
		curIssue := issues.Issue{
			Org: org,
			Repo: repo,
			Number: closedIssueNum,
		}
		closedIssues = append(closedIssues, curIssue)
	}
	return closedIssues, nil
}



func (s IssueService) Create(issue issues.Issue, parent *issues.IssueRef) (int,error) {

	log := s.log.WithField("title", issue.Title)

	var parentOrg, parentRepo string
	var parentIssueNumber int

	user, _, err := s.github.Users.Get(s.ctx(), "")
	if err != nil {
		return -1, err
	}

	issueRequest := &github.IssueRequest{
		Title:    github.String(issue.Title),
		Assignee: user.Login,
	}
	dumpJSON("assignee", issueRequest.Assignee)

	//var parentIssue *issues.Issue
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
			return -1, errors.Wrap(err, "get issue")
		}

		//issue.Org = parentOrg
		//issue.Repo = parentRepo

		if parentIssue.Milestone != nil {
			milestones, _, err := s.github.Issues.ListMilestones(s.ctx(), issue.Org, issue.Repo, nil)
			//dumpJSON("milestones", milestones)

			if err != nil {
				return -1, errors.Wrap(err, "get milestones for new issue")
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
		return -1, errors.Wrap(err, "creating issue")
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
		return -1, err
	}

	// Maybe figure out a better way than just commenting it out
	//s.log.Infof("Creating branch %q.", branchName)
	_, err = s.git.Exec("checkout", "-B", branchName)
	if err != nil {
		return -1, errors.Wrap(err, "create branch")
	}

	s.log.WithField("branch", branchName).Info("Pushing branch.")
	err = pkg.NewCommand("git", "push", "-u", "origin", branchName).RunE()
	if err != nil {
		return -1, errors.Wrap(err, "push branch")
	}

	// update parent issue body
	if parent != nil {
		parentIssue, _, err := s.github.Issues.Get(s.ctx(), parentOrg, parentRepo, parentIssueNumber)
		if err != nil {
			return -1, errors.Wrap(err, "get issue")
		}
		issueString := issue.Org + "/" + issue.Repo + "/#" + strconv.Itoa(issueNumber)
		parentNewChild := fmt.Sprintf("\nrequires %s", issueString)
		parentNewBody := *parentIssue.Body
		parentNewBody += parentNewChild
		newParentRequest := &github.IssueRequest{
			Title: parentIssue.Title,
			Body: &parentNewBody,
		}

		editedParent, response, err := s.github.Issues.Edit(context.Background(), parentOrg, parentRepo, parentIssueNumber, newParentRequest)
		if err != nil {
			return -1, errors.Wrap(err, "edit parent story")
		}
		if response.StatusCode != http.StatusOK {
			return -1, fmt.Errorf("the edit issue endpoint returned %v", response.StatusCode)
		}
		s.log.WithField("new body", editedParent.Body)
	}

	return issueNumber, nil

}

// Helper function to split IssueRef
func Split(r rune) bool {
	return r == '#' || r == '/'
}

func (s IssueService) AddDependency(from, to issues.IssueRef, parentIssueNum int) error {

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

func (s IssueService) GetParents(issue issues.IssueRef) ([]issues.Issue, error) {

	var parentIssues []issues.Issue
	org, repo, number, err := issue.Parts()
	//repoId, err := s.GetRepoIdbyName(org, repo)
	if err != nil {
		return nil, err
	}

	parentOrg := "naveegoinc"
	parentRepo := "stories"

	githubIssue, _, err := s.github.Issues.Get(s.ctx(), org, repo, number)
	body := *githubIssue.Body
	regForParent := regexp.MustCompile(`required by [a-z]+\/[a-z]+\/#[0-9]+`)
	regForParentNum := regexp.MustCompile(`[0-9]+`)
	parents := regForParent.FindAllString(body,-1)
	if len(parents) < 1 {
		return parentIssues, nil
	}

	for _, parent := range parents {
		var parentIssue issues.Issue
		parentNum := regForParentNum.FindAllString(parent, -1)
		parentNumInt, err := strconv.Atoi(parentNum[0])
		if err != nil {
			return parentIssues, errors.Wrap(err, "get parent issue")
		}
		parentIssue = issues.Issue{
			Number: parentNumInt,
			Org: parentOrg,
			Repo: parentRepo,
		}
		parentIssues = append(parentIssues, parentIssue)
	}

	return parentIssues, nil

}

func (s IssueService) GetChildren(issue issues.IssueRef) ([]issues.Issue, error) {

	var childIssues []issues.Issue
	org, repo, number, err := issue.Parts()
	if err != nil {
		return nil, errors.Wrap(err, "parts of issue")
	}

	// find children from an issue's body
	githubIssuePointer, _,  err := s.github.Issues.Get(s.ctx(), org, repo, number)
	githubIssue := *githubIssuePointer
	issueBody := *githubIssue.Body
	reg := regexp.MustCompile(`requires [a-z]+\/[a-z]+\/#[0-9]+`)
	childrenMatch := reg.FindAllString(issueBody, -1)
	if len(childrenMatch) < 1 {
		return nil, nil
	}
	regOR := regexp.MustCompile(`[a-z]+\/[a-z]+`)

	i := 0

	for i < len(childrenMatch) {
		childMatch := childrenMatch[i]

		orMatch := regOR.FindAllString(childMatch, -1)
		orMatchString := orMatch[0]
		OR := strings.Split(orMatchString, "/")
		childOrg := OR[0]
		childRepo := OR[1]

		regNum := regexp.MustCompile(`[0-9]+`)
		childNumMatch := regNum.FindAllString(childMatch, -1)
		childNumString := childNumMatch[0]
		childNum, err := strconv.Atoi(childNumString)
		if err != nil {
			return nil, errors.Wrap(err, "child number")
		}

		child := issues.Issue {
			Org: childOrg,
			Repo: childRepo,
			Number: childNum,
		}
		childIssues = append(childIssues, child)
	}

	/*timeline, _, err := s.github.Issues.ListIssueTimeline(s.ctx(), org, repo, number, nil)
	if err != nil {
		return nil, errors.Wrap(err, "list issue timeline")
	}
	for _, tl := range timeline {
		event := *tl.Event
		if event != "cross-referenced" {
			continue
		}
		url := tl.Source.URL

		regUrl := regexp.MustCompile(`repos/[a-z]+/[a-z]+/issues/[0-9]+`)
		urlMatch := regUrl.FindAllString(url, -1)
		urlMatchString := urlMatch[0]
		urlSplit := strings.Split(urlMatchString, "/")
		childOrg := urlSplit[1]
		childRepo := urlSplit[2]
		childNumber, err := strconv.Atoi(urlSplit[4])
		if err != nil {
			return childIssues, errors.Wrap(err, "get child issue information from url")
		}
		childIssuePointer,_, err := s.github.Issues.Get(s.ctx(), childOrg, childRepo, childNumber)
		if err != nil {
			return nil, errors.Wrap(err, "get child issue pointer")
		}
		childIssueContent := *childIssuePointer
		childIssue := issues.Issue{
			Number: childNumber,
			Org: childOrg,
			Repo: childRepo,
			Body: *childIssueContent.Body,
			Assignee: *childIssueContent.Assignee.Login,
			Milestone: childIssueContent.Milestone.Number,
			Title: *childIssueContent.Title,
		}
		if &childIssueContent.ClosedAt == nil {
			childIssue.IsClosed = false
		} else {
			childIssue.IsClosed = true
		}
		childIssues = append(childIssues, childIssue)
	} */


		//dumpJSON("childIssues", childIssues)

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


//type Issue issues.Issue
type IssueRef issues.IssueRef

func dumpJSON(label string, data interface{}) {
	j, _ := json.MarshalIndent(data, "", "  ")
	fmt.Fprintf(os.Stderr, "%s:\n%s\n\n", label, string(j))
}
