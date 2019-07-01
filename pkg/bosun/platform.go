package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/util/multierr"
	"github.com/naveego/bosun/pkg/zenhub"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	MasterName       = "master"
	PlanFileName     = "plan.yaml"
	ManifestFileName = "manifest.yaml"
)

var (
	MasterVersion = semver.New("0.0.0-master")
	MaxVersion    = semver.Version{Major: math.MaxInt64}
)

// Platform is a collection of releasable apps which work together in a single cluster.
// The platform contains a history of all releases created for the platform.
type Platform struct {
	ConfigShared        `yaml:",inline"`
	DefaultChartRepo    string                      `yaml:"defaultChartRepo"`
	ReleaseBranchFormat string                      `yaml:"releaseBranchFormat"`
	MasterBranch        string                      `yaml:"masterBranch"`
	ReleaseDirectory    string                      `yaml:"releaseDirectory" json:"releaseDirectory"`
	MasterMetadata      *ReleaseMetadata            `yaml:"master" json:"master"`
	MasterManifest      *ReleaseManifest            `yaml:"-" json:"-"`
	ReleaseMetadata     []*ReleaseMetadata          `yaml:"releases" json:"releases"`
	Repos               []*Repo                     `yaml:"repos" json:"repos"`
	Apps                []*AppMetadata              `yaml:"apps"`
	ReleaseManifests    map[string]*ReleaseManifest `yaml:"-" json:"-"`
	ZenHubConfig        *zenhub.Config              `yaml:"zenHubConfig"`
	PlanningReleaseName string                      `yaml:"planningReleaseName,omitempty"`

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
	if p.ReleaseManifests == nil {
		p.ReleaseManifests = map[string]*ReleaseManifest{}
	}
	if p.MasterMetadata != nil {
		p.MasterMetadata.Branch = p.MasterBranch
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

func (p *Platform) GetReleaseMetadataSortedByVersion(descending bool, includeLatest bool) []*ReleaseMetadata {
	out := make([]*ReleaseMetadata, len(p.ReleaseMetadata))
	copy(out, p.ReleaseMetadata)
	if descending {
		sort.Sort(sort.Reverse(releaseMetadataSorting(out)))
	} else {
		sort.Sort(releaseMetadataSorting(out))
	}

	if includeLatest {
		out = append(out, p.MasterMetadata)
	}

	return out
}

func (p *Platform) MakeReleaseBranchName(releaseName string) string {
	if releaseName == MasterName {
		return p.MasterBranch
	}
	return strings.Replace(p.ReleaseBranchFormat, "*", releaseName, 1)
}

type ReleasePlanSettings struct {
	Name         string
	Version      semver.Version
	BranchParent string
	Bump         string
}

func (p *Platform) checkPlanningOngoing() error {
	if p.PlanningReleaseName != "" {
		return errors.Errorf("currently editing plan for release %q, commit or discard the plan before starting a new one", p.PlanningReleaseName)
	}
	return nil
}

func (p *Platform) CreateReleasePlan(ctx BosunContext, settings ReleasePlanSettings) (*ReleasePlan, error) {
	if err := p.checkPlanningOngoing(); err != nil {
		return nil, err
	}
	ctx.Log.Debug("Creating new release plan.")

	var err error
	if settings.Bump == "" && settings.Version.Empty() {
		return nil, errors.New("either version or bump must be provided")
	}
	if settings.Bump != "" {
		previousReleaseMetadata := p.GetPreviousReleaseMetadata(MaxVersion)
		if previousReleaseMetadata == nil {
			previousReleaseMetadata = p.MasterMetadata
		}
		settings.Version, err = previousReleaseMetadata.Version.Bump(settings.Bump)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	if settings.Name == "" {
		settings.Name = settings.Version.String()
	}

	metadata := &ReleaseMetadata{
		Version: settings.Version,
		Name:    settings.Name,
		Branch:  p.MakeReleaseBranchName(settings.Version.String()),
	}

	if settings.BranchParent != "" {
		branchParentMetadata, err := p.GetReleaseMetadataByName(settings.BranchParent)
		if err != nil {
			return nil, errors.Wrapf(err, "getting patch parent")
		}
		metadata.PreferredParentBranch = branchParentMetadata.Branch
	}

	existing, _ := p.GetReleaseMetadataByName(metadata.Name)
	if existing == nil {
		existing, _ = p.GetReleaseMetadataByVersion(metadata.Version)
	}
	if existing != nil {
		return nil, errors.Errorf("release already exists with name %q and version %v", metadata.Name, metadata.Version)
	}

	metadata.Branch = p.ReleaseBranchFormat

	manifest := &ReleaseManifest{
		ReleaseMetadata: metadata,
	}
	manifest.init()

	masterManifest, err := p.GetMasterManifest()
	if err != nil {
		return nil, errors.Wrap(err, "get master manifest")
	}

	plan := NewReleasePlan(metadata)

	masterAppManifests, err := masterManifest.GetAppManifests()
	if err != nil {
		return nil, err
	}

	for appName, appManifest := range masterAppManifests {
		log := ctx.Log.WithField("app", appName)

		appPlan := &AppPlan{
			Name:                appName,
			Repo:                appManifest.Repo,
			PreviousReleaseName: MasterName,
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
	p.PlanningReleaseName = settings.Name

	p.AddReleaseManifest(manifest)

	return plan, nil
}

func (p *Platform) UpdateAppPlanWithChanges(ctx BosunContext, appPlan *AppPlan, app *App, branchParent string) error {
	if p.fetched == nil {
		p.fetched = map[string][]string{}
	}

	appName := appPlan.Name
	log := ctx.Log.WithField("app", appName)

	previousAppMetadata, err := p.GetMostRecentlyReleasedAppMetadata(appPlan.Name)

	if previousAppMetadata != nil {
		var previousReleaseBranch string

		appPlan.PreviousReleaseName = previousAppMetadata.PinnedReleaseVersion.String()
		// if we have a branch parent, and this app was released in it
		// then we should branch off that branch
		if branchParent != "" && previousAppMetadata.Branch == branchParent {
			appPlan.FromBranch = previousAppMetadata.Branch
		} else {
			appPlan.FromBranch = MasterName
		}

		previousReleaseBranch = previousAppMetadata.Branch

		appPlan.PreviousReleaseVersion = previousAppMetadata.Version.String()
		appPlan.CurrentVersionInMaster = app.Version.String()

		if app.BranchForRelease && app.IsRepoCloned() {
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

func (p *Platform) RePlanRelease(ctx BosunContext, metadata *ReleaseMetadata) (*ReleasePlan, error) {
	if err := p.checkPlanningOngoing(); err != nil {
		return nil, err
	}

	manifest, err := p.GetReleaseManifest(metadata)
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

	p.PlanningReleaseName = manifest.Name

	ctx.Log.Infof("Readied new release plan for %s.", manifest)

	return plan, nil
}

type AppValidationResult struct {
	Message string
	Err     error
}

func (p *Platform) GetPlan(ctx BosunContext) (*ReleasePlan, error) {
	if p.PlanningReleaseName == "" {
		return nil, errors.New("no release being planned")
	}

	release, err := p.GetReleaseManifestByName(p.PlanningReleaseName)
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

	plan, err := p.GetPlan(ctx)
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
			ctx.Log.Infof("No upgrade available for app %q; adding version from release %q, with no deploy requested.", appName, appPlan.PreviousReleaseName)
			var appManifest *AppManifest
			previousReleaseName := appPlan.PreviousReleaseName
			if previousReleaseName == "" {
				previousReleaseName = MasterName
			}

			appManifest, err = p.GetAppManifestFromRelease(previousReleaseName, appName)
			if err != nil {
				return nil, err
			}

			previousReleaseMetadata, _ := p.GetReleaseMetadataByName(previousReleaseName)
			if previousReleaseMetadata != nil {
				appManifest.PinToRelease(previousReleaseMetadata)
			}

			err = releaseManifest.AddApp(appManifest, appPlan.Deploy)
			if err != nil {
				return nil, err
			}
		}

	}

	p.AddReleaseManifest(releaseManifest)

	ctx.Log.Infof("Added release %q to releases for platform.", releaseManifest.Name)

	releaseManifest.MarkDirty()

	p.PlanningReleaseName = ""

	return releaseManifest, nil
}

// DeleteRelease deletes the release immediately, it calls save itself.
func (p *Platform) DeleteRelease(ctx BosunContext, name string) error {

	dir := p.GetManifestDirectoryPath(name)
	err := os.RemoveAll(dir)
	if err != nil {
		return err
	}

	delete(p.ReleaseManifests, name)

	var releaseMetadata []*ReleaseMetadata
	for _, rm := range p.ReleaseMetadata {
		if rm.Name != name {
			releaseMetadata = append(releaseMetadata, rm)
		}
	}
	p.ReleaseMetadata = releaseMetadata

	return p.Save(ctx)
}

// DiscardPlan discards the current plan; it calls save itself.
func (p *Platform) DiscardPlan(ctx BosunContext) error {
	if p.PlanningReleaseName != "" {

		ctx.Log.Warnf("Discarding plan for release %q.", p.PlanningReleaseName)
		p.PlanningReleaseName = ""
		return p.Save(ctx)
	}
	return nil
}

// Save saves the platform. This will update the file containing the platform,
// and will write out any release manifests which have been loaded in this platform.
func (p *Platform) Save(ctx BosunContext) error {

	if ctx.GetParams().DryRun {
		ctx.Log.Warn("Skipping platform save because dry run flag was set.")
	}

	ctx.Log.Info("Saving platform...")
	sort.Sort(sort.Reverse(releaseMetadataSorting(p.ReleaseMetadata)))

	manifests := p.ReleaseManifests
	if p.MasterManifest != nil {
		manifests[MasterName] = p.MasterManifest
	}

	// save the release manifests
	for _, manifest := range manifests {
		if !manifest.dirty {
			ctx.Log.Debugf("Skipping save of manifest %q because it wasn't dirty.", manifest.Name)
			continue
		}
		dir := p.GetManifestDirectoryPath(manifest.Name)
		err := os.MkdirAll(dir, 0700)
		if err != nil {
			return errors.Wrapf(err, "create directory for release %q", manifest.Name)
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

func (p *Platform) GetReleaseMetadataByName(name string) (*ReleaseMetadata, error) {
	if name == MasterName {
		return p.GetMasterMetadata(), nil
	}

	for _, rm := range p.ReleaseMetadata {
		if rm.Name == name {
			return rm, nil
		}
	}

	return nil, errors.Errorf("this platform has no release named %q ", name)
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

	for _, r := range p.GetReleaseMetadataSortedByVersion(true, false) {
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

func (p *Platform) GetReleaseManifestByName(name string) (*ReleaseManifest, error) {
	releaseMetadata, err := p.GetReleaseMetadataByName(name)
	if err != nil {
		return nil, err
	}
	releaseManifest, err := p.GetReleaseManifest(releaseMetadata)
	if err != nil {
		return nil, err
	}

	return releaseManifest, nil
}

func (p *Platform) GetReleaseManifest(metadata *ReleaseMetadata) (*ReleaseManifest, error) {
	dir := p.GetManifestDirectoryPath(metadata.Name)
	manifestPath := filepath.Join(dir, "manifest.yaml")

	b, err := ioutil.ReadFile(manifestPath)
	if err != nil {
		return nil, errors.Wrapf(err, "read manifest for %q", metadata.Name)
	}

	var manifest *ReleaseManifest
	err = yaml.Unmarshal(b, &manifest)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal manifest for %q", metadata.Name)
	}

	manifest.dir = dir

	manifest.Platform = p

	if p.ReleaseManifests == nil {
		p.ReleaseManifests = map[string]*ReleaseManifest{}
	}
	p.ReleaseManifests[metadata.Name] = manifest
	return manifest, err
}

func (p *Platform) GetMasterMetadata() *ReleaseMetadata {
	if p.MasterMetadata == nil {
		p.MasterMetadata = &ReleaseMetadata{
			Name: "latest",
		}
	}

	return p.MasterMetadata
}
func (p *Platform) GetMasterManifest() (*ReleaseManifest, error) {
	if p.MasterManifest != nil {
		return p.MasterManifest, nil
	}

	metadata := p.GetMasterMetadata()
	manifest, err := p.GetReleaseManifest(metadata)
	if err != nil {
		manifest = &ReleaseManifest{
			ReleaseMetadata: metadata,
		}
		manifest.init()
		p.MasterManifest = manifest
	}

	return manifest, nil
}

func (p *Platform) IncludeApp(ctx BosunContext, name string) error {
	manifest, err := p.GetMasterManifest()
	if err != nil {
		return err
	}

	app, err := ctx.Bosun.GetApp(name)
	if err != nil {
		return err
	}

	appManifest, err := app.GetManifest(ctx)
	if err != nil {
		return err
	}

	err = manifest.AddApp(appManifest, false)

	return err
}

// RefreshApp checks out the master branch of the app, then reloads it.
// If a release is being planned, the plan will be updated with the refreshed app.
func (p *Platform) RefreshApp(ctx BosunContext, name string) error {
	manifest, err := p.GetMasterManifest()
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

	currentRelease, err := b.GetCurrentReleaseManifest(true)
	if err != nil {
		ctx.Log.WithError(err).Warn("No current release to update.")
	} else {
		err = currentRelease.RefreshApp(ctx, name)
	}

	return nil
}

func (p *Platform) GetAppManifestFromRelease(releaseName string, appName string) (*AppManifest, error) {

	releaseManifest, err := p.GetReleaseManifestByName(releaseName)
	if err != nil {
		return nil, err
	}

	appManifests, err := releaseManifest.GetAppManifests()
	if err != nil {
		return nil, err
	}

	appManifest, ok := appManifests[appName]
	if !ok {
		return nil, errors.Errorf("release %q did not have a manifest for app %q", releaseName, appName)

	}
	return appManifest, nil
}

func (p *Platform) GetLatestAppManifestByName(appName string) (*AppManifest, error) {

	latestRelease, err := p.GetMasterManifest()
	if err != nil {
		return nil, err
	}

	appManifest, err := latestRelease.GetAppManifest(appName)
	return appManifest, err
}

func (p *Platform) GetLatestReleaseMetadata() (*ReleaseMetadata, error) {
	rm := p.GetReleaseMetadataSortedByVersion(true, true)
	if len(rm) == 0 {
		return nil, errors.New("no releases found")
	}

	return rm[0], nil
}

func (p *Platform) GetLatestReleaseManifest() (*ReleaseManifest, error) {
	latestReleaseMetadata, err := p.GetLatestReleaseMetadata()
	if err != nil {
		return nil, err
	}

	manifest, err := p.GetReleaseManifest(latestReleaseMetadata)
	return manifest, err
}

func (p *Platform) GetMostRecentlyReleasedAppMetadata(name string) (*AppMetadata, error) {
	releaseManifest, err := p.GetLatestReleaseManifest()
	if err != nil {
		return nil, err
	}

	appMetadata, ok := releaseManifest.AppMetadata[name]
	if !ok {
		return nil, errors.Errorf("no app %q in release %q", name, releaseManifest.Name)
	}

	return appMetadata, nil
}

func (p *Platform) AddReleaseManifest(manifest *ReleaseManifest) {
	p.ReleaseManifests[manifest.Name] = manifest.MarkDirty()
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
