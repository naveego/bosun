package cmd

import (
	"github.com/spf13/cobra"
)

var appDeployCmd = addCommand(appCmd, &cobra.Command{
	Use:   "deploy [name] [name...]",
	Short: "Deploys the requested app.",
	Long: `If app is not specified, the first app in the nearest bosun.yaml file is used.

If you want to apply specific value sets to this deployment, use the --value-sets (or -v) flag.
You can view the available value-sets using "bosun env value-sets". 

A common use case for using value-sets is if you want to change the tag and pull behavior so that
the pod will use an image you just built using the minikube docker agent. The "latest" and "pullIfNotPresent"
value-sets are available for this. To use them:

bosun app deploy {appName} --value-sets latest,pullIfNotPresent
`,
	SilenceUsage: true,
	RunE: deployApp,
}, deployAppFlags)

const (
	argAppDeployValuesOnly = "dump-values"
	argAppDeployRenderOnly = "render-only"
)
