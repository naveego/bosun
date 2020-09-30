package cmd

import (
	"github.com/spf13/cobra"
	// "log"
)


var featureQACmd = addCommand(featureCmd, &cobra.Command{
	Use:     "qa",
	Short:   "Commands related to testing the feature.",
})
