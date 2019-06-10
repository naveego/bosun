package git

import (
	"context"
	"fmt"
	"github.com/google/go-github/v20/github"
	"github.com/pkg/errors"
	"os"
	"strings"
)

func CreateDeploy(client *github.Client, orgSlashRepo, ref, environment string) (int64, error) {

	org, repo, err := parseOrgSlashRepo(orgSlashRepo)

	deploymentRequest := &github.DeploymentRequest{
		Description:      github.String(fmt.Sprintf("Deployment to %s", environment)),
		Environment:      &environment,
		Ref:              &ref,
		Task:             github.String("deploy"),
		AutoMerge:        github.Bool(false),
		RequiredContexts: &[]string{},
	}

	deployment, _, err := client.Repositories.CreateDeployment(context.Background(), org, repo, deploymentRequest)
	if err != nil {
		return 0, err
	}

	id := *deployment.ID

	return id, nil
}

func UpdateDeploy(client *github.Client, orgSlashRepo string, deployID int64, state string) error {

	req := &github.DeploymentStatusRequest{
		State: &state,
	}

	buildID, ok := os.LookupEnv("TEAMCITY_BUILD_ID")
	if ok {
		req.LogURL = github.String(fmt.Sprintf("https://ci.n5o.black/viewLog.html?buildId=%s", buildID))
	}

	org, repo, err := parseOrgSlashRepo(orgSlashRepo)

	_, _, err = client.Repositories.CreateDeploymentStatus(context.Background(), org, repo, deployID, req)

	return err
}

func parseOrgSlashRepo(orgSlashRepo string) (org string, repo string, err error) {
	segs := strings.Split(orgSlashRepo, "/")
	if len(segs) != 2 {
		return "", "", errors.Errorf("orgSlashRepo must be org/repo, not like %q", orgSlashRepo)
	}
	org, repo = segs[0], segs[1]

	return
}
