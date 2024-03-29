package cmd

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	// "log"
)

var storyCmd = addCommand(rootCmd, &cobra.Command{
	Use:   "story",
	Short: "Commands related to stories development.",
})

var storyHandlersCmd = addCommand(storyCmd, &cobra.Command{
	Use:   "handlers",
	Short: "Show story handler configs.",
	Args:  cobra.ExactArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {

		b := MustGetBosun(cli.Parameters{NoEnvironment: true})

		configs := b.GetStoryHandlerConfiguration()

		y, _ := yaml.MarshalString(configs)

		fmt.Println(y)

		return nil

	},
}, func(cmd *cobra.Command) {
})


var storyShowtCmd = addCommand(storyCmd, &cobra.Command{
	Use:   "show {story}",
	Short: "Show information about a story.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var storyID = args[0]

		b := MustGetBosun(cli.Parameters{NoEnvironment: true})

		storyHandler, err := GetStoryHandler(b, storyID)
		if err != nil {
			return err
		}

		story, err := storyHandler.GetStory(storyID)
		if err != nil {
			return err
		}

		y, _ := yaml.MarshalString(story)

		fmt.Println(y)

		if viper.GetBool(argStoryShowDetails) {
			color.Blue("Details:\n")
			y, _ = yaml.MarshalString(story.ProviderState)
			fmt.Println(y)
		}

		if viper.GetBool(argStoryShowBranches) {
			color.Blue("Branches:\n")

			branches, branchesErr := storyHandler.GetBranches(story)
			if branchesErr != nil {
				return branchesErr
			}

			branchesErr = printOutputWithDefaultFormat("t", branches)
			if branchesErr != nil {
				return branchesErr
			}

		}

		return nil

	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(argStoryShowDetails, false, "Show provider details")
	cmd.Flags().Bool(argStoryShowBranches, false, "Show branches for the story")
})

const (
	argStoryShowDetails  = "details"
	argStoryShowBranches = "branches"
)
