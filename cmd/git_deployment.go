package cmd

import (
	"context"
	"fmt"
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"strconv"
)

var gitDeploymentCmd = addCommand(gitCmd, &cobra.Command{
	Use:   "deployment",
	Short: "Deploy-related commands.",
})

var gitDeployDryRunCmd = addCommand(gitDeploymentCmd, &cobra.Command{
	Use:   "dry-run [app]",
	Short: "Notifies github that a deploy has happened.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun()
		app := mustGetApp(b, args)
		repoRef, err := app.Repo.GetRef()
		if err != nil {
			return err
		}
		deployer, err := b.GetDeployer(repoRef)
		if err != nil {
			return err
		}

		previousDeployment, err := deployer.GetMostRecentSuccessfulDeployment()
		if err != nil {
			return err
		}

		ctx := b.NewContext()
		log := ctx.Log

		log.Infof("Found recent deployment:\n%s", util.MustYaml(previousDeployment))

		previousRef := previousDeployment.GetRef()
		g, err := app.Repo.LocalRepo.Git()
		if err != nil {
			return err
		}
		head := g.Commit()
		changeLog, err := g.ChangeLog(previousRef, head, nil, git.GitChangeLogOptions{})
		if err != nil {
			return err
		}

		changes := changeLog.Changes

		log.Infof("Found %d issues since previous deployment.", len(changes))
		log.Debug("Finding stories for issues...")

		issueSvc, err := b.GetIssueService()
		if err != nil {
			return err
		}
		stories, err := changes.MapToStories(issueSvc)
		if err != nil {
			return err
		}
		log.Infof("Found %d stories impacted by issues since last deployment.", len(stories))

		for _, story := range stories {
			fmt.Printf("%s - %s\n", story.StoryRef, story.StoryTitle)
		}

		return nil

	},
})

var gitDeployStartCmd = addCommand(gitDeploymentCmd, &cobra.Command{
	Use:   "start {cluster}",
	Args:  cobra.ExactArgs(1),
	Short: "Notifies github that a deploy has happened.",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := mustGetGithubClient()

		cluster := args[0]
		sha := pkg.NewCommand("git rev-parse HEAD").MustOut()
		isProd := cluster == "blue"

		deploymentRequest := &github.DeploymentRequest{
			Description:           github.String(fmt.Sprintf("Deployment to %s", cluster)),
			Environment:           &cluster,
			Ref:                   &sha,
			ProductionEnvironment: &isProd,
			Task:                  github.String("deploy"),
		}

		org, repo := getOrgAndRepo()

		deployment, _, err := client.Repositories.CreateDeployment(context.Background(), org, repo, deploymentRequest)
		if err != nil {
			return err
		}

		id := *deployment.ID
		fmt.Println(id)
		return nil
	},
})

var gitDeployUpdateCmd = addCommand(gitDeploymentCmd, &cobra.Command{
	Use:   "update {deployment-id} {success|failure}",
	Args:  cobra.ExactArgs(2),
	Short: "Notifies github that a deploy has happened.",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := mustGetGithubClient()

		org, repo := getOrgAndRepo()

		deploymentID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return errors.Wrap(err, "invalid deployment ID")
		}

		req := &github.DeploymentStatusRequest{
			State: &args[1],
		}

		_, _, err = client.Repositories.CreateDeploymentStatus(context.Background(), org, repo, deploymentID, req)
		if err != nil {
			return err
		}
		return nil
	},
})
