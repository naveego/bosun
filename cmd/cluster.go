package cmd

import (
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/spf13/cobra"
)

// kubeCmd represents the kube command
var clusterCmd = addCommand(rootCmd, &cobra.Command{
	Use:   "cluster",
	Short: "Commands for interacting with clusters.",
})

var clusterListDefinitionsCmd = addCommand(clusterCmd, &cobra.Command{
	Use:          "list",
	Aliases:      []string{"ls"},
	Short:        "Lists all cluster definitions. ",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b := MustGetBosun(cli.Parameters{NoCluster: true, NoEnvironment: true})

		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		clusters, err := p.GetClusters()
		if err != nil {
			return err
		}

		return renderOutput(clusters)
	},
})

var clusterConfigureClusterCmd = addCommand(clusterCmd, &cobra.Command{
	Use:   "configure [name]",
	Args:  cobra.MaximumNArgs(1),
	Short: "Configures the specified cluster, or the current cluster if none specified",
	RunE: func(cmd *cobra.Command, args []string) error {

		b := MustGetBosun()
		ctx := b.NewContext()

		p, err := b.GetCurrentPlatform()
		if err != nil {
			return err
		}

		clusters, err := p.GetClusters()
		if err != nil {
			return err
		}

		var cluster *kube.Cluster
		if len(args) == 1 {
			name := args[0]
			cluster, err = clusters.GetPossiblyUnconfiguredCluster(name, ctx)
			if err != nil {
				return err
			}
		} else {
			cluster, err = b.GetCurrentCluster()
			if err != nil {
				return err
			}
		}

		err = cluster.ConfigureKubectl()

		return err
	},
}, func(cmd *cobra.Command) {
})
