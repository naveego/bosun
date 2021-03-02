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

		return nil

	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(argStoryShowDetails, false, "Show provider details")
})

const (
	argStoryShowDetails = "details"
)
