package bosun

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/util/multierr"
	"github.com/naveego/bosun/pkg/util/worker"
	"github.com/naveego/bosun/pkg/zenhub"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
)

const (
	SlotCurrent      = "current"
	SlotStable       = "stable"
	SlotUnstable     = "unstable"
	SlotPrevious     = "previous"
	PlanFileName     = "plan.yaml"
	ManifestFileName = "manifest.yaml"
)

var (
	UnstableVersion = semver.New("0.0.0-unstable")
	MaxVersion      = semver.Version{Major: math.MaxInt64}
)

// Platform is a collection of releasable apps which work together in a single cluster.
// The platform contains a history of all releases created for the platform.
type Platform struct {
	core.ConfigShared            `yaml:",inline"`
	DefaultChartRepo             string                      `yaml:"defaultChartRepo"`
	Branching                    git.BranchSpec              `yaml:"branching"`
	ReleaseBranchFormat_OBSOLETE string                      `yaml:"releaseBranchFormat,omitempty"`
	MasterBranch_OBSOLETE        string                      `yaml:"masterBranch,omitempty"`
	ReleaseDirectory             string                      `yaml:"releaseDirectory" json:"releaseDirectory"`
	ReleaseMetadata              []*ReleaseMetadata          `yaml:"releases" json:"releases"`
	Repos                        []*Repo                     `yaml:"repos" json:"repos"`
	Apps                         []*AppMetadata              `yaml:"apps"`
	ZenHubConfig                 *zenhub.Config              `yaml:"zenHubConfig"`
	NextReleaseName              string                      `yaml:"nextReleaseName,omitempty"`
	Clusters                     kube.ConfigDefinitions      `yaml:"clusters"`
	releaseManifests             map[string]*ReleaseManifest `yaml:"-"`
}

func (p *Platform) MarshalYAML() (interface{}, error) {
	if p == nil {
		return nil, nil
	}
	type proxy Platform
	px := proxy(*p)

	return &px, nil
}

func (p *Platform) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type proxy Platform
	var px proxy
	if p != nil {
		px = proxy(*p)
	}

	err := unmarshal(&px)

	if err == nil {
		*p = Platform(px)
	}

	p.Branching.Master = util.DefaultString(p.Branching.Master, p.MasterBranch_OBSOLETE, "master")
	p.Branching.Develop = util.DefaultString(p.Branching.Develop, "develop")
	p.Branching.Release = util.DefaultString(p.Branching.Release, p.ReleaseBranchFormat_OBSOLETE, "release/{{.Version}}")

	if p.releaseManifests == nil {
		p.releaseManifests = map[string]*ReleaseManifest{}
	}
	if p.ZenHubConfig == nil {
		p.ZenHubConfig = &zenhub.Config{
			StoryBoardName: "Stories",
			TaskBoardName:  "Tasks",
			StoryColumnMapping: issues.ColumnMapping{
				issues.ColumnInDevelopment: "In Development",
				issues.ColumnWaitingForUAT: "UAT",
				issues.ColumnDone:          "Done",
				issues.ColumnClosed:        "Closed",
			},
			TaskColumnMapping: issues.ColumnMapping{
				issues.ColumnInDevelopment:    "In Progress",
				issues.ColumnWaitingForMerge:  "Ready for Merge",
				issues.ColumnWaitingForDeploy: "Done",
				issues.ColumnClosed:           "Closed",
			},
		}
	}

	return err
}

func (p *Platform) GetCluster(name string) (*kube.ConfigDefinition, error) {
	return p.Clusters.GetKubeConfigDefinitionByName(name)
}

func (p *Platform) GetCurrentRelease() (*ReleaseManifest, error) {
	return p.GetReleaseManifestBySlot(SlotCurrent)
}

func (p *Platform) GetStableRelease() (*ReleaseManifest, error) {
	return p.GetReleaseManifestBySlot(SlotStable)
}

func (p *Platform) GetPreviousRelease() (*ReleaseManifest, error) {
	return p.GetReleaseManifestBySlot(SlotPrevious)
}

func (p *Platform) GetUnstableRelease() (*ReleaseManifest, error) {
	return p.GetReleaseManifestBySlot(SlotUnstable)
}

func (p *Platform) MustGetNextRelease() *ReleaseManifest {
	return p.MustGetReleaseManifestBySlot(SlotCurrent)
}

func (p *Platform) MustGetStableRelease() *ReleaseManifest {
	return p.MustGetReleaseManifestBySlot(SlotStable)
}

func (p *Platform) MustGetUnstableRelease() *ReleaseManifest {
	return p.MustGetReleaseManifestBySlot(SlotUnstable)
}

func (p *Platform) GetReleaseMetadataSortedByVersion(descending bool) []*ReleaseMetadata {
	out := make([]*ReleaseMetadata, len(p.ReleaseMetadata))
	copy(out, p.ReleaseMetadata)
	if descending {
		sort.Sort(sort.Reverse(releaseMetadataSorting(out)))
	} else {
		sort.Sort(releaseMetadataSorting(out))
	}

	return out
}

func (p *Platform) MakeReleaseBranchName(version semver.Version) string {
	if version == UnstableVersion {
		return p.Branching.Develop
	}
	name, _ := p.Branching.RenderRelease(git.BranchParts{git.BranchPartVersion: version.String()})
	return name
}

type ReleasePlanSettings struct {
	Name         string
	Version      semver.Version
	BranchParent string
	Bump         string
}

func (p *Platform) checkPlanningOngoing() error {
	if release, err := p.GetCurrentRelease(); err == nil {
		return errors.Errorf("currently editing plan for release %q, commit or discard the plan before starting a new one", release.String())
	}
	return nil
}

func (p *Platform) SwitchToReleaseBranch(ctx BosunContext, branch string) error {

	platformRepoPath, err := git.GetRepoPath(p.FromPath)
	if err != nil {
		return err
	}
	localRepo := &LocalRepo{Path: platformRepoPath}
	if localRepo.IsDirty() {
		return errors.New("repo is dirty, commit or stash your changes before adding it to the release")
	}

	log := ctx.Log()

	log.Debug("Checking if release branch exists...")

	branchExists, err := localRepo.DoesBranchExist(ctx, branch)
	if err != nil {
		return err
	}
	if branchExists {
		log.Info("Release branch already exists, switching to it.")
		err = localRepo.SwitchToBranchAndPull(ctx.Services(), branch)
		if err != nil {
			return errors.Wrap(err, "switching to release branch")
		}
	} else {
		log.Info("Creating release branch...")
		err = localRepo.SwitchToNewBranch(ctx, "master", branch)
		if err != nil {
			return errors.Wrap(err, "creating release branch")
		}
	}

	return nil

}

func (p *Platform) CreateReleasePlan(ctx BosunContext, settings ReleasePlanSettings) (*ReleasePlan, error) {
	if err := p.checkPlanningOngoing(); err != nil {
		return nil, err
	}
	ctx.Log().Debug("Creating new release plan.")

	metadata := &ReleaseMetadata{
		Version: settings.Version,
		Name:    settings.Name,
		Branch:  p.MakeReleaseBranchName(settings.Version),
	}

	if err := p.SwitchToReleaseBranch(ctx, metadata.Branch); err != nil {
		return nil, err
	}

	var err error
	if settings.Bump == "" && settings.Version.Empty() {
		return nil, errors.New("either version or bump must be provided")
	}
	if settings.Bump != "" {
		previousReleaseMetadata := p.MustGetStableRelease()
		settings.Version, err = previousReleaseMetadata.Version.Bump(settings.Bump)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	if settings.Name == "" {
		settings.Name = settings.Version.String()
	}

	if settings.BranchParent != "" {
		branchParentMetadata, err := p.GetReleaseMetadataByNameOrVersion(settings.BranchParent)
		if err != nil {
			return nil, errors.Wrapf(err, "getting patch parent")
		}
		metadata.PreferredParentBranch = branchParentMetadata.Branch
	}

	existing, _ := p.GetReleaseMetadataByNameOrVersion(metadata.Name)
	if existing == nil {
		existing, _ = p.GetReleaseMetadataByVersion(metadata.Version)
	}
	if existing != nil {
		return nil, errors.Errorf("release already exists with name %q and version %v", metadata.Name, metadata.Version)
	}

	manifest := &ReleaseManifest{
		ReleaseMetadata: metadata,
	}
	manifest.init()

	plan := NewReleasePlan(metadata)

	err = p.UpdatePlan(ctx, plan)
	if err != nil {
		return nil, err
	}

	ctx.Log().Infof("Created new release plan %s.", manifest)

	manifest.plan = plan
	p.NextReleaseName = settings.Name

	p.SetReleaseManifest(SlotCurrent, manifest)

	return plan, nil
}

// UpdatePlan updates the plan using the provided apps. If no apps are provided, all apps in the unstable release will be updated in the plan.
func (p *Platform) UpdatePlan(ctx BosunContext, plan *ReleasePlan, apps ...*App) error {

	workspaceAppProvider := ctx.Bosun.workspaceAppProvider

	unstableManifest, err := p.GetUnstableRelease()
	if err != nil {
		return errors.Wrap(err, "must have an unstable release to plan a release")
	}
	err = unstableManifest.RefreshApps(ctx, apps...)
	if err != nil {
		return err
	}

	currentManifest, err := p.GetCurrentRelease()
	if err != nil {
		currentManifest = &ReleaseManifest{
			ReleaseMetadata: plan.ReleaseMetadata,
			Slot:            SlotCurrent,
		}
	}
	err = currentManifest.RefreshApps(ctx, apps...)
	if err != nil {
		return err
	}

	stableManifest, err := p.GetStableRelease()
	if err != nil {
		stableManifest = &ReleaseManifest{
			Slot: SlotStable,
		}
	}

	unstableAppProvider := NewReleaseManifestAppProvider(unstableManifest)
	unstableApps, err := unstableAppProvider.GetAllApps()
	if err != nil {
		return errors.Wrap(err, "get unstable apps")
	}

	currentAppProvider := NewReleaseManifestAppProvider(currentManifest)
	currentApps, err := currentAppProvider.GetAllApps()
	if err != nil {
		return errors.Wrap(err, "get current apps")
	}

	stableAppProvider := NewReleaseManifestAppProvider(stableManifest)
	stableApps, err := stableAppProvider.GetAllApps()
	if err != nil {
		return errors.Wrap(err, "get stable apps")
	}

	if len(apps) == 0 {
		apps = AppList{}
		for _, app := range unstableApps {
			if app.HasChart() {
				apps = append(apps, app)
			}
		}
	}

	for _, app := range apps {

		appName := app.Name

		log := ctx.Log().WithField("app", app.Name)

		appPlan, ok := plan.Apps[appName]
		if !ok {
			appPlan = &AppPlan{
				Name:      appName,
				Providers: map[string]AppProviderPlan{},
			}
		}

		var stableVersion *App
		var unstableVersion *App
		var currentVersion *App
		var diffVersion *App
		var diffSlot string

		if stableVersion, ok = stableApps[appName]; ok {

			appPlan.Providers[SlotStable] = AppProviderPlan{
				Version:        stableVersion.Version.String(),
				Branch:         stableVersion.AppManifest.Branch,
				Commit:         stableVersion.AppManifest.Hashes.Commit,
				ReleaseVersion: stableVersion.GetMostRecentReleaseVersion(),
			}

			log.Infof("Found stable version of app (%s)", appPlan.Providers[SlotStable])
		}

		log.Info("Finding current version or unstable version for app...")

		if currentVersion, ok = currentApps[appName]; ok && currentVersion.AppManifest.PinnedReleaseVersion.EqualSafe(currentManifest.Version) {
			diffSlot = SlotCurrent
			diffVersion = currentVersion
			appPlan.Providers[SlotCurrent] = AppProviderPlan{
				Version: currentVersion.Version.String(),
				Branch:  currentVersion.AppManifest.Branch,
				Commit:  currentVersion.AppManifest.Hashes.Commit,
			}
			log.Infof("Found current version of app (%s)", appPlan.Providers[SlotCurrent])
		} else if unstableVersion, ok = unstableApps[appName]; ok {
			if stableVersion == nil || stableVersion.AppManifest.Hashes.Commit != unstableVersion.AppManifest.Hashes.Commit {
				// If the unstable version is different from the stable version, make it available as an option
				diffSlot = SlotUnstable
				diffVersion = unstableVersion
				appPlan.Providers[SlotUnstable] = AppProviderPlan{
					Version: unstableVersion.Version.String(),
					Branch:  unstableVersion.AppManifest.Branch,
					Commit:  unstableVersion.AppManifest.Hashes.Commit,
				}
				log.Infof("Found unstable version of app (%s)", appPlan.Providers[SlotUnstable])
			}
		} else {
			return errors.Errorf("mysterious app %q does not come from any release", appName)
		}

		if stableVersion != nil {

			localVersion, localVersionErr := workspaceAppProvider.GetApp(appName)
			if localVersionErr != nil {
				return errors.Wrapf(localVersionErr, "get local version of app %q", appName)
			}

			cloned := localVersion.Repo.CheckCloned() == nil
			owned := localVersion.BranchForRelease
			if cloned && owned && diffVersion != nil {
				diffProviderPlan := appPlan.Providers[diffSlot]
				log.Info("Computing change log...")

				developVersion, changeLogErr := localVersion.GetManifestFromBranch(ctx, diffVersion.AppManifest.Branch)
				if changeLogErr != nil {
					return changeLogErr
				}
				diffProviderPlan.Version = developVersion.Version.String()

				localRepo := localVersion.Repo.LocalRepo
				g, changeLogErr := localRepo.Git()
				if changeLogErr != nil {
					return errors.Wrapf(changeLogErr, "could not get most recent tag for %q", appName)
				}

				changeLog, changeLogErr := g.ChangeLog(diffVersion.AppManifest.Hashes.Commit, stableVersion.AppManifest.Hashes.Commit, nil, git.GitChangeLogOptions{})
				if changeLogErr != nil {
					return errors.Wrapf(changeLogErr, "could not get changelog for %q", appName)
				}

				if len(changeLog.Changes) > 0 {
					diffProviderPlan.Bump = changeLog.VersionBump
					for _, change := range changeLog.Changes.FilterByBump(semver.BumpMajor, semver.BumpMinor, semver.BumpPatch) {
						diffProviderPlan.Changelog = append(diffProviderPlan.Changelog, fmt.Sprintf("%s (%s) %s", change.Title, change.Committer, change.Issue))
					}
				}

				appPlan.Providers[diffSlot] = diffProviderPlan
			}
		}

		// If the app has no changes and the version hasn't been changed manually, we'll default to the stable version
		versions := map[string]bool{}
		changeCount := 0
		for _, appVersion := range appPlan.Providers {
			versions[appVersion.Version] = true
			changeCount += len(appVersion.Changelog)
		}
		if len(versions) == 1 && changeCount == 0 && appPlan.ChosenProvider == "" {
			for provider := range appPlan.Providers {
				appPlan.ChosenProvider = provider
			}
		}

		plan.Apps[appName] = appPlan
	}

	return nil
}

func (p *Platform) RePlanRelease(ctx BosunContext, apps ...*App) (*ReleasePlan, error) {
	current, err := p.GetCurrentRelease()
	if err != nil {
		return nil, err
	}

	plan, err := current.GetPlan()
	if err != nil {
		return nil, errors.Wrapf(err, "could not load release plan; if release is old, move release plan from manifest.yaml to a new plan.yaml file")
	}

	err = p.UpdatePlan(ctx, plan, apps...)
	if err != nil {
		return nil, err
	}

	current.plan = plan

	p.SetReleaseManifest(SlotCurrent, current)

	ctx.Log().Infof("Prepared new release plan for %s.", current)

	return plan, nil
}

type AppValidationResult struct {
	Message string
	Err     error
}

func (p *Platform) GetPlan(ctx BosunContext) (*ReleasePlan, error) {
	if p.NextReleaseName == "" {
		return nil, errors.New("no release being planned")
	}

	release, err := p.GetCurrentRelease()
	if err != nil {
		return nil, err
	}

	plan, err := release.GetPlan()
	return plan, err
}

func (p *Platform) ValidatePlan(ctx BosunContext) (map[string]AppValidationResult, error) {

	plan, err := p.GetPlan(ctx)
	if err != nil {
		return nil, err
	}

	results := map[string]AppValidationResult{}

	for appName, appPlan := range plan.Apps {

		r := AppValidationResult{}

		me := multierr.New()

		if appPlan.ChosenProvider == "" {
			me.Collect(errors.New("no provider chosen"))
		} else if _, ok := appPlan.Providers[appPlan.ChosenProvider]; !ok {
			me.Collect(errors.Errorf("invalid provider %q", appPlan.ChosenProvider))
		}

		r.Err = me.ToError()

		results[appName] = r
	}

	return results, nil
}

func (p *Platform) CommitPlan(ctx BosunContext) (*ReleaseManifest, error) {

	currentRelease, err := p.GetCurrentRelease()
	if err != nil {
		return nil, err
	}
	plan, err := currentRelease.GetPlan()
	if err != nil {
		return nil, err
	}

	stable, err := p.GetStableRelease()
	if err != nil {
		return nil, err
	}

	releaseMetadata := plan.ReleaseMetadata
	releaseManifest := NewReleaseManifest(releaseMetadata)

	releaseManifest.plan = plan

	validationResults, err := p.ValidatePlan(ctx)
	if err != nil {
		return nil, err
	}

	appErrs := multierr.New()
	for appName, validationResult := range validationResults {
		if validationResult.Err != nil {
			appErrs.Collect(fmt.Errorf("%s invalid: %s", appName, validationResult.Err))
		}
	}
	if err = appErrs.ToError(); err != nil {
		return nil, err
	}

	appCh := make(chan *AppManifest, len(plan.Apps))
	errCh := make(chan error)

	dispatcher := worker.NewDispatcher(ctx.Log(), 100)

	for _, unclosedAppName := range util.SortedKeys(plan.Apps) {
		appName := unclosedAppName
		originalApp, err := ctx.Bosun.GetApp(appName)
		if err != nil {
			return nil, errors.Wrapf(err, "app %q is not real", appName)
		}
		repo := originalApp.RepoName

		dispatcher.Dispatch(repo, func() {
			app, err := func() (*AppManifest, error) {

				var app *App
				appPlan := plan.Apps[appName]

				log := ctx.WithLogField("app", appName).Log()

				validationResult := validationResults[appName]
				if validationResult.Err != nil {
					return nil, errors.Errorf("app %q failed validation: %s", appName, validationResult.Err)
				}

				switch appPlan.ChosenProvider {
				case SlotStable:
					log.Infof("App %q will not be upgraded in this release; adding version last released in %q, with no deploy requested.", appName, appPlan.ChosenProvider)

					app, err = ctx.Bosun.GetAppFromProvider(appName, appPlan.ChosenProvider)
					return stable.PrepareAppManifest(ctx, app, semver.BumpNone, "")

				case SlotUnstable:
					// we pass the expected version here to avoid multiple bumps
					// if something goes wrong during branching

					log.Infof("App %q will be upgraded in this release; adding version from unstable release.", appName)

					providerPlan := appPlan.Providers[SlotUnstable]
					if err != nil {
						return nil, errors.Errorf("app does not exist")
					}
					bump := semver.BumpNone
					if providerPlan.Bump != "" {
						bump = providerPlan.Bump
					}
					if appPlan.BumpOverride != "" {
						bump = appPlan.BumpOverride
					}

					appManifest, err := currentRelease.PrepareAppManifest(ctx, originalApp, bump, "")
					return appManifest, err

				case SlotCurrent:
					log.Infof("App %q has already been added to this release.", appName)

					app, err = ctx.Bosun.GetAppFromProvider(appName, appPlan.ChosenProvider)
					return currentRelease.PrepareAppManifest(ctx, app, semver.BumpNone, "")
				default:
					return nil, errors.Errorf("invalid provider %q", appPlan.ChosenProvider)
				}
			}()

			if err != nil {
				errCh <- err
			} else {
				appCh <- app
			}
		})
	}

	for range plan.Apps {
		select {
		case appManifest := <-appCh:

			if appPlan, ok := plan.Apps[appManifest.Name]; ok {

				ctx.Log().WithField("app", appManifest.Name).WithField("version", appManifest.Version).Info("Adding app to release.")
				err = releaseManifest.AddApp(appManifest, appPlan.Deploy)
				if err != nil {
					return nil, err
				}
			} else {
				ctx.Log().WithField("app", appManifest.Name).Warn("Requested app was not in plan!")
			}
		case commitErr := <-errCh:
			return nil, commitErr
		}
	}

	p.SetReleaseManifest(SlotCurrent, releaseManifest)

	ctx.Log().Infof("Added release %q to releases for platform.", releaseManifest.Name)

	releaseManifest.MarkDirty()
	currentRelease.MarkDeleted()

	p.NextReleaseName = ""

	return releaseManifest, nil
}

// Save saves the platform. This will update the file containing the platform,
// and will write out any release manifests which have been loaded in this platform.
func (p *Platform) Save(ctx BosunContext) error {

	if ctx.GetParameters().DryRun {
		ctx.Log().Warn("Skipping platform save because dry run flag was set.")
	}

	ctx.Log().Info("Saving platform...")
	sort.Sort(sort.Reverse(releaseMetadataSorting(p.ReleaseMetadata)))

	manifests := p.releaseManifests

	// save the release manifests
	for slot, manifest := range manifests {
		if !manifest.dirty {
			ctx.Log().Debugf("Skipping save of manifest slot %q because it wasn't dirty.", slot)
			continue
		}
		ctx.Log().Infof("Saving manifest slot %q because it was dirty.", slot)
		manifest.Slot = slot
		dir := p.GetManifestDirectoryPath(slot)
		err := os.RemoveAll(dir)
		if err != nil {
			return err
		}

		if manifest.deleted {
			continue
		}

		err = os.MkdirAll(dir, 0700)
		if err != nil {
			return errors.Wrapf(err, "create directory for release %q", slot)
		}

		err = writeYaml(filepath.Join(dir, ManifestFileName), manifest)
		if err != nil {
			return err
		}

		if manifest.plan != nil {
			err = writeYaml(filepath.Join(dir, PlanFileName), manifest.plan)
			if err != nil {
				return err
			}
		}

		appManifests, err := manifest.GetAppManifests()
		if err != nil {
			return err
		}

		for _, appRelease := range appManifests {
			path := filepath.Join(dir, appRelease.Name+".yaml")
			err = writeYaml(path, appRelease)
			if err != nil {
				return errors.Wrapf(err, "write app %q", appRelease.Name)
			}
		}

		for _, toDelete := range manifest.toDelete {
			path := filepath.Join(dir, toDelete+".yaml")
			_ = os.Remove(path)
		}
	}

	err := p.File.Save()

	if err != nil {
		return err
	}

	ctx.Log().Info("Platform saved.")
	return nil
}

func writeYaml(path string, value interface{}) error {
	y, err := yaml.Marshal(value)
	if err != nil {
		return errors.Wrapf(err, "marshal value to be written to %q", path)
	}

	err = ioutil.WriteFile(path, y, 0600)
	return err
}

func (p *Platform) GetReleaseMetadataByNameOrVersion(name string) (*ReleaseMetadata, error) {
	switch name {
	case SlotCurrent, SlotUnstable, SlotStable:
		r, err := p.GetReleaseManifestBySlot(name)
		if err != nil {
			return nil, err
		}
		return r.ReleaseMetadata, nil
	}

	for _, rm := range p.ReleaseMetadata {
		if rm.Name == name {
			return rm, nil
		}
	}

	version, err := semver.NewVersion(name)
	if err != nil {
		return nil, errors.Errorf("this platform has no release named %q ", name)
	}

	return p.GetReleaseMetadataByVersion(version)
}

func (p *Platform) GetReleaseMetadataByVersion(v semver.Version) (*ReleaseMetadata, error) {
	for _, rm := range p.ReleaseMetadata {
		if rm.Version.Equal(v) {
			return rm, nil
		}
	}

	return nil, errors.Errorf("this platform has no release with version %q", v)
}

func (p *Platform) GetPreviousReleaseMetadata(version semver.Version) *ReleaseMetadata {

	for _, r := range p.GetReleaseMetadataSortedByVersion(true) {
		if r.Version.LessThan(version) {
			return r
		}
	}

	return nil
}

func (p *Platform) GetManifestDirectoryPath(name string) string {
	dir := filepath.Join(filepath.Dir(p.FromPath), p.ReleaseDirectory, name)
	return dir
}

func (p *Platform) MustGetReleaseManifestBySlot(name string) *ReleaseManifest {
	releaseMetadata, err := p.GetReleaseManifestBySlot(name)
	if err != nil {
		color.Red("Could not get release %q:\n%+v", err)
		os.Exit(1)
	}
	return releaseMetadata
}

func (p *Platform) GetReleaseManifestBySlot(slot string) (*ReleaseManifest, error) {
	if manifest, ok := p.releaseManifests[slot]; ok {
		return manifest, nil
	}

	dir := p.GetManifestDirectoryPath(slot)

	if _, err := os.Stat(dir); err != nil {
		if slot == SlotCurrent {
			return nil, errors.Wrap(err, "error getting directory, you may not be on a release branch")
		}
		return nil, err
	}

	manifestPath := filepath.Join(dir, "manifest.yaml")

	b, err := ioutil.ReadFile(manifestPath)
	if err != nil {
		return nil, errors.Wrapf(err, "read manifest for slot %q", slot)
	}

	var manifest *ReleaseManifest
	err = yaml.Unmarshal(b, &manifest)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal manifest for slot %q", slot)
	}

	manifest.dir = dir

	manifest.Platform = p

	if p.releaseManifests == nil {
		p.releaseManifests = map[string]*ReleaseManifest{}
	}
	p.releaseManifests[slot] = manifest
	manifest.Slot = slot
	return manifest, err
}

func (p *Platform) IncludeApp(ctx BosunContext, name string) error {

	app, err := ctx.Bosun.GetApp(name)
	if err != nil {
		return err
	}

	appManifest, err := app.GetManifest(ctx)
	if err != nil {
		return err
	}

	var found bool
	for _, knownApp := range p.Apps {
		if knownApp.Name == name {
			found = true
			break
		}
	}
	if !found {
		p.Apps = append(p.Apps, appManifest.AppMetadata)
	}

	manifest, err := p.GetUnstableRelease()
	if err != nil {
		return err
	}
	err = manifest.AddApp(appManifest, false)

	return err
}

// RefreshApp checks updates the specified slot with the specified branch of the named app.
func (p *Platform) RefreshApp(ctx BosunContext, name string, branch string, slot string) error {
	releaseManifest, err := p.GetReleaseManifestBySlot(slot)
	if err != nil {
		return err
	}

	return releaseManifest.RefreshApp(ctx, name, branch)

}

func (p *Platform) GetAppManifestByNameFromSlot(appName string, slot string) (*AppManifest, error) {

	release, err := p.GetReleaseManifestBySlot(slot)
	if err != nil {
		return nil, err
	}

	appManifest, err := release.GetAppManifest(appName)
	return appManifest, err
}

func (p *Platform) GetStableAppMetadata(name string) (*AppMetadata, error) {
	manifest, err := p.GetStableRelease()
	if err != nil {
		return nil, err
	}

	if appMetadata, ok := manifest.GetAllAppMetadata()[name]; ok {
		return appMetadata, nil
	}

	return nil, errors.Errorf("no app %q in stable release", name)
}

func (p *Platform) SetReleaseManifest(slot string, manifest *ReleaseManifest) {

	manifest.Slot = slot
	p.releaseManifests[slot] = manifest.MarkDirty()
	var updatedMetadata []*ReleaseMetadata
	replaced := false
	for _, metadata := range p.ReleaseMetadata {
		if metadata.Name == manifest.Name {
			updatedMetadata = append(updatedMetadata, manifest.ReleaseMetadata)
			replaced = true
		} else {
			updatedMetadata = append(updatedMetadata, metadata)
		}
	}
	if !replaced {
		updatedMetadata = append(updatedMetadata, manifest.ReleaseMetadata)
	}
	p.ReleaseMetadata = updatedMetadata
}

func (p *Platform) CommitCurrentRelease(ctx BosunContext) error {

	release, err := p.GetCurrentRelease()
	if err != nil {
		return err
	}

	platformDir, err := git.GetRepoPath(p.FromPath)
	if err != nil {
		return err
	}

	releaseBranch := fmt.Sprintf("release/%s", release.Version)

	progress := map[string]bool{}
	progressFileName := filepath.Join(os.TempDir(), fmt.Sprintf("bosun-release-commit-%s.yaml", release.Version))
	_ = util.LoadYaml(progressFileName, &progress)

	defer func() {
		_ = util.SaveYaml(progressFileName, progress)
	}()

	mergeTargets := map[string]*mergeTarget{
		"devops-develop": {
			dir:        platformDir,
			name:       "devops",
			version:    release.Version.String(),
			fromBranch: releaseBranch,
			toBranch:   "develop",
			tags: map[string]string{
				"":        release.Version.String(),
				"release": release.Name,
			},
		},
		"devops-master": {
			dir:        platformDir,
			name:       "devops",
			version:    release.Version.String(),
			fromBranch: releaseBranch,
			toBranch:   "master",
			tags: map[string]string{
				"":        release.Version.String(),
				"release": release.Name,
			},
		},
	}

	appsNames := map[string]bool{}
	for appName := range release.GetAllAppMetadata() {
		appsNames[appName] = true
	}

	b := ctx.Bosun

	for name := range release.UpgradedApps {
		log := ctx.Log().WithField("app", name)

		appDeploy, err := release.GetAppManifest(name)
		if err != nil {
			return err
		}

		app, err := b.GetAppFromProvider(name, WorkspaceProviderName)
		if err != nil {
			ctx.Log().WithError(err).Errorf("App repo %s (%s) not available.", appDeploy.Name, appDeploy.Repo)
			continue
		}

		if !app.BranchForRelease {
			ctx.Log().Warnf("App repo (%s) for app %s is not branched for release.", app.RepoName, app.Name)
			continue
		}

		// if appDeploy.PinnedReleaseVersion == nil {
		// 	ctx.Log().Warnf("App repo (%s) does not have a release branch pinned, probably not part of this release.", app.RepoName, release.Name, release.Version)
		// 	continue
		// }
		//
		// if *appDeploy.PinnedReleaseVersion != release.Version {
		// 	ctx.Log().Warnf("App repo (%s) is not changed for this release.", app.RepoName)
		// 	continue
		// }

		manifest, err := app.GetManifest(ctx)
		if err != nil {
			return errors.Wrapf(err, "App manifest %s (%s) not available.", appDeploy.Name, appDeploy.Repo)
		}

		if !app.IsRepoCloned() {
			return errors.Errorf("App repo (%s) for app %s is not cloned, cannot merge.", app.RepoName, app.Name)
		}

		appBranch, err := app.Branching.RenderRelease(release.GetBranchParts())
		if err != nil {
			return err
		}

		mt, ok := mergeTargets[app.Repo.Name]
		if !ok {
			masterName := app.Repo.Name
			if progress[masterName] {
				log.Infof("Release version has already been merged to master.")
			} else {
				mt = &mergeTarget{
					dir:        app.Repo.LocalRepo.Path,
					version:    manifest.Version.String(),
					name:       manifest.Name,
					fromBranch: appBranch,
					toBranch:   app.Branching.Master,
					tags:       map[string]string{},
				}
				mt.tags[app.RepoName] = fmt.Sprintf("%s@%s-%s", app.Name, manifest.Version.String(), release.Version.String())
				mergeTargets[masterName] = mt
			}

			if app.Branching.Develop != app.Branching.Master {
				developName := app.RepoName + "-develop"
				if progress[developName] {
					log.Info("Release version has already been merged to develop.")
				} else {

					mergeTargets[developName] = &mergeTarget{
						dir:        app.Repo.LocalRepo.Path,
						version:    manifest.Version.String(),
						name:       manifest.Name,
						fromBranch: appBranch,
						toBranch:   app.Branching.Develop,
						tags:       map[string]string{},
					}
				}
			}
		}
	}

	if len(mergeTargets) == 0 {
		return errors.New("no apps found")
	}

	fmt.Println("About to merge:")
	for label, target := range mergeTargets {
		fmt.Printf("- %s: %s@%s %s -> %s (tags %+v)\n", label, target.name, target.version, target.fromBranch, target.toBranch, target.tags)
	}

	warnings := multierr.New()

	errs := multierr.New()
	// validate that merge will work
	for _, target := range mergeTargets {

		localRepo := &LocalRepo{Name: target.name, Path: target.dir}

		if localRepo.IsDirty() {
			errs.Collect(errors.Errorf("Repo at %s is dirty, cannot merge.", localRepo.Path))
		}
	}

	if err = errs.ToError(); err != nil {
		return err
	}

	for targetLabel, target := range mergeTargets {

		log := ctx.Log().WithField("repo", target.name)

		localRepo := &LocalRepo{Name: target.name, Path: target.dir}

		if localRepo.IsDirty() {
			return errors.Errorf("Repo at %s is dirty, cannot merge.", localRepo.Path)
		}

		repoDir := localRepo.Path

		g, _ := git.NewGitWrapper(repoDir)

		err = g.Fetch()
		if err != nil {
			return err
		}

		log.Info("Checking out release branch...")

		_, err = g.Exec("checkout", target.fromBranch)
		if err != nil {
			return errors.Errorf("checkout %s: %s", repoDir, target.fromBranch)
		}

		log.Info("Pulling release branch...")
		err = g.Pull()
		if err != nil {
			return err
		}

		log.Infof("Checking out base branch %s...", target.toBranch)
		_, err = g.Exec("checkout", target.toBranch)
		if err != nil {
			return err
		}

		log.Infof("Pulling base branch %s...", target.toBranch)
		_, err = g.Exec("pull")
		if err != nil {
			return errors.Wrapf(err, "Could not pull branch, you'll need to resolve any merge conflicts.")
		}

		var tags []string
		for _, tag := range target.tags {
			tags = []string{tag}
		}

		changes, err := g.Exec("log", fmt.Sprintf("%s..%s", target.toBranch, target.fromBranch), "--oneline")
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			log.Infof("Branch %q has already been merged into %q.", target.fromBranch, target.toBranch)
		} else {
			tagged := false
			log.Info("Tagging release branch...")
			for _, tag := range tags {
				tagArgs := []string{"tag", tag, "-a", "-m", fmt.Sprintf("Release %s", release.Name)}
				tagArgs = append(tagArgs, "--force")
				_, err = g.Exec(tagArgs...)
				if err != nil {
					log.WithError(err).Warn("Could not tag repo, skipping merge. Set --force flag to force tag.")
				} else {
					tagged = true
				}
			}

			if tagged {
				log.Info("Pushing tags...")

				pushArgs := []string{"push", "--tags"}
				pushArgs = append(pushArgs, "--force")

				_, err = g.Exec(pushArgs...)
				if err != nil {
					return errors.Errorf("push tags: %s", err)
				}
			}

			log.Infof("Merging into branch %s...", target.toBranch)

			_, err = g.Exec("merge", "-m", fmt.Sprintf("Merge %s into %s to commit release %s", target.fromBranch, target.toBranch, release.Version), target.fromBranch)
			if err != nil {
				warnings.Collect(errors.Errorf("Merge for %s from %s to %s failed (you'll need to complete the merge yourself): %s", targetLabel, target.fromBranch, target.toBranch, err))
				continue
			}
		}

		changes, err = g.Exec("log", fmt.Sprintf("origin/%s..%s", target.toBranch, target.fromBranch), "--oneline")
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			log.Infof("Branch %s has already been pushed", target.toBranch)
			progress[targetLabel] = true
			continue
		}

		log.Infof("Pushing branch %s...", target.toBranch)

		_, err = g.Exec("push")
		if err != nil {
			warnings.Collect(errors.Errorf("Push for %s of branch %s failed (you'll need to push it yourself): %s", targetLabel, target.toBranch, err))
			continue
		}

		log.Infof("Merged back to %s and pushed.", target.toBranch)

		progress[targetLabel] = true
	}

	err = warnings.ToError()
	if err != nil {
		return warnings.ToError()
	}

	err = p.makeCurrentReleaseStable(ctx, p.Branching.Develop)
	if err != nil {
		return err
	}
	err = p.makeCurrentReleaseStable(ctx, p.Branching.Master)
	if err != nil {
		return err
	}

	return nil
}

func (p *Platform) makeCurrentReleaseStable(ctx BosunContext, branch string) error {

	log := ctx.WithLogField("branch", branch).Log()

	g, _ := git.NewGitWrapper(p.FromPath)
	err := g.CheckOutBranch(branch)
	if err != nil {
		return err
	}

	current, err := p.GetCurrentRelease()
	if err != nil {
		return err
	}
	stable, err := p.GetStableRelease()
	if err != nil {
		return err
	}

	currentDir := current.dir

	log.Info("Deleting stable release directory.")
	err = os.RemoveAll(stable.dir)
	if err != nil {
		return err
	}

	log.Info("Saving current release as stable release.")
	p.SetReleaseManifest(SlotStable, current)

	err = p.Save(ctx)
	if err != nil {
		return err
	}

	log.Info("Deleting old current release directory.")
	err = os.RemoveAll(currentDir)
	if err != nil {
		return err
	}

	log.Info("Current release has become the stable release.")

	message := fmt.Sprintf("Committing release %s to %s.", current.Version, branch)
	err = g.AddAndCommit(message, ".")
	if err != nil {
		return err
	}

	err = g.Push()
	if err != nil {
		return err
	}

	return nil
}

type mergeTarget struct {
	dir        string
	version    string
	name       string
	fromBranch string
	toBranch   string
	tags       map[string]string
}

type StatusDiff struct {
	From string
	To   string
}

func NewVersion(version string) (semver.Version, error) {
	v := semver.Version{}

	if err := v.Set(version); err != nil {
		return v, err
	}

	return v, nil
}

type AppHashes struct {
	Commit    string `yaml:"commit,omitempty"`
	Chart     string `yaml:"chart,omitempty"`
	AppConfig string `yaml:"appConfig,omitempty"`
}
