package cmd

import (
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var appPublishImageCmd = addCommand(
	appCmd,
	&cobra.Command{
		Use:     "publish-image [app]",
		Aliases: []string{"publish-images"},
		Args:    cobra.MaximumNArgs(1),
		Short:   "Publishes the image for an app.",
		Long: `If app is not provided, the current directory is used.
The image will be published with the "latest" tag and with a tag for the current version.
If the current branch is a release branch, the image will also be published with a tag formatted
as "version-release".
`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {

			b := MustGetBosunNoEnvironment()
			app := mustGetApp(b, args)

			helper := bosun.NewAppImageHelper(b)
			req := bosun.PublishImagesRequest{App:app, Pattern: viper.GetString(ArgImagePattern)}

			err := helper.PublishImages(req)
			return err
		},
	}, func(cmd *cobra.Command) {
		cmd.Flags().String(ArgImagePattern, "", "filter pattern for images to actually process")
	})


var appBuildImageCmd = addCommand(
	appCmd,
	&cobra.Command{
		Use:           "build-image [app]",
		Aliases:       []string{"build-images"},
		Args:          cobra.MaximumNArgs(1),
		Short:         "Builds the image(s) for an app.",
		Long:          `If app is not provided, the current directory is used. The image(s) will be built with the "latest" tag.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			b := MustGetBosunNoEnvironment()
			app := mustGetApp(b, args)
			ctx := b.NewContext().WithApp(app)
			req := bosun.BuildImageRequest{
				Ctx: ctx,
				Pattern: viper.GetString(ArgImagePattern),
			}
			err := app.BuildImages(req)
			return err
		},
	}, func(cmd *cobra.Command) {
		cmd.Flags().String(ArgImagePattern, "", "filter pattern for images to actually process")
	})

const (
	ArgImagePattern = "pattern"
)