package cmd

import (
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/spf13/cobra"
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

			b := MustGetBosun(cli.Parameters{NoEnvironment:true})
			app := mustGetApp(b, args)

			helper := bosun.NewAppImageHelper(b)
			req := bosun.PublishImagesRequest{App:app}

			err := helper.PublishImages(req)
			return err
		},
	})