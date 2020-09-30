package cmd

import (
	"github.com/spf13/cobra"
	// "log"
)

var featureCmd = addCommand(rootCmd, &cobra.Command{
	Use:     "feature",
	Aliases: []string{"feat"},
	Short:   "Commands related to feature development.",

})