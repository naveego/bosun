package bosun

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/util"
	yaml2 "github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"time"
)

type ReleaseCommitterPlan struct {
	ReleaseVersion semver.Version             `yaml:"releaseVersion"`
	Steps          []ReleaseCommitterPlanStep `yaml:"steps"`
}

type ReleaseCommitterPlanStep struct {
	CompletedAt time.Time                 `yaml:"completed,omitempty"`
	Error       string                    `yaml:"error,omitempty"`
	Repo        issues.RepoRef            `yaml:"repo"`
	RepoPath    string                    `yaml:"repoPath"`
	App         string                    `yaml:"app,omitempty"`
	Description string                    `yaml:"description"`
	Bump        *ReleaseCommitBumpAction  `yaml:"bump,omitempty"`
	Merge       *ReleaseCommitMergeAction `yaml:"merge,omitempty"`
	Tag         *ReleaseCommitTagAction   `yaml:"tag,omitempty"`
}

type ReleaseCommitBumpAction struct {
	Version semver.Version `yaml:"version,omitempty"`
	Branch  string         `yaml:"branch"`
}

type ReleaseCommitMergeAction struct {
	ToBranch   string `yaml:"targetBranch"`
	FromBranch string `yaml:"fromBranch"`
}

type ReleaseCommitTagAction struct {
	Branch string   `yaml:"branch"`
	Tags   []string `yaml:"tags"`
}

func (r ReleaseCommitterPlanStep) String() string {
	return r.Description
}

type ReleaseCommitter struct {
	release          *ReleaseManifest
	releaseBranch    string
	planPath         string
	plan             *ReleaseCommitterPlan
	log              *logrus.Entry
	platformRepo     issues.RepoRef
	platform         *Platform
	platformRepoPath string
	bosun            *Bosun
}

func NewReleaseCommitter(platform *Platform, b *Bosun) (*ReleaseCommitter, error) {

	log := b.NewContext().Log()

	release, err := platform.GetCurrentRelease()
	if err != nil {
		return nil, err
	}
	platformRepoPath, err := git.GetRepoPath(platform.FromPath)
	if err != nil {
		return nil, err
	}

	releaseBranch := fmt.Sprintf("release/%s", release.Version)

	progressFileName := filepath.Join(os.TempDir(), fmt.Sprintf("bosun-release-commit-plan-%s.yaml", release.Version))

	log.Infof("Storing plan at %s", progressFileName)

	org, repo := git.GetOrgAndRepoFromPath(platform.FromPath)

	platformRepo := issues.RepoRef{Org: org, Repo: repo}

	var plan ReleaseCommitterPlan

	_ = yaml2.LoadYaml(progressFileName, &plan)

	r := &ReleaseCommitter{
		bosun:            b,
		release:          release,
		platform:         platform,
		releaseBranch:    releaseBranch,
		planPath:         progressFileName,
		platformRepo:     platformRepo,
		platformRepoPath: platformRepoPath,
		plan:             &plan,
		log:              log,
	}

	return r, nil
}

func (r *ReleaseCommitter) updatePlan(mutator func(plan *ReleaseCommitterPlan)) error {
	mutator(r.plan)
	return yaml2.SaveYaml(r.planPath, r.plan)
}

func (r *ReleaseCommitter) updatePlanStep(index int, mutator func(plan *ReleaseCommitterPlanStep)) error {
	return r.updatePlan(func(plan *ReleaseCommitterPlan) {

		step := plan.Steps[index]
		mutator(&step)
		plan.Steps[index] = step

	})
}

func (r *ReleaseCommitter) addPlanSteps(steps ...ReleaseCommitterPlanStep) error {
	return r.updatePlan(func(plan *ReleaseCommitterPlan) {
		plan.Steps = append(plan.Steps, steps...)
	})
}

func (r *ReleaseCommitter) Plan() error {

	r.log.Info("Planning release commit")

	if err := r.updatePlan(func(plan *ReleaseCommitterPlan) {
		plan.ReleaseVersion = r.release.Version
		plan.Steps = []ReleaseCommitterPlanStep{}
	}); err != nil {
		return err
	}

	for _, appName := range util.SortedKeys(r.release.UpgradedApps) {

		log := r.log.WithField("app", appName)

		upgraded := r.release.UpgradedApps[appName]

		if !upgraded {
			log.Info("Skipping app because it wasn't marked as upgraded in the manifest.")
			continue
		}

		log.Info("Planning steps to commit app.")

		app, ok := r.release.appManifests[appName]
		if !ok {
			log.Warn("App was marked as upgraded but it was not found in the manifest.")
			continue
		}

		if app.RepoRef() == r.platformRepo {
			log.Infof("Skipping planning for app because it is in the platform repo and will commit with the platform.")
			continue
		}

		localApp, err := r.bosun.GetApp(appName, WorkspaceProviderName)
		if err != nil {
			log.WithError(err).Warnf("Skipping planning for app because it could not be found in local workspace.")
			continue
		}

		localAppRepoPath, err := git.GetRepoPath(localApp.FromPath)
		if err != nil {
			log.WithError(err).Warnf("Skipping planning for app because it did not have a local repo path.")
			continue
		}

		steps := []ReleaseCommitterPlanStep{
			{
				Repo:        app.RepoRef(),
				RepoPath:    localAppRepoPath,
				Description: "Tag release commits",
				Tag: &ReleaseCommitTagAction{
					Branch: r.releaseBranch,
					Tags: []string{
						fmt.Sprintf("v%s", app.Version),
						fmt.Sprintf("release-%s", r.release.Version),
					},
				},
			}, {
				Repo:        app.RepoRef(),
				Description: "Merge to develop",
				App:         appName,
				RepoPath:    localAppRepoPath,
				Merge: &ReleaseCommitMergeAction{
					FromBranch: r.releaseBranch,
					ToBranch:   app.AppConfig.Branching.Develop,
				},
			}, {
				Repo:        app.RepoRef(),
				Description: "Bump develop",
				App:         appName,
				RepoPath:    localAppRepoPath,
				Bump: &ReleaseCommitBumpAction{
					Version: app.Version.BumpPatch(),
					Branch:  app.AppConfig.Branching.Develop,
				},
			}, {
				Repo:        app.RepoRef(),
				RepoPath:    localAppRepoPath,
				Description: "Merge to master",
				App:         appName,
				Merge: &ReleaseCommitMergeAction{
					FromBranch: r.releaseBranch,
					ToBranch:   app.AppConfig.Branching.Master,
				},
			},
		}

		err = r.addPlanSteps(steps...)
		if err != nil {
			return err
		}
	}

	err := r.addPlanSteps(
		ReleaseCommitterPlanStep{

			Repo:        r.platformRepo,
			RepoPath:    r.platformRepoPath,
			Description: "Tag release commits",
			Tag: &ReleaseCommitTagAction{
				Branch: r.releaseBranch,
				Tags: []string{
					fmt.Sprintf("release-%s", r.release.Version),
				},
			},
		},

		ReleaseCommitterPlanStep{
			Repo:        r.platformRepo,
			RepoPath:    r.platformRepoPath,
			Description: "Merge to develop",
			Merge: &ReleaseCommitMergeAction{
				FromBranch: r.releaseBranch,
				ToBranch:   r.platform.Branching.Develop,
			},
		}, ReleaseCommitterPlanStep{
			Repo:        r.platformRepo,
			RepoPath:    r.platformRepoPath,
			Description: "Merge to master",
			Merge: &ReleaseCommitMergeAction{
				FromBranch: r.releaseBranch,
				ToBranch:   r.platform.Branching.Master,
			},
		})

	if err != nil {
		r.log.WithError(err).Error("Error adding master plan step.")
		return err
	}

	return nil
}

func (r *ReleaseCommitter) Execute() error {

	if len(r.plan.Steps) == 0 {
		return errors.New("no steps planned")
	}

	r.log.Infof("Executing %d steps", len(r.plan.Steps))

	for i, step := range r.plan.Steps {
		if !step.CompletedAt.IsZero() {
			r.log.Debugf("Skipping step %d (%s) because it is completed.", i, step)
			continue
		}

		for {
			err := r.ExecuteStep(i, step)
			if err != nil {

				color.Red("Step %d failed\n", i)
				fmt.Print("Step: ")
				color.Blue(step.String() + "\n")
				fmt.Println("Error: ")
				color.Red(err.Error())
				fmt.Println()
				fmt.Println("You can try to fix the issue in another terminal or you can abort.")
				confirmed := cli.RequestConfirmFromUser(" Enter 'y' when you have completed or 'n' to abort.", )
				if !confirmed {
					updateErr := r.updatePlanStep(i, func(step *ReleaseCommitterPlanStep) {
						step.Error = err.Error()
					})
					if updateErr != nil {
						return errors.Wrapf(updateErr, "error recording error on step %d %s; original error: %s", i, step, err)
					}
					return errors.Wrapf(err, "error on step %d %s", i, step)
				}
			} else {

				updateErr := r.updatePlanStep(i, func(step *ReleaseCommitterPlanStep) {
					step.CompletedAt = time.Now()
				})
				if updateErr != nil {
					return errors.Wrapf(updateErr, "error recording completion on step %d %s; original error: %s", i, step, err)
				}
				break
			}
		}
	}

	r.log.Infof("Completed %d steps", len(r.plan.Steps))

	return nil
}

func (r *ReleaseCommitter) ExecuteStep(i int, step ReleaseCommitterPlanStep) error {

	log := r.log.WithField("step", step.String()).WithField("index", i).WithField("repo", step.Repo)
	log.Info("Executing step.")

	g, err := getGitWrapper(step, log)
	if err != nil {
		return err
	}

	if g.IsDirty() {
		return errors.Errorf("repo %s is dirty, commit all changes before proceeding", step.RepoPath)
	}

	if step.Tag != nil {

		err = ensureBranch(g, step.Tag.Branch, log)
		if err != nil {
			return err
		}

		log.Infof("Applying tags %v", step.Tag.Tags)

		for _, tag := range step.Tag.Tags {
			_, err = g.Exec("tag", tag, "--force")
			if err != nil {
				return err
			}
		}

		_, err = g.Exec("push", "--force", "--tags")

		return err
	}

	if step.Bump != nil {

		log.Infof("Bumping app to version %s", step.Bump.Version)

		err = ensureBranch(g, step.Bump.Branch, log)
		if err != nil {
			return err
		}

		var app *App
		app, err = r.bosun.GetApp(step.App, WorkspaceProviderName)
		if err != nil {
			return err
		}

		if app.Version == step.Bump.Version {
			log.Infof("App is already on version %s", step.Bump.Version)
			return nil
		}

		err = app.BumpVersion(r.bosun.NewContext(), step.Bump.Version.String())

		if err != nil {
			return err
		}

		_, err = g.Exec("push", "--force")

		return err
	}

	if step.Merge != nil {

		log.Infof("Merging %s into %s", step.Merge.FromBranch, step.Merge.ToBranch)

		err = ensureBranch(g, step.Merge.FromBranch, log)
		if err != nil {
			return err
		}

		err = ensureBranch(g, step.Merge.ToBranch, log)
		if err != nil {
			return err
		}

		_, err = g.ExecVerbose("merge", "-m", fmt.Sprintf("Merge %s into %s to commit release %s", step.Merge.FromBranch, step.Merge.ToBranch, r.release.Version), step.Merge.FromBranch)
		for err != nil {

			confirmed := cli.RequestConfirmFromUser("Merge for %s from %s to %s in %s failed, you'll need to complete the merge yourself: %s\nEnter 'y' when you have completed the merge in another terminal, 'n' to abort release commit", r.release.Version, step.Merge.FromBranch, step.Merge.ToBranch, r.release.Version, step.RepoPath, err)
			if !confirmed {
				_, err = g.Exec("merge", "--abort")
				break
			}

			_, err = g.Exec("merge", "--continue")
		}

		err = g.Push()
		if err != nil {
			return err
		}

		return nil
	}

	return errors.Errorf("unknown action type")
}

func ensureBranch(g git.GitWrapper, branch string, log *logrus.Entry) error {
	log.Infof("Ensuring branch %q is present and up-to-date", branch)

	err := g.CheckOutOrCreateBranch(branch)
	if err != nil {
		return err
	}

	err = g.Pull()
	if err != nil {
		return err
	}
	return nil
}

func getGitWrapper(step ReleaseCommitterPlanStep, log *logrus.Entry) (git.GitWrapper, error) {

	if step.RepoPath == "" {
		return git.GitWrapper{}, errors.New("repo path was not set")
	}

	log.Infof("Switching git to use path %s", step.RepoPath)
	g, err := git.NewGitWrapper(step.RepoPath)

	if err != nil {
		return git.GitWrapper{}, err
	}
	err = g.Fetch()
	if err != nil {
		return git.GitWrapper{}, err
	}
	return g, err
}

func (r *ReleaseCommitter) GetPlan() (ReleaseCommitterPlan, error) {
	if len(r.plan.Steps) == 0 {
		return ReleaseCommitterPlan{}, errors.New("no steps planned")
	}

	return *r.plan, nil
}
