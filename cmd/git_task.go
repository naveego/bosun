package cmd

import (
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"strings"

	// "os"
)

var gitTaskCmd = addCommand(gitCmd, &cobra.Command{
	Use:   "task {task name}",
	Args:  cobra.ExactArgs(1),
	Short: "Creates a task in the current repo, and a branch for that task. Optionally attaches task to a story, if flags are set.",
	Long:  `Requires github hub tool to be installed (https://hub.github.com/).`,
	RunE: func(cmd *cobra.Command, args []string) error {

		var err error

		err = viper.BindPFlags(cmd.Flags())
		if err != nil {
			return err
		}

		taskName := args[0]
		org, repo := git.GetCurrentOrgAndRepo()
		title := viper.GetString(ArgGitTitle)
		if title == "" {
			if len(taskName) > 50 {
				title = taskName[:50] + "..."
			} else {
				title = taskName
			}
		}
		body := viper.GetString(ArgGitBody)
		if body == "" {
			body = taskName
		}
		repoPath, err := git.GetCurrentRepoPath()
		if err != nil {
			return err
		}
		b := MustGetBosun(cli.Parameters{ProviderPriority: []string{bosun.WorkspaceProviderName}})

		app, err := getCurrentApp(b)
		if err != nil {
			return err
		}

		svc, err := b.GetIssueService()
		if err != nil {
			return errors.New("get issue service")
		}

		var parentRef *issues.IssueRef
		var parent *issues.Issue

		storyNumber := viper.GetInt(ArgGitTaskStory)
		if storyNumber > 0 {

			parentOrg := viper.GetString(ArgGitTaskParentOrg)
			parentRepo := viper.GetString(ArgGitTaskParentRepo)

			tmp := issues.NewIssueRef(parentOrg, parentRepo, storyNumber)
			parentRef = &tmp

			parentTemp, parentErr := svc.GetIssue(*parentRef)
			if parentErr != nil {
				pkg.Log.WithError(parentErr).Error("Could not get parent issue " + parentRef.String())
			} else {
				parent = &parentTemp
				body = fmt.Sprintf(`%s

## Parent Issue (as of when this issue was created) %s

%s
`, body, parentRef, parent.Body)
			}
		}

		issue := issues.Issue{
			Title:         title,
			Body:          body,
			Org:           org,
			Repo:          repo,
			IsClosed:      false,
			BranchPattern: app.Branching.Feature,
		}

		number, err := svc.Create(issue, parentRef)
		if err != nil {
			return err
		}

		fmt.Println(number)

		g, err := git.NewGitWrapper(repoPath)
		if err != nil {
			return err
		}

		currentBranch := g.Branch()

		branch, err := app.Branching.RenderFeature(issue.Slug(), number)
		if err != nil {
			return errors.Wrap(err, "could not create branch")
		}

		err = g.CreateBranch(branch)
		if err != nil {
			return err
		}

		if parent != nil {
			err = createStoryCommit(g, currentBranch, branch, *parent, issue)
			if err != nil {
				return errors.Wrap(err, "creating story commit")
			}
			check(g.Push())
		}

		return nil
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().StringP(ArgGitTitle, "n", "", "Issue title.")
	cmd.Flags().StringP(ArgGitBody, "m", "", "Issue body.")
	cmd.Flags().String(ArgGitTaskParentOrg, "naveegoinc", "Issue org.")
	cmd.Flags().String(ArgGitTaskParentRepo, "stories", "Issue repo.")
	cmd.Flags().Int(ArgGitTaskStory, 0, "Number of the story to use as a parent.")
})

func createStoryCommit(g git.GitWrapper, baseBranch string, branch string, story issues.Issue, task issues.Issue) error {

	color.Blue("Bosun can create an initial commit on this branch which will help to document the purpose of the branch and tie it back to the story.")

	const skipOption = "Skip initial commit"
	typeOptions := append([]string{skipOption}, conventionalCommitTypeOptions...)

	var selectedType string
	var selectedScope string

	typeQues := &survey.Select{
		Message: "Select the type of change needed to complete this task:",
		Options: typeOptions,
	}

	check(survey.AskOne(typeQues, &selectedType))
	if selectedType == skipOption {
		return nil
	}

	selectedType = strings.Split(selectedType, ":")[0]

	scopeQues := &survey.Input{
		Message: "What is the scope of this change (e.g. component or file name)? (press enter to skip if you don't know yet)",
	}
	check(survey.AskOne(scopeQues, &selectedScope))

	message := fmt.Sprintf(`%s`, selectedType)
	if selectedScope != "" {
		message = fmt.Sprintf("%s(%s)", message, selectedScope)
	}

	message = fmt.Sprintf("%s: %s", message, strings.ToLower(task.Title))
	issueMetadata := BosunIssueMetadata{
		BaseBranch:baseBranch,
		Branch:branch,
		Story:story.Ref().String(),
		Task:task.Ref().String(),
	}
	message = fmt.Sprintf(`%s

%s

%s
`,message, issueMetadata.String(), task.Body)

	_, err := g.Exec("commit", "-m", message, "--allow-empty")

	pkg.Log.Info("Created initial commit.")
	color.Green("%s\n", message)

	return err
}

const (
	ArgGitTitle          = "title"
	ArgGitBody           = "body"
	ArgGitTaskStory      = "story"
	ArgGitTaskParentOrg  = "parent-org"
	ArgGitTaskParentRepo = "parent-repo"
)

type BosunIssueMetadata struct {
	BaseBranch string `yaml:"baseBranch"`
	Branch string `yaml:"branch"`
	Story string `yaml:"story"`
	Task string `yaml:"task"`
}
func (b BosunIssueMetadata) String() string {
	y, _ := yaml.MarshalString(b)
	return fmt.Sprintf("```bosun\n%s\n```", y)
}

var gitTaskShow = addCommand(gitTaskCmd, &cobra.Command{
	Use:   "issue [app]",
	Short: "Shows the issue for the current branch, if any.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := MustGetBosun(cli.Parameters{ProviderPriority: []string{bosun.WorkspaceProviderName}})

		app := mustGetApp(b, args)

		localRepo := app.Repo.LocalRepo

		branch := app.GetBranchName()

		if !app.Branching.IsFeature(branch) {
			return errors.Errorf("%q is not a feature branch (template is %q)", branch, app.Branching.Feature)
		}

		issueNumber, err := app.Branching.GetIssueNumber(branch)
		if err != nil {
			return err
		}

		svc, err := b.GetIssueService()
		if err != nil {
			return err
		}

		org, repo := git.GetOrgAndRepoFromPath(localRepo.Path)

		ref := issues.NewIssueRef(org, repo, issueNumber)

		issue, err := svc.GetIssue(ref)
		if err != nil {
			return err
		}

		return renderOutput(issue)
	},
})
