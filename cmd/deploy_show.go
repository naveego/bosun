package cmd

import (
	"fmt"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/spf13/cobra"
	"path/filepath"
)

var deployShowCmd = addCommand(deployCmd, &cobra.Command{
	Use:          "show [release|stable|unstable]",
	Short:        "Show the deployment plan and its progress.",
	Args:         cobra.RangeArgs(0, 1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {

		b := MustGetBosun()

		plan, err := getPlan(b, args)
		if err != nil {
			return err
		}

		stack, err := b.GetCurrentStack()
		 if err != nil {
		 	return err
		 }

		stackBrn := stack.Brn


		report := plan.GetDeploymentProgressReportForStack(stackBrn)

		return renderOutput(report)
	},
})

func getPlan(b *bosun.Bosun, args []string)(*bosun.DeploymentPlan, error) {

	p, err := b.GetCurrentPlatform()
	if err != nil {
		return nil, err
	}

	pathOrSlot := "release"

	if len(args) > 0 {
		pathOrSlot = args[0]
	}

	var path string
	var plan *bosun.DeploymentPlan

	switch pathOrSlot {
	case "release", "current", bosun.SlotStable, bosun.SlotUnstable:
		_, folder, resolveReleaseErr := getReleaseAndPlanFolderName(b, pathOrSlot)
		if resolveReleaseErr != nil {
			return nil, resolveReleaseErr
		}

		path = filepath.Join(p.GetDeploymentsDir(), fmt.Sprintf("%s/plan.yaml", folder))
		break
	default:
		path = pathOrSlot
		break
	}

	plan, err = bosun.LoadDeploymentPlanFromFile(path)

	return plan, err
}