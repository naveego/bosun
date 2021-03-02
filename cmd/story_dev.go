package cmd

import (
	"fmt"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/stories"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"strings"

	// "log"
)

var storyDevCmd = addCommand(storyCmd, &cobra.Command{
	Use:   "dev",
	Short: "Commands related to developing the story.",
})

var storyDevStartCmd = addCommand(storyDevCmd, &cobra.Command{
	Use:   "start {story} [title] [body]",
	Short: "Start development on a story.",
	Args:  cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		var story = args[0]

		var title, body string
		if len(args) > 1 {
			title = args[1]
		}
		if len(args) > 2 {
			body = args[2]
		}
		if title == "" {
			title = cli.RequestStringFromUser("Issue title")
		}
		if body == "" {
			body = cli.RequestStringFromUser("Issue description")
		}

		return StartFeatureDevelopment(title, body, story)
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().String(ArgGitTaskParentOrg, "naveegoinc", "Issue org.")
	cmd.Flags().String(ArgGitTaskParentRepo, "stories", "Issue repo.")
})

var storyDevConnectCmd = addCommand(storyDevCmd, &cobra.Command{
	Use:   "connect-branch {story} {org/repo/number}",
	Short: "Connects the current branch with the provided issue to a story.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {

		var storyID = args[0]

		issueRef, err := issues.ParseIssueRef(args[1])
		if err != nil {
			return errors.Wrap(err, "invalid issue ref")
		}

		b := MustGetBosun(cli.Parameters{NoEnvironment: true})

		storyHandler, err := GetStoryHandler(b, storyID)
		if err != nil {
			return err
		}

		story, err := storyHandler.GetStory(storyID)
		if err != nil {
			return err
		}
		wd, _ := os.Getwd()
		g, err := git.NewGitWrapper(wd)
		if err != nil {
			return err
		}

		branch := g.Branch()

		event, err := stories.Event{
			Payload: stories.EventBranchCreated{
				Branch: branch,
			},
			URL:   fmt.Sprintf("https://github.com/%s/%s/tree/%s", issueRef.Org, issueRef.Repo, strings.ReplaceAll(branch, "/", "%2f")),
			Issue: &issueRef,
			Story: story,
			StoryID: storyID,
		}.Validated()
		if err != nil {
			return err
		}

		err = storyHandler.HandleEvent(event)

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().String(ArgGitTaskParentOrg, "naveegoinc", "Issue org.")
	cmd.Flags().String(ArgGitTaskParentRepo, "stories", "Issue repo.")
})
