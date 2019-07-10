package cmd

import (
	"errors"
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/naveego/bosun/pkg/git"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io/ioutil"
	"os"
	"strings"
)

var gitCommitCmd = addCommand(gitCmd, &cobra.Command{
	Use:     "commit",
	Aliases: []string{"cz"},
	Short:   "Commits with a formatted message",
	RunE: func(cmd *cobra.Command, args []string) error {

		var err error
		var out string
		err = viper.BindPFlags(cmd.Flags())
		if err != nil {
			return err
		}

		repoPath, err := git.GetCurrentRepoPath()
		if err != nil {
			return err
		}

		g, err := git.NewGitWrapper(repoPath)
		if err != nil {
			return err
		}

		out, err = g.Exec("diff", "--name-only", "--cached")
		if err != nil {
			return err
		}
		if out == "" {
			return errors.New("No files added to staging! Did you forget to run git add?")
		}

		retryFlag := viper.GetBool(GitRetry)

		var tmpFileExists bool
		tmpFileExists, err = exists(TempFileGitCommit)
		if err != nil {
			return err
		}

		var msg string
		if retryFlag && tmpFileExists {

			var bytes []byte
			bytes, err = ioutil.ReadFile(TempFileGitCommit)
			msg = string(bytes)
			if err != nil {
				return err
			}
		} else {

			typeAns := ""
			scopeAns := ""
			shortAns := ""
			longAns := ""
			affectedIssueAns := ""
			breakingChangesDescriptionAns := ""
			breakingChangesAns := false
			branch := strings.Split(g.Branch(), "/")

			typeQues := &survey.Select{
				Message: "Select the type of change that you're committing:",
				Options: []string{
					"feat:     A new feature",
					"fix:      A bug fix",
					"docs:     Documentation only changes",
					"style:    Changes that do not affect the meaning of the code",
					"refactor: A code change that neither fixes a bug nor adds a feature",
					"perf:     A code change that improves performance",
					"test:     Adding missing tests or correcting existing tests",
					"build:    Changes that affect the build system or external dependencies",
					"ci:       Changes to our CI configuration files and scripts",
					"chore:    Other changes that don't modify src or test files",
					"revert:   Reverts a previous commit",
				},
			}

			scopeQues := &survey.Input{
				Message: "What is the scope of this change (e.g. component or file name)? (press enter to skip)",
			}

			shortQues := &survey.Input{
				Message: "Write a short, imperative tense description of the change:",
			}

			longQues := &survey.Input{
				Message: "Provide a longer description of the change: (press enter to skip)",
			}

			breakingChangesQues := &survey.Confirm{
				Message: "Are there any breaking changes?",
			}

			breakingChangesDescriptionQues := &survey.Input{
				Message: "Describe the breaking changes:",
			}

			survey.AskOne(typeQues, &typeAns)
			survey.AskOne(scopeQues, &scopeAns)
			survey.AskOne(shortQues, &shortAns)
			survey.AskOne(longQues, &longAns)
			survey.AskOne(breakingChangesQues, &breakingChangesAns)

			if breakingChangesAns {
				survey.AskOne(breakingChangesDescriptionQues, &breakingChangesDescriptionAns)
				breakingChangesDescriptionAns = "BREAKING CHANGE: " + breakingChangesDescriptionAns
			}

			if len(branch) == 3 && branch[0] == "issue" {
				affectedIssueAns = branch[1]
			}

			title := strings.Split(typeAns, ":")[0] + "(" + scopeAns + "): " + shortAns
			descriptionArray := []string{longAns, breakingChangesDescriptionAns, affectedIssueAns}
			description := strings.Join(descriptionArray, "\n\n")

			msg = fmt.Sprintf("%s\n\n%s", title, description)

		}

		_, err = g.Exec("commit", "-m", msg)
		if tmpFileExists {
			if os.Remove(TempFileGitCommit) != nil {
				return err
			}
		}

		if err != nil {
			tmpFile, err := ioutil.TempFile(os.TempDir(), "")
			if err != nil {
				return err
			}
			if _, err := tmpFile.Write([]byte(msg)); err != nil {
				return err
			}
			err = os.Rename(tmpFile.Name(), TempFileGitCommit)
			if err != nil {
				return err
			}
			err = tmpFile.Close()
			if err != nil {
				return err
			}
		}

		return err

	},
}, func(cmd *cobra.Command) {
	cmd.Flags().BoolP(GitRetry, "r", false, "commits with the previously failed commit message")
})

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

const (
	GitRetry          = "retry"
	TempFileGitCommit = "/tmp/bosun_git_commit"
)
