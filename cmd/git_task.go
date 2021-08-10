package cmd

import (
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/jira"
	"github.com/naveego/bosun/pkg/stories"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"strings"

	// "os"
)

func init() {
	stories.RegisterFactory(jira.Factory)
}

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

		body := viper.GetString(ArgGitBody)
		story := viper.GetString(ArgGitTaskStory)

		return StartFeatureDevelopment(taskName, body, story)
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().StringP(ArgGitTitle, "n", "", "Issue title.")
	cmd.Flags().StringP(ArgGitBody, "m", "", "Issue body.")
	cmd.Flags().String(ArgGitTaskParentOrg, "naveegoinc", "Issue org.")
	cmd.Flags().String(ArgGitTaskParentRepo, "stories", "Issue repo.")
	cmd.Flags().Int(ArgGitTaskStory, 0, "ID of the story to use as a parent.")
})

func StartFeatureDevelopment(taskName string, body string, storyID string) error {
	var err error

	org, repo := git.GetCurrentOrgAndRepo().OrgAndRepo()
	var title string
	if len(taskName) > 50 {
		title = taskName[:50] + "..."
	} else {
		title = taskName
	}

	if storyID == "" {
		return errors.New("story ID is required")
	}

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

	var story *stories.Story

	var storyHandler stories.StoryHandler

	storyHandler, err = GetStoryHandler(b, storyID)
	if err != nil {
		return err
	}

	storyTmp, storyErr := storyHandler.GetStory(storyID)
	if storyErr != nil {
		core.Log.WithError(storyErr).Errorf("Could not get story with ID %q", storyID)
	} else if storyTmp != nil {
		story = storyTmp
		body = fmt.Sprintf(`%s

## Parent Issue (as of when this issue was created) %s

%s
`, body, story.URL, story.Body)
	}

	issue := issues.Issue{
		Title:         title,
		Body:          body,
		Org:           org,
		Repo:          repo,
		IsClosed:      false,
		BranchPattern: app.Branching.Feature,
	}

	g, err := git.NewGitWrapper(repoPath)
	if err != nil {
		return err
	}

	ctx := b.NewContext()

	currentBranch := g.Branch()

	branch, err := app.Branching.RenderFeature(issue.Slug(), storyID)
	if err != nil {
		return errors.Wrap(err, "could not create branch")
	}

	err = g.CreateBranch(branch)
	if err != nil {
		return err
	}

	if story != nil {
		err = createStoryCommit(g, currentBranch, branch, *story, issue)
		if err != nil {
			return errors.Wrap(err, "creating story commit")
		}
		check(g.Push())


		if storyHandler == nil {
			ctx.Log().Info("No story handler, will not attempt to move story to In Development")
		} else {
			ctx.Log().Info("Attempting to to move story to In Development")
			event, validationErr := stories.Event{
				Payload: stories.EventBranchCreated{},
				StoryID: storyID,
				Story:   story,
				Issue:   issue.RefPtr(),
			}.Validated()
			if validationErr != nil {
				return validationErr
			}
			err = storyHandler.HandleEvent(event)
			if err != nil {
				return err
			}
			ctx.Log().Info("Story moved to In Development")
		}
	} else {
		ctx.Log().Info("No story found, will not attempt to create initial commit or move story")
	}

	return nil
}

func createStoryCommit(g git.GitWrapper, baseBranch string, branch string, story stories.Story, task issues.Issue) error {

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
		BaseBranch: baseBranch,
		Branch:     branch,
		Story:      story.ID,
		Task:       task.Ref().String(),
	}
	message = fmt.Sprintf(`%s

%s

%s
`, message, issueMetadata.String(), task.Body)

	_, err := g.Exec("commit", "-m", message, "--allow-empty", "--no-verify")

	core.Log.Info("Created initial commit.")
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
	Branch     string `yaml:"branch"`
	Story      string `yaml:"story"`
	Task       string `yaml:"task"`
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

		org, repo := git.GetRepoRefFromPath(localRepo.Path).OrgAndRepo()

		ref := issues.NewIssueRef(org, repo, issueNumber)

		issue, err := svc.GetIssue(ref)
		if err != nil {
			return err
		}

		return renderOutput(issue)
	},
})

func GetStoryHandler(b *bosun.Bosun, storyID string) (stories.StoryHandler, error) {
	stories.Configure(b.GetStoryHandlerConfiguration())
	return stories.GetStoryHandler(b.NewContext(), storyID)
}
