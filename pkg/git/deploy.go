package git

import (
	"context"
	"fmt"
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/pkg/errors"
	"os"
	"time"
)

type Deployer struct {
	github       *github.Client
	issueService issues.IssueService
	dir          string
	org          string
	repo         string
}

func NewDeployer(repoRef issues.RepoRef, client *github.Client, issueService issues.IssueService) (*Deployer, error) {

	return &Deployer{
		github:       client,
		issueService: issueService,
		org:          repoRef.Org,
		repo:         repoRef.Repo,
	}, nil
}

func (d Deployer) CreateDeploy(ref, environment string) (int64, error) {

	deploymentRequest := &github.DeploymentRequest{
		Description:      github.String(fmt.Sprintf("Deployment to %s", environment)),
		Environment:      &environment,
		Ref:              &ref,
		Task:             github.String("deploy"),
		AutoMerge:        github.Bool(false),
		RequiredContexts: &[]string{},
	}

	deployment, _, err := d.github.Repositories.CreateDeployment(context.Background(), d.org, d.repo, deploymentRequest)
	if err != nil {
		return 0, err
	}

	id := *deployment.ID

	return id, nil
}

func (d Deployer) UpdateDeploy(deployID int64, state string, message string) error {

	req := &github.DeploymentStatusRequest{
		State:       &state,
		Description: &message,
	}

	buildID, ok := os.LookupEnv("TEAMCITY_BUILD_ID")
	if ok {
		req.LogURL = github.String(fmt.Sprintf("https://ci.n5o.black/viewLog.html?buildId=%s", buildID))
	}

	_, _, err := d.github.Repositories.CreateDeploymentStatus(context.Background(), d.org, d.repo, deployID, req)

	return err
}

func (d Deployer) GetMostRecentSuccessfulDeployment() (*github.Deployment, error) {

	recentDeployments, _, err := d.github.Repositories.ListDeployments(timeoutContext(5*time.Second), d.org, d.repo, nil)
	if err != nil {
		return nil, err
	}
	for _, deployment := range recentDeployments {
		statuses, _, err := d.github.Repositories.ListDeploymentStatuses(timeoutContext(5*time.Second), d.org, d.repo, deployment.GetID(), nil)
		if err != nil {
			return nil, err
		}
		for _, status := range statuses {
			if status.GetState() == "success" {
				return deployment, nil
			}
		}
	}
	return nil, errors.Errorf("could not find a recent successful deployment (checked %d deployments)", len(recentDeployments))
}

func stdctx() context.Context {
	return timeoutContext(5 * time.Second)
}

func timeoutContext(timeout time.Duration) context.Context {
	ctx, _ := context.WithTimeout(context.Background(), timeout)
	return ctx
}
