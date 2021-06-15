package cmd

import (
	"fmt"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/stories"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/util/stringsn"
	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	// "log"
)

var storyQACmd = addCommand(storyCmd, &cobra.Command{
	Use:   "qa",
	Short: "Commands related to testing the story.",
})

var storyQAShowBranchesCmd = &cobra.Command{
	Use:   "branches {story}",
	Short: "Show branches related to a story.",
	Args:  cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error

		b := MustGetBosun(cli.Parameters{
			ProviderPriority: []string{bosun.WorkspaceProviderName},
			NoEnvironment:    true,
		})

		var items []pterm.BulletListItem

		qaStory, err := getQAStory(b, args[0])
		if err != nil {
			return err
		}

		for _, app := range qaStory.Apps {
			items = append(items, pterm.BulletListItem{Text: fmt.Sprintf("%s - %s (%s)", app.Branch.Repo, app.Branch.Branch, app.App.Name)})
		}

		return pterm.DefaultBulletList.WithItems(items).Render()
	},
}

var storyQAStartCmd = addCommand(storyQACmd, &cobra.Command{
	Use:   "start {story}",
	Short: "Create a stack for a story and deploy to it.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error

		b := MustGetBosun(cli.Parameters{
			ProviderPriority: []string{bosun.WorkspaceProviderName},
		})

		storyID := args[0]

		qaStory, err := getQAStory(b, args[0])
		if err != nil {
			return errors.Wrapf(err, "could not get story using ID %q", storyID)
		}

		err = qaStory.createOrLoadStack()

		if err != nil {
			return errors.Wrap(err, "loading stack")
		}

		err = b.UseStack(qaStory.Stack.Brn)
		if err != nil {
			return err
		}

		provider := cli.RequestChoice("What baseline do you want to deploy to the stack?", bosun.SlotStable, bosun.SlotUnstable, bosun.WorkspaceProviderName)

		err = resetCurrentStack(qaStory.Bosun, qaStory.Platform, provider)
		if err != nil {
			return errors.Wrapf(err, "reseting stack")
		}

		err = qaStory.deployStoryApps(nil)

		if err != nil {
			return errors.Wrap(err, "deploying story apps")
		}

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(argQAStoryStartCreateStack, false, "Create a new stack")
	cmd.Flags().String(argQAStoryStartStackName, "", "Name of stack to use or create.")
	cmd.Flags().String(argQAStoryStartStackTemplate, "", "Template of stack to use or create.")
	cmd.Flags().Bool(argQAStoryValidateOnly, false, "Only validate the deployment")
	cmd.Flags().Bool(argDeployExecuteSkipValidate, false, "Skip validation of the deployment")
	cmd.Flags().Bool(argDeployExecuteDiffOnly, false, "Display the diffs for the deploy, but do not actually execute.")
	cmd.Flags().Bool(argDeployExecuteValuesOnly, false, "Display the values which would be used for the deploy, but do not actually execute.")
})

var storyQADeployCmd = addCommand(storyQACmd, &cobra.Command{
	Use:   "deploy [apps...]",
	Short: "Deploy the apps for a story to the current stack. If the current stack doesn't have a story assigned you will be prompted to set it.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error
		var stack *kube.Stack
		b := MustGetBosun(cli.Parameters{
			ProviderPriority: []string{bosun.WorkspaceProviderName},
		})

		var storyID string

		stack, err = b.GetCurrentStack()
		if err != nil {
			return err
		}
		storyID = stack.GetStoryID()
		if storyID == "" {
			storyID = cli.RequestStringFromUser("The stack %q has no story assigned, what story do you want to assign?", stack.Name)
			stack.SetStoryID(storyID)
			err = stack.Save()
			if err != nil {
				return err
			}
		}

		if storyID == "" {
			return errors.New("no story ID")
		}

		qaStory, err := getQAStory(b, storyID)
		if err != nil {
			return errors.Wrapf(err, "could not get story using ID %q", storyID)
		}

		err = qaStory.createOrLoadStack()

		if err != nil {
			return err
		}

		err = qaStory.deployStoryApps(args)

		if err != nil {
			return err
		}

		return err
	},
}, func(cmd *cobra.Command) {
	cmd.Flags().Bool(argQAStoryValidateOnly, false, "Only validate the deployment")
	cmd.Flags().Bool(argDeployExecuteSkipValidate, false, "Skip validation of the deployment")
	cmd.Flags().Bool(argDeployExecuteDiffOnly, false, "Display the diffs for the deploy, but do not actually execute.")
	cmd.Flags().Bool(argDeployExecuteValuesOnly, false, "Display the values which would be used for the deploy, but do not actually execute.")
})

const (
	argQAStoryValidateOnly       = "validate-only"
	argQAStoryStartCreateStack   = "create-stack"
	argQAStoryStartStackName     = "name"
	argQAStoryStartStackTemplate = "template"
)

func (q *QAStory) createOrLoadStack() error {

	var err error
	q.Cluster, err = q.Bosun.GetCurrentCluster()
	if err != nil {
		return err
	}

	stacks, err := q.Cluster.GetStackStates()
	if err != nil {
		return err
	}

	stackName := viper.GetString(argQAStoryStartStackName)

	if stackName == "" {
		for _, stackState := range stacks {
			if stackState.StoryID == q.Story.ID {
				stackName = stackState.Name
				break
			}
		}
	}

	createNew := viper.GetBool(argQAStoryStartCreateStack)
	if stackName == "" {
		if !createNew {
			stackChoices := append([]string{"CREATE NEW"}, util.SortedKeys(stacks)...)
			stackName = cli.RequestChoice("Choose a stack to use to test this story", stackChoices...)
			createNew = stackName == "CREATE NEW"
		}
	}

	if createNew {
		if stackName == "" || stackName == "CREATE NEW" {
			stackName = cli.RequestStringFromUser("Enter a name for the new stack")
		}

		err = q.createStack(stackName, viper.GetString(argQAStoryStartStackTemplate))
		if err != nil {
			return err
		}
	} else {
		q.Stack, err = q.Cluster.GetStack(stackName)
		if err != nil {
			return err
		}
	}
	q.Stack.SetStoryID(q.Story.ID)
	err = q.Stack.Save()
	if err != nil {
		return err
	}
	return nil
}

func (q *QAStory) createStack(name, stackTemplateName string) error {

	if q.Cluster == nil {
		panic("createOrLoad stack needs to be called first")
	}

	var templateChoices []string
	for _, template := range q.Cluster.StackTemplates {
		templateChoices = append(templateChoices, template.Name)
	}

	if stackTemplateName == "" {
		stackTemplateName = cli.RequestChoice("Choose a template for your stack", templateChoices...)
	}

	if name == "" {
		name = cli.RequestStringFromUser("Enter a name for the stack")
	}

	var err error
	q.Stack, err = q.Cluster.CreateStack(name, stackTemplateName)

	if err != nil {
		return err
	}

	err = q.Stack.Initialize()

	return err
}

func (q *QAStory) deployStoryApps(appNameFilter []string) error {
	var req = bosun.CreateDeploymentPlanRequest{
		AppOptions:            map[string]bosun.AppProviderRequest{},
		AutomaticDependencies: false,
		IgnoreDependencies:    true,
	}

	for _, app := range q.Apps {

		if len(appNameFilter) > 0 && !stringsn.Contains(app.App.Name, appNameFilter) {
			continue
		}

		req.AppOptions[app.App.Name] = bosun.AppProviderRequest{
			Name:             app.App.Name,
			Branch:           app.Branch.Branch.String(),
			ProviderPriority: []string{bosun.WorkspaceProviderName},
		}
	}

	planCreator := bosun.NewDeploymentPlanCreator(q.Bosun, q.Platform)

	plan, err := planCreator.CreateDeploymentPlan(req)

	if err != nil {
		return err
	}

	executeRequest := bosun.ExecuteDeploymentPlanRequest{
		Plan:           plan,
		Recycle:        viper.GetBool(argDeployAppRecycle),
		DumpValuesOnly: viper.GetBool(argAppDeployValuesOnly),
		DiffOnly:       viper.GetBool(argDeployAppDiffOnly),
		Validate:       !viper.GetBool(argDeployExecuteSkipValidate),
		ValidateOnly:   viper.GetBool(argQAStoryValidateOnly),
	}

	executor := bosun.NewDeploymentPlanExecutor(q.Bosun, q.Platform)

	_, err = executor.Execute(executeRequest)

	return err

}

func getQAStory(b *bosun.Bosun, storyID string) (*QAStory, error) {

	var err error
	var story *stories.Story
	var storyHandler stories.StoryHandler
	storyHandler, err = GetStoryHandler(b, storyID)
	if err != nil {
		return nil, err
	}

	story, err = storyHandler.GetStory(storyID)
	if err != nil {
		return nil, err
	}

	branches, err := storyHandler.GetBranches(story)
	if err != nil {
		return nil, err
	}

	p, err := b.GetCurrentPlatform()
	if err != nil {
		return nil, err
	}

	qaStory := &QAStory{
		Bosun:    b,
		Platform: p,
		Story:    story,
		Handler:  storyHandler,
		Apps:     map[string]*QAApp{},
	}

	ctx := b.NewContext()

	for _, branch := range branches {

		app, appErr := b.GetAppFromRepo(branch.Repo.String())
		if appErr != nil {
			ctx.Log().WithError(appErr).Errorf("Couldn't find an app for repo %q", branch.Repo.String())
			continue
		}

		qaApp := &QAApp{
			Branch: branch,
			App:    app,
		}
		qaStory.Apps[app.Name] = qaApp
	}

	return qaStory, nil
}

const (
	argStoryQABranchesValidate = "validate"
)

var _ = addCommand(storyQACmd, storyQAShowBranchesCmd)
var _ = addCommand(storyDevCmd, storyQAShowBranchesCmd)

type QAStory struct {
	Bosun    *bosun.Bosun
	Platform *bosun.Platform
	Cluster  *kube.Cluster
	Stack    *kube.Stack
	Story    *stories.Story
	Handler  stories.StoryHandler
	Apps     map[string]*QAApp
}

type QAApp struct {
	Branch stories.BranchRef
	App    *bosun.App
}
