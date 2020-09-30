package cmd

import (
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"strconv"

	// "log"
)

var featureDevCmd = addCommand(featureCmd, &cobra.Command{
	Use:   "dev",
	Short: "Commands related to developing the feature.",
})

var featureDevStartCmd = addCommand(featureDevCmd, &cobra.Command{
	Use:   "start {story number} [title] [body]",
	Short: "Start development on a feature.",
	Args:  cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error
		if err != nil {
			return err
		}

		storyNumber, err := strconv.Atoi(args[0])
		if err != nil {
			return errors.Wrapf(err, "invalid story number")
		}

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

		parentOrg := viper.GetString(ArgGitTaskParentOrg)
		parentRepo := viper.GetString(ArgGitTaskParentRepo)
		parent := issues.NewIssueRef(parentOrg, parentRepo, storyNumber)

		return StartFeatureDevelopment(title, body, &parent)
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().String(ArgGitTaskParentOrg, "naveegoinc", "Issue org.")
	cmd.Flags().String(ArgGitTaskParentRepo, "stories", "Issue repo.")
})
