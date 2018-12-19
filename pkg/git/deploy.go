package git

import (
	"context"
	"fmt"
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg"
	"os"
)

func CreateDeploy(repoPath, environment string)(int64, error) {
	var err error
	repoPath, err = GetRepoPath(repoPath)
	if err != nil {
		return 0, err
	}
	sha := pkg.NewCommand("git", "-C", repoPath, "rev-parse", "HEAD").MustOut()



	deploymentRequest := &github.DeploymentRequest{
		Description: github.String(fmt.Sprintf("Deployment to %s", environment)),
		Environment: &environment,
		Ref:         &sha,
		Task:github.String("deploy"),
		AutoMerge:github.Bool(false),
	}



	org, repo := GetOrgAndRepoFromPath(repoPath)
	client := mustGetGitClient()

	deployment, _, err := client.Repositories.CreateDeployment(context.Background(), org, repo, deploymentRequest)
	if err != nil {
		return 0, err
	}

	id := *deployment.ID

	return id, nil
}

func UpdateDeploy(repoPath string, deployID int64, state string) error {

	var err error
	repoPath, err = GetRepoPath(repoPath)
	if err != nil {
		return err
	}

	req := &github.DeploymentStatusRequest{
		State:&state,
	}

	buildID, ok := os.LookupEnv("TEAMCITY_BUILD_ID")
	if ok {
		req.LogURL = github.String(fmt.Sprintf("https://ci.n5o.black/viewLog.html?buildId=%s", buildID))
	}

	org, repo := GetOrgAndRepoFromPath(repoPath)
	client := mustGetGitClient()

	_, _, err = client.Repositories.CreateDeploymentStatus(context.Background(), org, repo, deployID, req)

	return err
}
