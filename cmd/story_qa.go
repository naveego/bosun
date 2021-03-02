package cmd

import (
	"github.com/spf13/cobra"
	// "log"
)


var storyQACmd = addCommand(storyCmd, &cobra.Command{
	Use:     "qa",
	Short:   "Commands related to testing the story.",
})
