package cmd

import (
	"fmt"
	"github.com/aryann/difflib"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"strings"
)

// releaseCmd represents the release command
var releasePlanCmd = addCommand(releaseCmd, &cobra.Command{
	Use:     "plan",
	Aliases: []string{"planning", "p"},
	Short:   "Contains sub-commands for release planning.",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
})

var releasePlanShowCmd = addCommand(releasePlanCmd, &cobra.Command{
	Use:          "show",
	Aliases:      []string{"dump"},
	Short:        "Shows the current release plan.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b, p := getReleaseCmdDeps()

		plan, err := p.GetPlan(b.NewContext())
		if err != nil {
			return err
		}

		err = printOutput(plan)
		return err
	},
})

var releasePlanEditCmd = addCommand(releasePlanCmd, &cobra.Command{
	Use:          "edit",
	Short:        "Opens release plan in an editor.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b, p := getReleaseCmdDeps()

		plan, err := p.GetPlan(b.NewContext())
		if err != nil {
			return err
		}

		err = cli.Edit(plan.FromPath)

		return err
	},
})

var releasePlanStartCmd = addCommand(releasePlanCmd, &cobra.Command{
	Use:     "start",
	Aliases: []string{"create"},
	Short:   "Begins planning a new release.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b, p := getReleaseCmdDeps()
		ctx := b.NewContext()

		var err error
		var version semver.Version
		versionString := viper.GetString(ArgReleasePlanStartVersion)
		if versionString != "" {
			version, err = semver.NewVersion(versionString)
			if err != nil {
				return errors.Errorf("invalid version: %s", err)
			}
		}

		settings := bosun.ReleasePlanSettings{
			Name:         viper.GetString(ArgReleasePlanStartName),
			Version:      version,
			Bump:         viper.GetString(ArgReleasePlanStartBump),
		}

		_, err = p.CreateReleasePlan(ctx, settings)
		if err != nil {
			return err
		}

		err = p.Save(ctx)

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().String(ArgReleasePlanStartName, "", "The name of the release (defaults to the version if not provided).")
	cmd.Flags().String(ArgReleasePlanStartVersion, "", "The version of the release.")
	cmd.Flags().String(ArgReleasePlanStartBump, "", "The version bump of the release.")
})

const (
	ArgReleasePlanStartName        = "name"
	ArgReleasePlanStartVersion     = "version"
	ArgReleasePlanStartBump        = "bump"
)

var releasePlanDiscardCmd = addCommand(releasePlanCmd, &cobra.Command{
	Use:   "discard",
	Args:  cobra.NoArgs,
	Short: "Discard the current release plan.",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, p := getReleaseCmdDeps()
		plan, err := p.GetCurrentRelease()
		if err != nil {
			return err
		}

		err = plan.IsMutable()
		if err != nil {
			return err
		}
		branch, err := p.GetCurrentBranch()
		if err != nil {
			return err
		}
		g, err := git.NewGitWrapper(p.FromPath)
		if err != nil {
			return err
		}

		if cli.RequestConfirmFromUser("Are you sure you want to discard the current release plan?") {
			_, err = g.Exec("reset", "--hard")
			if err != nil {
				return err
			}
        	check(g.CheckOutOrCreateBranch(p.Branching.Develop))
			_, err = g.Exec("branch", "-D", branch.String())
			return err
		}

		return nil
	},
})

var releasePlanValidateCmd = addCommand(releasePlanCmd, &cobra.Command{
	Use:          "validate",
	Args:         cobra.NoArgs,
	Short:        "Validates the current release plan.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b, p := getReleaseCmdDeps()
		ctx := b.NewContext()
		results, err := p.ValidatePlan(ctx)
		if err != nil {
			return err
		}

		hadErrs := false

		for _, k := range util.SortedKeys(results) {
			result := results[k]
			fmt.Println(k)
			if result.Err != nil {
				lines := strings.Split(result.Err.Error(), "\n")
				for _, line := range lines {
					color.Red("  %s\n", line)
				}
				hadErrs = true
			} else if result.Message != "" {
				color.Green("  %s\n", result.Message)
			} else {
				color.Green("  OK\n")
			}
		}

		if hadErrs {
			return errors.New("at least one app invalid")
		}
		return nil
	},
})

var releasePlanCommitCmd = addCommand(releasePlanCmd, &cobra.Command{
	Use:   "commit",
	Args:  cobra.NoArgs,
	Short: "GetCurrentCommit the current release plan.",
	RunE: func(cmd *cobra.Command, args []string) error {
		b, p := getReleaseCmdDeps()
		ctx := b.NewContext()
		_, err := p.CommitPlan(ctx)

		if err != nil {
			return err
		}

		err = p.Save(ctx)
		if err != nil {
			return err
		}

		err = b.UseRelease(bosun.SlotStable)
		if err != nil {
			return err
		}

		return b.Save()
	},
})

var releasePlanAppCmd = addCommand(releasePlanCmd, &cobra.Command{
	Use:   "app",
	Short: "Sets the disposition of an app in the release.",
	Long:  "Alternatively, you can edit the plan directly in the platform yaml file.",
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		b, p := getReleaseCmdDeps()

		plan, err := p.GetPlan(b.NewContext())
		if err != nil {
			return err
		}

		ctx := b.NewContext()

		var apps []*bosun.App
		fp := getFilterParams(b, args)
		if !fp.IsEmpty() {
			apps, _ = fp.GetAppsChain(fp.Chain().Including(filter.MustParse(core.LabelDeployable)))
		}

		appPlans := map[string]*bosun.AppPlan{}

		if len(apps) > 0 {
			for _, app := range apps {
				if intent, ok := plan.Apps[app.Name]; ok {
					appPlans[app.Name] = intent
				}
			}
		} else {
			var appPlanList []*bosun.AppPlan
			for _, name := range util.SortedKeys(plan.Apps) {
				appPlan := plan.Apps[name]
				appPlanList = append(appPlanList, appPlan)
			}

			selectAppUI := promptui.Select{
				Label:             "Select an app",
				Items:             appPlanList,
				StartInSearchMode: true,
				Templates:         editStatusTemplates,
			}
			index, _, runErr := selectAppUI.Run()
			if runErr != nil {
				return runErr
			}

			selectedAppPlan := appPlanList[index]
			appPlans[selectedAppPlan.Name] = selectedAppPlan
		}

		changes := map[string][]difflib.DiffRecord{}
		for _, appPlan := range appPlans {
			original := MustYaml(appPlan)

			deploySet := cmd.Flags().Changed(ArgReleaseSetStatusDeploy)
			var deploy bool
			if deploySet {
				deploy = viper.GetBool(ArgReleaseSetStatusDeploy)
			} else {

				deployUI := promptui.Prompt{
					Label: fmt.Sprintf("Do you want to deploy %q? [y/N] ", appPlan.Name),
				}

				deployResult, deployErr := deployUI.Run()
				if deployErr != nil {
					return deployErr
				}

				deploy = strings.HasPrefix(strings.ToLower(deployResult), "y")
			}

			appPlan.Deploy = deploy

			reason := viper.GetString(ArgReleaseSetStatusReason)
			if reason == "" {

				reasonUI := promptui.Prompt{
					Label:     fmt.Sprintf("Why do you want to make this decision for %s? ", appPlan.Name),
					AllowEdit: true,
				}

				reason, err = reasonUI.Run()
				if err != nil {
					return err
				}
			}

			bump := viper.GetString(ArgReleaseSetStatusBump)
			if bump == "" {
				bumpUI := promptui.Select{
					Label: fmt.Sprintf("What kind of version bump is appropriate for %q", appPlan.Name),
					Items: []string{"none", "patch", "minor", "major"},
				}
				_, bump, err = bumpUI.Run()
				if err != nil {
					return err
				}
			}
			appPlan.BumpOverride = semver.Bump(bump)

			updated := MustYaml(appPlan)

			changes[appPlan.Name] = diffStrings(original, updated)
		}

		for name, diffs := range changes {
			fmt.Printf("Changes to %q:\n", name)
			for _, diff := range diffs {
				if diff.Delta != difflib.Common {
					fmt.Println(renderDiff(diff))
				}
			}
		}

		err = p.Save(ctx)
		if err != nil {
			return err
		}

		return nil
	},
}, withFilteringFlags,
	func(cmd *cobra.Command) {
		cmd.Flags().Bool(ArgReleaseSetStatusDeploy, false, "Set to deploy matched apps.")
		cmd.Flags().String(ArgReleaseSetStatusReason, "", "The reason to set for the status change for matched apps.")
		cmd.Flags().String(ArgReleaseSetStatusBump, "", "The version bump to apply to upgrades among matched apps.")
	})

const (
	ArgReleaseSetStatusDeploy = "deploy"
	ArgReleaseSetStatusReason = "reason"
	ArgReleaseSetStatusBump   = "bump"
)

var editStatusTemplates = &promptui.SelectTemplates{
	Label:    "{{ . }}:",
	Active:   "> {{ .String | cyan }}",
	Inactive: "  {{ .String }}",
	Selected: "> {{ .String }}",
	Details:  ``,
}
