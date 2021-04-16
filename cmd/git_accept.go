package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/git"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"strconv"
)

var gitAcceptPullRequestCmd = addCommand(gitCmd, &cobra.Command{
	Use:           "accept-pull-request [number] [major|minor|patch|major.minor.patch]",
	Aliases:       []string{"accept-pr", "accept"},
	Args:          cobra.RangeArgs(1, 2),
	SilenceUsage:  true,
	SilenceErrors: true,
	Short:         "Accepts a pull request and merges it into master, optionally bumping the version and tagging the master branch.",
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())
		var err error
		wd, _ := os.Getwd()
		repoPath, err := git.GetRepoPath(wd)
		if err != nil {
			return err
		}

		prNumber, err := strconv.Atoi(args[0])
		if err != nil {
			return err
		}

		client := mustGetGithubClient()

		b := MustGetBosun()
		svc, err := b.GetIssueService()
		if err != nil {
			b.NewContext().Log().Warnf("Could not get issue service, issues will not be updated: %s", err)
		}

		acceptPRCommand := git.GitAcceptPRCommand{
			RepoDirectory: repoPath,
			PRNumber:      prNumber,
			Client:        client,
			IssueService:  svc,
		}

		err = acceptPRCommand.Execute()
		if err != nil {
			return err
		}

		g, err := git.NewGitWrapper(repoPath)
		if err != nil {
			return err
		}
		_ = g.Fetch()

		if g.Branch() == "develop" {
			err = g.Pull()
			return err
		} else {
			core.Log.Infof("You should probably pull the develop branch now.")
		}

		return nil
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().StringSlice(ArgGitAcceptPRAppVersion, []string{}, "DeployedApps to apply version bump to.")
})

const ArgGitAcceptPRAppVersion = "app"

func getOrgAndRepo() (string, string) {
	return git.GetCurrentOrgAndRepo().OrgAndRepo()
}

func dumpJSON(label string, data interface{}) {
	if viper.GetBool(ArgGlobalVerbose) {
		j, _ := json.MarshalIndent(data, "", "  ")
		fmt.Fprintf(os.Stderr, "%s:\n%s\n\n", label, string(j))
	}
}
