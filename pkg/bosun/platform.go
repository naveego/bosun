package bosun

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/util/multierr"
	"github.com/naveego/bosun/pkg/zenhub"
	"github.com/pkg/errors"
	"github.com/prometheus/common/log"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	NextName         = "next"
	StableName       = "stable"
	UnstableName     = "unstable"
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
	ConfigShared        `yaml:",inline"`
	DefaultChartRepo    string                      `yaml:"defaultChartRepo"`
	ReleaseBranchFormat string                      `yaml:"releaseBranchFormat"`
	MasterBranch        string                      `yaml:"masterBranch"`
	ReleaseDirectory    string                      `yaml:"releaseDirectory" json:"releaseDirectory"`
	ReleaseMetadata     []*ReleaseMetadata          `yaml:"releases" json:"releases"`
	Repos               []*Repo                     `yaml:"repos" json:"repos"`
	Apps                []*AppMetadata              `yaml:"apps"`
	ZenHubConfig        *zenhub.Config              `yaml:"zenHubConfig"`
	NextReleaseName     string                      `yaml:"nextReleaseName,omitempty"`
	releaseManifests    map[string]*ReleaseManifest `yaml:"-"`
	// cache of repos which have been fetched during this run
	fetched map[string][]string `yaml:"-"`
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

	if p.MasterBranch == "" {
		p.MasterBranch = "master"
	}
	if p.ReleaseBranchFormat == "" {
		p.ReleaseBranchFormat = "release/*"
	}
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

func (p *Platform) GetNextRelease() (*ReleaseManifest, error) {
	if p.NextReleaseName == "" {
		return nil, errors.New("no next release defined")
	}
	return p.GetReleaseManifestBySlot(NextName)
}

func (p *Platform) GetStableRelease() (*ReleaseManifest, error) {
	return p.GetReleaseManifestBySlot(StableName)
}

func (p *Platform) GetUnstableRelease() (*ReleaseManifest, error) {
	return p.GetReleaseManifestBySlot(UnstableName)
}

func (p *Platform) MustGetNextRelease() *ReleaseManifest {
	return p.MustGetReleaseManifestBySlot(NextName)
}

func (p *Platform) MustGetStableRelease() *ReleaseManifest {
	return p.MustGetReleaseManifestBySlot(StableName)
}

func (p *Platform) MustGetUnstableRelease() *ReleaseManifest {
	return p.MustGetReleaseManifestBySlot(UnstableName)
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
		return p.MasterBranch
	}
	return strings.Replace(p.ReleaseBranchFormat, "*", version.String(), 1)
}

type ReleasePlanSettings struct {
	Name         string
	Version      semver.Version
	BranchParent string
	Bump         string
}

func (p *Platform) checkPlanningOngoing() error {
	if p.NextReleaseName != "" {
		return errors.Errorf("currently editing plan for release %q, commit or discard the plan before starting a new one", p.NextReleaseName)
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

	log.Debug("Checking if release branch exists...")

	branchExists, err := localRepo.DoesBranchExist(ctx, branch)
	if err != nil {
		return err
	}
	if branchExists {
		log.Info("Release branch already exists, switching to it.")
		err = localRepo.SwitchToBranchAndPull(ctx, branch)
		if err != nil {
			return errors.Wrap(err, "switching to release branch")
		}
	} else {
		log.Info("Creating release branch...")
		// TODO: make from branch configurable
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
	ctx.Log.Debug("Creating new release plan.")

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

	for _, appManifest := range p.Apps {
		appName := appManifest.Name
		log := ctx.Log.WithField("app", appName)

		appPlan := &AppPlan{
			Name:                appName,
			Repo:                appManifest.Repo,
			PreviousReleaseName: UnstableName,
			FromBranch:          p.MasterBranch,
			ToBranch:            manifest.Branch,
		}
		app, err := ctx.Bosun.GetApp(appName)
		if err != nil {
			log.WithError(err).Warn("Could not get app.")
			continue
		}
		if !app.HasChart() {
			continue
		}

		err = p.UpdateAppPlanWithChanges(ctx, appPlan, app, settings.BranchParent)
		if err != nil {
			return nil, errors.Wrapf(err, "updated app %q with changes", app.Name)
		}

		plan.Apps[appName] = appPlan
	}

	ctx.Log.Infof("Created new release plan %s.", manifest)

	manifest.plan = plan
	p.NextReleaseName = settings.Name

	p.SetReleaseManifest(NextName, manifest)

	return plan, nil
}

func (p *Platform) UpdateAppPlanWithChanges(ctx BosunContext, appPlan *AppPlan, app *App, branchParent string) error {
	if p.fetched == nil {
		p.fetched = map[string][]string{}
	}

	appName := appPlan.Name
	log := ctx.Log.WithField("app", appName)

	previousAppMetadata, err := p.GetStableAppMetadata(appPlan.Name)

	if previousAppMetadata != nil {

		log.Debug("Comparing to previous release...")

		var previousReleaseBranch string

		appPlan.PreviousReleaseName = previousAppMetadata.PinnedReleaseVersion.String()
		// if we have a branch parent, and this app was released in it
		// then we should branch off that branch
		if branchParent != "" && previousAppMetadata.Branch == branchParent {
			appPlan.FromBranch = previousAppMetadata.Branch
		} else {
			appPlan.FromBranch = UnstableName
		}

		previousReleaseBranch = previousAppMetadata.Branch

		appPlan.PreviousReleaseVersion = previousAppMetadata.Version.String()
		appPlan.CurrentVersionInMaster = app.Version.String()

		if app.BranchForRelease && app.IsRepoCloned() {
			log.Debug("Finding changes from previous release...")

			var changes []string
			var ok bool
			if changes, ok = p.fetched[app.RepoName]; !ok {
				localRepo := app.Repo.LocalRepo
				log.Info("Fetching changes...")
				g := localRepo.git()

				err = g.Fetch()
				if err != nil {
					return errors.Wrapf(err, "fetching commits for %q", appName)
				} else {
					changes, err = g.ExecLines("log", "--left-right", "--cherry-pick", "--no-merges", "--oneline", "--no-color", fmt.Sprintf("%s...origin/%s", p.MasterBranch, previousReleaseBranch))
					if err != nil {
						return errors.Wrapf(err, "checking for changes for %q", appName)
					}
				}

				log.Infof("Fetched changes (found %d)", len(changes))

				p.fetched[app.RepoName] = changes
			}

			appPlan.CommitsNotInPreviousRelease = changes
		}
	}

	return nil
}

func (p *Platform) RePlanRelease(ctx BosunContext) (*ReleasePlan, error) {
	if err := p.checkPlanningOngoing(); err != nil {
		return nil, err
	}

	manifest, err := p.GetStableRelease()
	if err != nil {
		return nil, err
	}

	plan, err := manifest.GetPlan()
	if err != nil {
		return nil, errors.Wrapf(err, "could not load release plan; if release is old, move release plan from manifest.yaml to a new plan.yaml file")
	}

	for appName, appPlan := range plan.Apps {
		app, err := ctx.Bosun.GetApp(appName)
		if err != nil {
			return nil, errors.Wrapf(err, "getting app %q found in plan", appName)
		}
		if !app.HasChart() {
			continue
		}

		err = p.UpdateAppPlanWithChanges(ctx, appPlan, app, manifest.PreferredParentBranch)
		if err != nil {
			return nil, errors.Wrapf(err, "updated app %q with changes", app.Name)
		}
	}

	p.NextReleaseName = manifest.Name

	ctx.Log.Infof("Readied new release plan for %s.", manifest)

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

	release, err := p.GetNextRelease()
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

		if appPlan.Upgrade == true {
			if appPlan.FromBranch == "" {
				me.Collect(errors.Errorf("upgrade was true but fromBranch was empty"))
			}
			if appPlan.ToBranch == "" {
				me.Collect(errors.Errorf("upgrade was true but toBranch was empty"))
			}
			if appPlan.Bump == "" {
				me.Collect(errors.New("upgrade was true but bump was empty (should be 'none', 'patch', 'minor', or 'major')"))
			}
			appPlan.Deploy = true
		} else {
			if appPlan.Reason == "" {

				if len(appPlan.CommitsNotInPreviousRelease) > 0 {
					me.Collect(errors.Errorf("%d change commits detected: if not upgrading, you must provide a reason", len(appPlan.CommitsNotInPreviousRelease)))
				}

				if appPlan.CurrentVersionInMaster != appPlan.PreviousReleaseVersion {
					me.Collect(errors.Errorf("version changed from %q to %q: if not upgrading, you must provide a reason", appPlan.PreviousReleaseVersion, appPlan.CurrentVersionInMaster))
				}
			}
		}

		r.Err = me.ToError()

		results[appName] = r
	}

	return results, nil
}

func (p *Platform) CommitPlan(ctx BosunContext) (*ReleaseManifest, error) {

	nextRelease, err := p.GetNextRelease()
	if err != nil {
		return nil, err
	}
	plan, err := nextRelease.GetPlan()
	if err != nil {
		return nil, err
	}

	previousRelease, err := p.GetStableRelease()
	if err != nil {
		previousRelease, err = p.GetUnstableRelease()
		if err != nil {
			return nil, err
		}
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

	for _, appName := range util.SortedKeys(plan.Apps) {
		appPlan := plan.Apps[appName]

		validationResult := validationResults[appName]
		if validationResult.Err != nil {
			return nil, errors.Errorf("app %q failed validation: %s", appName, validationResult.Err)
		}

		if appPlan.Upgrade {
			// we pass the expected version here to avoid multiple bumps
			// if something goes wrong during branching
			expectedVersion := semver.New(appPlan.CurrentVersionInMaster)
			err = releaseManifest.UpgradeApp(ctx, appName, appPlan.FromBranch, appPlan.ToBranch, appPlan.Bump, expectedVersion)
			if err != nil {
				return nil, errors.Wrapf(err, "upgrading app %s", appName)
			}
		} else {
			ctx.Log.Infof("No upgrade available for app %q; adding version last released in %q, with no deploy requested.", appName, appPlan.PreviousReleaseName)
			var appManifest *AppManifest
			previousReleaseName := appPlan.PreviousReleaseName
			if previousReleaseName == "" {
				previousReleaseName = UnstableName
			}

			appManifest, err = previousRelease.GetAppManifest(appName)
			if err != nil {
				return nil, err
			}

			previousReleaseMetadata, _ := p.GetReleaseMetadataByNameOrVersion(previousReleaseName)
			if previousReleaseMetadata != nil {
				appManifest.PinToRelease(previousReleaseMetadata)
			}

			err = releaseManifest.AddApp(appManifest, appPlan.Deploy)
			if err != nil {
				return nil, err
			}
		}

	}

	p.SetReleaseManifest(StableName, releaseManifest)

	ctx.Log.Infof("Added release %q to releases for platform.", releaseManifest.Name)

	releaseManifest.MarkDirty()
	nextRelease.MarkDeleted()

	p.NextReleaseName = ""

	return releaseManifest, nil
}

// Save saves the platform. This will update the file containing the platform,
// and will write out any release manifests which have been loaded in this platform.
func (p *Platform) Save(ctx BosunContext) error {

	if ctx.GetParams().DryRun {
		ctx.Log.Warn("Skipping platform save because dry run flag was set.")
	}

	ctx.Log.Info("Saving platform...")
	sort.Sort(sort.Reverse(releaseMetadataSorting(p.ReleaseMetadata)))

	manifests := p.releaseManifests

	// save the release manifests
	for slot, manifest := range manifests {
		if !manifest.dirty {
			ctx.Log.Debugf("Skipping save of manifest slot %q because it wasn't dirty.", slot)
			continue
		}
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

	ctx.Log.Info("Platform saved.")
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
	if name == UnstableName {
		manifest, err := p.GetUnstableRelease()
		if err != nil {
			return nil, err
		}
		return manifest.ReleaseMetadata, nil
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
	dir := p.GetManifestDirectoryPath(slot)
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

// RefreshApp checks out the master branch of the app, then reloads it.
// If a release is being planned, the plan will be updated with the refreshed app.
func (p *Platform) RefreshApp(ctx BosunContext, name string, slot string) error {
	manifest, err := p.GetReleaseManifestBySlot(slot)
	if err != nil {
		return err
	}

	b := ctx.Bosun
	app, err := b.GetApp(name)
	if err != nil {
		return err
	}
	ctx = ctx.WithApp(app)

	currentAppManifest, err := manifest.GetAppManifest(name)
	if err != nil {
		return err
	}

	currentBranch := app.GetBranchName()

	if !currentBranch.IsMaster() {
		defer func() {
			e := app.CheckOutBranch(string(currentBranch))
			if e != nil {
				ctx.Log.WithError(e).Warnf("Returning to branch %q failed.", currentBranch)
			}
		}()
		err = app.CheckOutBranch(p.MasterBranch)
		if err != nil {
			return errors.Wrapf(err, "could not check out %q branch for app %q", p.MasterBranch, name)
		}
	}

	app, err = b.ReloadApp(name)
	if err != nil {
		return errors.Wrap(err, "reload app")
	}

	appManifest, err := app.GetManifest(ctx)
	if err != nil {
		return err
	}

	if appManifest.DiffersFrom(currentAppManifest.AppMetadata) {
		ctx.Log.Info("Updating manifest.")
		err = manifest.AddApp(appManifest, false)
		if err != nil {
			return err
		}
	} else {
		ctx.Log.Debug("No changes detected.")
	}

	err = manifest.RefreshApp(ctx, name)
	if err != nil {
		return err
	}

	manifest.MarkDirty()

	return nil
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

	p.releaseManifests[slot] = manifest.MarkDirty()
	var updatedMetadata []*ReleaseMetadata
	// replace metadata
	for _, metadata := range p.ReleaseMetadata {
		if metadata.Name == manifest.Name {
			updatedMetadata = append(updatedMetadata, manifest.ReleaseMetadata)
		} else {
			updatedMetadata = append(updatedMetadata, metadata)
		}
	}
	p.ReleaseMetadata = updatedMetadata
}

func (p *Platform) CommitStableRelease(ctx BosunContext) error {

	release, err := p.GetStableRelease()
	if err != nil {
		return err
	}

	platformDir, err := git.GetRepoPath(p.FromPath)
	if err != nil {
		return err
	}

	mergeTargets := map[string]mergeTarget{
		"devops": {
			dir:     platformDir,
			name:    "devops",
			version: release.Version.String(),
			tag:     release.Version.String(),
		},
	}

	releaseBranch := fmt.Sprintf("release/%s", release.Version)

	appsNames := map[string]bool{}
	for appName := range release.GetAllAppMetadata() {
		appsNames[appName] = true
	}

	b := ctx.Bosun

	for _, appDeploy := range release.AppMetadata {

		app, err := b.GetApp(appDeploy.Name)
		if err != nil {
			ctx.Log.WithError(err).Errorf("App repo %s (%s) not available.", appDeploy.Name, appDeploy.Repo)
			continue
		}

		if !app.BranchForRelease {
			ctx.Log.Warnf("App repo (%s) for app %s is not branched for release.", app.RepoName, app.Name)
			continue
		}

		if appDeploy.Branch != releaseBranch {
			ctx.Log.Warnf("App repo (%s) does not have a release branch for release %s (%s), nothing to merge.", app.RepoName, release.Name, release.Version)
			continue
		}

		manifest, err := app.GetManifest(ctx)
		if err != nil {
			ctx.Log.WithError(err).Errorf("App manifest %s (%s) not available.", appDeploy.Name, appDeploy.Repo)
			continue
		}

		if !app.IsRepoCloned() {
			ctx.Log.Errorf("App repo (%s) for app %s is not cloned, cannot merge.", app.RepoName, app.Name)
			continue
		}

		mergeTargets[app.Repo.LocalRepo.Path] = mergeTarget{
			dir:     app.Repo.LocalRepo.Path,
			version: manifest.Version.String(),
			name:    manifest.Name,
			tag:     fmt.Sprintf("%s-%s", manifest.Version.String(), release.Version.String()),
		}
	}

	if len(mergeTargets) == 0 {
		return errors.New("no apps found")
	}

	fmt.Println("About to merge back to master:")
	for _, target := range mergeTargets {
		fmt.Printf("- %s: %s (tag %s)\n", target.name, target.version, target.tag)
	}

	for _, target := range mergeTargets {

		log := ctx.Log.WithField("repo", target.name)

		localRepo := &LocalRepo{Name: target.name, Path: target.dir}

		if localRepo.IsDirty() {
			log.Errorf("Repo at %s is dirty, cannot merge.", localRepo.Path)
			continue
		}

		repoDir := localRepo.Path

		g, _ := git.NewGitWrapper(repoDir)

		err := g.FetchAll()
		if err != nil {
			return err
		}

		log.Info("Checking out release branch...")

		_, err = g.Exec("checkout", releaseBranch)
		if err != nil {
			return errors.Errorf("checkout %s: %s", repoDir, releaseBranch)
		}

		log.Info("Pulling release branch...")
		err = g.Pull()
		if err != nil {
			return err
		}

		log.Info("Tagging release branch...")
		tagArgs := []string{"tag", target.tag, "-a", "-m", fmt.Sprintf("Release %s", release.Name)}
		tagArgs = append(tagArgs, "--force")

		_, err = g.Exec(tagArgs...)
		if err != nil {
			log.WithError(err).Warn("Could not tag repo, skipping merge. Set --force flag to force tag.")
		} else {
			log.Info("Pushing tags...")

			pushArgs := []string{"push", "--tags"}
			pushArgs = append(pushArgs, "--force")

			_, err = g.Exec(pushArgs...)
			if err != nil {
				return errors.Errorf("push tags: %s", err)
			}
		}

		log.Info("Checking for changes...")

		diff, err := g.Exec("log", "origin/master..origin/"+releaseBranch, "--oneline")
		if err != nil {
			return errors.Errorf("find diffs: %s", err)
		}

		if len(diff) == 0 {
			log.Info("No diffs found between release branch and master...")
		} else {

			log.Info("Deploy branch has diverged from master, will merge back...")

			gitToken, err := b.GetGithubToken()
			if err != nil {
				return err
			}
			gitClient := git.NewGithubClient(gitToken)

			log.Info("Creating pull request.")
			_, prNumber, err := git.GitPullRequestCommand{
				LocalRepoPath: repoDir,
				Base:          "master",
				FromBranch:    releaseBranch,
				Client:        gitClient,
			}.Execute()
			if err != nil {
				ctx.Log.WithError(err).Error("Could not create pull request.")
				continue
			}

			issueService, err := b.GetIssueService(target.dir)
			if err != nil {
				return err
			}
			log.Info("Accepting pull request.")
			err = git.GitAcceptPRCommand{
				PRNumber:                 prNumber,
				RepoDirectory:            repoDir,
				DoNotMergeBaseIntoBranch: true,
				Client:                   gitClient,
				IssueService:             issueService,
			}.Execute()

			if err != nil {
				ctx.Log.WithError(err).Error("Could not accept pull request.")
				continue
			}

			log.Info("Merged back to master.")
		}

	}

	return nil
}

type mergeTarget struct {
	dir     string
	version string
	name    string
	tag     string
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
