package cmd

import (
	"fmt"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/stories"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	// "log"
)


var storyQACmd = addCommand(storyCmd, &cobra.Command{
	Use:     "qa",
	Short:   "Commands related to testing the story.",
})


var storyQAShowBranchesCmd =

	&cobra.Command{
	Use:   "branches {story}",
	Short: "Show branches related to a story.",
	Args:  cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		var storyID = args[0]

		b := MustGetBosun(cli.Parameters{
			ProviderPriority: []string{bosun.WorkspaceProviderName},
			NoEnvironment: true,
		})
		var err error
		var story *stories.Story
		var storyHandler stories.StoryHandler
		storyHandler, err = GetStoryHandler(b, storyID)
		if err != nil {
			return err
		}

		story, err = storyHandler.GetStory(storyID)
		if err != nil {
			return err
		}


		branches, err := storyHandler.GetBranches(story)
		if err != nil {
			return err
		}

		var items []pterm.BulletListItem


		for _, branch := range branches {
			items = append(items, pterm.BulletListItem{Text: fmt.Sprintf("%s - %s", branch.Repo, branch.Branch)})
		}

		return pterm.DefaultBulletList.WithItems(items).Render()
	},
}

var _ = addCommand(storyQACmd, storyQAShowBranchesCmd)
var _ = addCommand(storyDevCmd, storyQAShowBranchesCmd)
