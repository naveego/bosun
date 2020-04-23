package cmd

import (
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/workspace"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var appToggleCmd = &cobra.Command{
	Use:          "toggle [name] [name...]",
	Short:        "Toggles or sets where traffic for an app will be routed to.",
	Long:         "If app is not specified, the first app in the nearest bosun.yaml file is used.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		viper.BindPFlags(cmd.Flags())

		// Set force to true so toggling back to minikube forces a deploy
		b := MustGetBosun(cli.Parameters{Force: true})
		p, err := b.GetCurrentPlatform()
		check(err)

		c := b.GetCurrentEnvironment()

		if err := b.ConfirmEnvironment(); err != nil {
			return err
		}

		if c.Name != "red" {
			return errors.New("Environment must be set to 'red' to toggle services.")
		}

		repos, err := getAppsIncludeCurrent(b, args)
		if err != nil {
			return err
		}
		apps, err := getAppDeploysFromApps(b, repos)
		if err != nil {
			return err
		}

		ctx := b.NewContext()

		appServiceChanged := false

		for _, app := range apps {

			ctx = ctx.WithAppDeploy(app)
			wantsLocalhost := viper.GetBool(ArgSvcToggleLocalhost)
			wantsMinikube := viper.GetBool(ArgSvcToggleMinikube)
			if wantsLocalhost {
				app.DesiredState.Routing = workspace.RoutingLocalhost
			} else if wantsMinikube {
				app.DesiredState.Routing = workspace.RoutingCluster
			} else {
				switch app.DesiredState.Routing {
				case workspace.RoutingCluster:
					app.DesiredState.Routing = workspace.RoutingLocalhost
				case workspace.RoutingLocalhost:
					app.DesiredState.Routing = workspace.RoutingCluster
				default:
					app.DesiredState.Routing = workspace.RoutingCluster
				}
			}

			if app.DesiredState.Routing == workspace.RoutingLocalhost {
				err = app.RouteToLocalhost(ctx, "")
				if err != nil {
					return err
				}

				appServiceChanged = true
			} else {
				// force upgrade the app to restore it to its normal state.
				ctx.Log().Info("Re-deploying app.")

				err = deployApps(b, p, []string{app.Name}, nil, []string{app.Name})

				if err != nil {
					color.Yellow("-------------------------\n")
					color.Blue("-------------------------\n")
					color.Red("Got an error, but toggling is fragile so you should try running this at least twice to see if it works.\n")
					color.Blue("-------------------------\n")
					color.Yellow("-------------------------\n")
					return err
				}
				appServiceChanged = true
			}

			b.SetDesiredState(app.Name, app.DesiredState)
		}

		if appServiceChanged {

			client, err := kube.GetKubeClient()
			if err != nil {
				return errors.Wrap(err, "get kube client for tweaking service")
			}

			ctx.Log().Warn("Recycling kube-dns to ensure new services are routed correctly.")

			podClient := client.CoreV1().Pods("kube-system")
			pods, err := podClient.List(metav1.ListOptions{
				LabelSelector: "k8s-app=kube-dns",
			})
			if err != nil {
				return errors.Wrap(err, "find kube-dns")
			}
			if len(pods.Items) == 0 {
				return errors.New("no kube-dns pods found")
			}
			for _, pod := range pods.Items {
				ctx.Log().Warnf("Deleting pod %q...", pod.Name)
				err = podClient.Delete(pod.Name, metav1.NewDeleteOptions(0))
				if err != nil {
					return errors.Wrapf(err, "delete pod %q", pod.Name)
				}
				ctx.Log().Warnf("Pod %q deleted. Kube-hosted services may be unavailable for a short time.", pod.Name)
			}
		}

		err = b.Save()

		return err
	},
}
