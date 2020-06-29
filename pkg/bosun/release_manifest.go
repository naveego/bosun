package bosun

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/templating"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"io/ioutil"
	"path/filepath"
	"strings"
)

type ReleaseMetadata struct {
	Name                  string         `yaml:"name"`
	Version               semver.Version `yaml:"version"`
	Branch                string         `yaml:"branch"`
	PreferredParentBranch string         `yaml:"preferredParentBranch,omitempty"`
	Description           string         `yaml:"description"`
}

func (r ReleaseMetadata) String() string {
	if r.Name == r.Version.String() {
		return r.Name
	}
	return fmt.Sprintf("%s@%s", r.Name, r.Version)
}

func (r ReleaseMetadata) GetReleaseBranchName(branchSpec git.BranchSpec) (string, error) {
	return templating.RenderTemplate(branchSpec.Release, r)
}

type releaseMetadataSorting []*ReleaseMetadata

func (p releaseMetadataSorting) Len() int { return len(p) }

func (p releaseMetadataSorting) Less(i, j int) bool {
	return p[i].Version.LessThan(p[j].Version)
}

func (p releaseMetadataSorting) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

// ReleaseManifest represents a release for a platform.
// Instances should be manipulated using methods on the platform,
// not updated directly.
type ReleaseManifest struct {
	*ReleaseMetadata           `yaml:"metadata"`
	DefaultDeployApps_OBSOLETE map[string]bool            `yaml:"defaultDeployApps,omitempty"`
	UpgradedApps               map[string]bool            `yaml:"upgradedApps,omitempty"`
	AppMetadata                map[string]*AppMetadata    `yaml:"apps"`
	ValueSets                  *values.ValueSetCollection `yaml:"valueSets,omitempty"`
	Platform                   *Platform                  `yaml:"-"`
	plan                       *ReleasePlan               `yaml:"-"`
	toDelete                   []string                   `yaml:"-"`
	dirty                      bool                       `yaml:"-"`
	dir                        string                     `yaml:"-"`
	appManifests               map[string]*AppManifest    `yaml:"-" json:"-"`
	deleted                    bool                       `yaml:"-"`
	Slot                       string                     `yaml:"-"`
}

func (r *ReleaseManifest) MarshalYAML() (interface{}, error) {
	if r == nil {
		return nil, nil
	}
	type proxy ReleaseManifest
	p := proxy(*r)

	return &p, nil
}

func (r *ReleaseManifest) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type proxy ReleaseManifest
	var p proxy
	if r != nil {
		p = proxy(*r)
	}

	err := unmarshal(&p)

	if err == nil {
		*r = ReleaseManifest(p)
	}

	if r.DefaultDeployApps_OBSOLETE != nil && r.UpgradedApps == nil {
		r.UpgradedApps = r.DefaultDeployApps_OBSOLETE
		r.DefaultDeployApps_OBSOLETE = nil
	}

	r.init()

	return err
}

func (r *ReleaseMetadata) GetBranchParts() git.BranchParts {
	return git.BranchParts{
		git.BranchPartVersion: r.Version.String(),
		git.BranchPartName:    r.Name,
	}
}

func NewReleaseManifest(metadata *ReleaseMetadata) *ReleaseManifest {
	r := &ReleaseManifest{ReleaseMetadata: metadata}
	r.init()
	return r
}

func (r *ReleaseManifest) MarkDirty() *ReleaseManifest {
	r.dirty = true
	return r
}
func (r *ReleaseManifest) MarkDeleted() *ReleaseManifest {
	r.dirty = true
	r.deleted = true
	return r
}

func (r *ReleaseManifest) GetPlan() (*ReleasePlan, error) {
	if r.plan == nil {

		fromPath := filepath.Join(r.dir, PlanFileName)
		b, err := ioutil.ReadFile(fromPath)
		if err != nil {
			return nil, err
		}
		err = yaml.Unmarshal(b, &r.plan)
		if err != nil {
			return nil, err
		}
		r.plan.FromPath = fromPath
		return r.plan, err
	}
	return r.plan, nil
}

func (r *ReleaseManifest) GetAppManifests() (map[string]*AppManifest, error) {

	pkg.Log.Debugf("Getting app manifests...")

	if r.appManifests == nil {
		appManifests := map[string]*AppManifest{}

		allAppMetadata := r.GetAllAppMetadata()
		for appName, appMetadata := range allAppMetadata {
			appManifest, err := LoadAppManifestFromPathAndName(r.dir, appName)
			if err != nil {
				return nil, errors.Wrapf(err, "load app manifest for app  %q", appName)
			}

			appManifest.AppMetadata = appMetadata

			appManifests[appName] = appManifest
		}

		r.appManifests = appManifests
	}
	pkg.Log.Debugf("Got %d app manifests.", len(r.appManifests))
	return r.appManifests, nil
}

// init ensures the instance is ready to use.
func (r *ReleaseManifest) init() {
	if r.AppMetadata == nil {
		r.AppMetadata = map[string]*AppMetadata{}
	}
}

func (r *ReleaseManifest) Headers() []string {
	return []string{"Name", "Version", "From Release", "GetCurrentCommit Hash", "Deploying"}
}

func (r *ReleaseManifest) Rows() [][]string {
	var out [][]string
	for _, name := range util.SortedKeys(r.AppMetadata) {
		deploy := r.UpgradedApps[name]
		app := r.AppMetadata[name]
		fromReleaseText := ""
		if app.PinnedReleaseVersion != nil {
			fromReleaseText = app.PinnedReleaseVersion.String()
			if *app.PinnedReleaseVersion == r.Version {
				fromReleaseText = color.GreenString(fromReleaseText)
			}
		}

		deploying := ""
		if deploy {
			deploying = color.GreenString("YES")
		}
		out = append(out, []string{app.Name, app.Version.String(), fromReleaseText, app.Hashes.Commit, deploying})
	}
	return out
}

func (r *ReleaseManifest) GetAllAppMetadata() map[string]*AppMetadata {
	return r.AppMetadata
}

// BumpForRelease upgrades the named app by creating a release branch and bumping the version
// in that branch based on the bump parameter (if the app's current version is expectedVersion).
// If the bump parameter is "none" then the app won't be bumped.
func (r *ReleaseManifest) BumpForRelease(ctx BosunContext, app *App, fromBranch, toBranch string, bump semver.Bump, expectedVersion semver.Version) (*App, error) {
	r.init()
	r.MarkDirty()

	name := app.Name

	appConfig := app.AppConfig

	if appConfig.BranchForRelease {
		log := ctx.Log().WithField("app", appConfig.Name)
		if !app.IsRepoCloned() {
			return nil, errors.New("repo is not cloned but must be branched for release; what is going on?")
		}

		localRepo := app.Repo.LocalRepo
		if localRepo.IsDirty() {
			return nil, errors.Errorf("repo at %q is dirty, commit or stash your changes before adding it to the release", localRepo.Path)
		}

		log.Infof("Ensuring release branch and version correct for app %q...", name)

		branchExists, err := localRepo.DoesBranchExist(ctx, toBranch)
		if err != nil {
			return nil, err
		}
		if branchExists {
			log.Info("Release branch already exists, switching to it.")
			err = localRepo.SwitchToBranchAndPull(ctx.Services(), toBranch)
			if err != nil {
				return nil, errors.Wrap(err, "switching to release branch")
			}
		} else {
			log.Info("Creating release branch...")
			err = localRepo.SwitchToNewBranch(ctx, fromBranch, toBranch)
			if err != nil {
				return nil, errors.Wrap(err, "creating release branch")
			}
		}

		if bump != "none" {
			if expectedVersion.LessThan(app.Version) {
				log.Warnf("Skipping version bump %q because version on branch is already %s (source branch is version %s).", bump, app.Version, expectedVersion)
			} else {
				log.Infof("Applying version bump %q to source branch version %s.", bump, app.Version)

				err = app.BumpVersion(ctx, string(bump))
				if err != nil {
					return nil, errors.Wrap(err, "bumping version")
				}
			}
		}

		app.AddReleaseToHistory(r.Version.String())
		err = app.FileSaver.Save()
		if err != nil {
			return nil, errors.Wrap(err, "saving after adding release to app history")
		}

		err = app.Repo.LocalRepo.Commit("chore(release): add release to history", app.FromPath)
		if err != nil &&
			!strings.Contains(err.Error(), "no changes added to commit") &&
			!strings.Contains(err.Error(), "nothing to commit") {
			return nil, err
		}

		err = localRepo.Push()
		if err != nil {
			return nil, errors.Wrap(err, "pushing branch")
		}

		log.Info("App has been branched and bumped correctly.")

		app, err = ctx.Bosun.ReloadApp(app.Name)
		if err != nil {
			return nil, errors.Wrap(err, "reload app after switching to new branch")
		}
	}

	return app, nil
}

func (r *ReleaseManifest) RefreshApps(ctx BosunContext, apps ...*App) error {

	requestedApps := map[string]bool{}
	for _, app := range apps {
		requestedApps[app.Name] = true
	}

	switch r.Slot {
	case SlotStable:
		allAppManifests, err := r.GetAppManifests()
		if err != nil {
			return err
		}
		for _, app := range allAppManifests {
			if _, ok := requestedApps[app.Name]; ok || len(requestedApps) == 0 {
				err = r.RefreshApp(ctx, app.Name, app.AppConfig.Branching.Master)
				if err != nil {
					ctx.Log().WithError(err).Errorf("Unable to refresh %q", app.Name)
				}
			}
		}
	case SlotUnstable:
		allAppManifests, err := r.GetAppManifests()
		if err != nil {
			return err
		}
		for _, app := range allAppManifests {
			if _, ok := requestedApps[app.Name]; ok || len(requestedApps) == 0 {
				err = r.RefreshApp(ctx, app.Name, app.AppConfig.Branching.Develop)
				if err != nil {
					ctx.Log().WithError(err).Errorf("Unable to refresh %q", app.Name)
				}
			}
		}
	case SlotCurrent:

		allAppManifests, err := r.GetAppManifests()
		if err != nil {
			return err
		}
		for _, app := range allAppManifests {
			// only update if app was requested or no apps were requested
			if _, ok := requestedApps[app.Name]; ok || len(requestedApps) == 0 {
				// only update this app has a release branch:
				err = r.RefreshApp(ctx, app.Name, app.Branch)
				if err != nil {
					ctx.Log().WithError(err).Errorf("Unable to refresh %q", app.Name)
				}
			}
		}
	default:
		return errors.Errorf("unsupported slot %q", r.Slot)
	}
	return nil
}

func (r *ReleaseManifest) RefreshApp(ctx BosunContext, name string, branch string) error {

	b := ctx.Bosun
	app, err := b.workspaceAppProvider.GetApp(name)
	if err != nil {
		return errors.Wrapf(err, "get local version of app %s to refresh", name)
	}
	ctx = ctx.WithApp(app)

	currentAppManifest, err := r.GetAppManifest(name)
	if err != nil {
		ctx.Log().Warnf("Could not get previous manifest for %q from release %q: %s", r.String(), name, err)
	}

	if currentAppManifest != nil && !ctx.GetParameters().Force {
		latestCommitHash, err := app.GetMostRecentCommitFromBranch(ctx, branch)
		if err != nil {
			return err
		}
		if strings.HasPrefix(latestCommitHash, currentAppManifest.Hashes.Commit) {
			ctx.Log().Infof("No changes detected, keeping app at %s@%s (most recent commit to %s is %s), use --force to override.", currentAppManifest.Version, currentAppManifest.Hashes.Commit, branch, latestCommitHash)
			return nil
		}
	}

	updatedAppManifest, err := app.GetManifestFromBranch(ctx, branch, true)
	if err != nil {
		return errors.Wrapf(err, "get manifest for %q from branch %q", name, branch)
	}

	previousVersion := "unknown"
	previousCommit := "unknown"
	if currentAppManifest != nil {
		previousVersion = currentAppManifest.Version.String()
		previousCommit = currentAppManifest.Hashes.Commit
	}
	currentVersion := updatedAppManifest.Version.String()
	currentCommit := updatedAppManifest.Hashes.Commit

	ctx.Log().Infof("Changes detected, will update app in release manifest: %s@%s => %s@%s", previousVersion, previousCommit, currentVersion, currentCommit)

	err = r.AddApp(updatedAppManifest, false)

	if err != nil {
		return err
	}

	return nil
}

// SyncApp refreshes the app's manifest from the release branch of that app.
func (r *ReleaseManifest) SyncApp(ctx BosunContext, name string) error {
	r.MarkDirty()

	b := ctx.Bosun
	app, err := b.GetApp(name)
	if err != nil {
		return err
	}

	appManifest, err := app.GetManifest(ctx)
	if err != nil {
		return err
	}

	appManifests, err := r.GetAppManifests()
	if err != nil {
		return err
	}

	appManifests[appManifest.Name] = appManifest

	return nil
}

func (r *ReleaseManifest) ExportDiagram() (string, error) {
	appManifests, err := r.GetAppManifests()
	if err != nil {
		return "", err
	}

	export := `# dot -Tpng myfile.dot >myfile.png
digraph g {
  rankdir="LR";
  node[style="rounded",shape="box"]
  edge[splines="curved"]`
	for _, app := range appManifests {

		export += fmt.Sprintf("%q;\n", app.Name)
		for _, dep := range app.AppConfig.DependsOn {
			export += fmt.Sprintf("%q -> %q;\n", app.Name, dep.Name)
		}
	}

	export += "}"
	return export, nil
}

func (r *ReleaseManifest) RemoveApp(appName string) {
	r.MarkDirty()
	r.init()
	delete(r.AppMetadata, appName)
	delete(r.appManifests, appName)
	delete(r.UpgradedApps, appName)
	r.toDelete = append(r.toDelete, appName)
}

func (r *ReleaseManifest) AddApp(manifest *AppManifest, addToDefaultDeploys bool) error {
	r.init()
	r.MarkDirty()
	appManifests, err := r.GetAppManifests()
	if err != nil {
		return err
	}

	err = manifest.MakePortable()
	if err != nil {
		return err
	}

	appManifests[manifest.Name] = manifest

	r.AppMetadata[manifest.Name] = manifest.AppMetadata
	if addToDefaultDeploys {
		if r.UpgradedApps == nil {
			r.UpgradedApps = map[string]bool{}
		}
		r.UpgradedApps[manifest.Name] = true
	}
	return nil
}

func (r *ReleaseManifest) PrepareAppManifest(ctx BosunContext, app *App, bump semver.Bump, branch string) (*AppManifest, error) {

	switch r.Slot {
	case SlotStable:
		if branch == "" {
			branch = app.Branching.Master
		}
		return app.GetManifestFromBranch(ctx, branch, true)
	case SlotUnstable:
		if branch == "" {
			branch = app.Branching.Develop
		}
		return app.GetManifestFromBranch(ctx, app.Branching.Develop, true)
	case SlotCurrent:
		if branch == "" {
			branch = app.Branching.Develop
		}
		if app.BranchForRelease {

			developAppManifest, err := app.GetManifestFromBranch(ctx, branch, false)
			if err != nil {
				return nil, errors.Wrapf(err, "get manifest for %q from %q", app.Name, app.Branching.Develop)
			}

			releaseBranch, err := r.ReleaseMetadata.GetReleaseBranchName(app.Branching)
			if err != nil {
				return nil, errors.Wrap(err, "create release branch name")
			}

			bumpedApp, err := r.BumpForRelease(ctx, app, app.Branching.Develop, releaseBranch, bump, developAppManifest.Version)
			if err != nil {
				return nil, errors.Wrapf(err, "upgrading app %s", app.Name)
			}
			appManifest, err := bumpedApp.GetManifestFromBranch(ctx, releaseBranch, true)
			if err != nil {
				return nil, errors.Wrapf(err, "get latest version of manifest from app")
			}
			return appManifest, err
		} else {
			return app.GetManifestFromBranch(ctx, branch, true)
		}
	default:
		return nil, errors.Errorf("invalid slot %q", r.Slot)
	}

}

func (r *ReleaseManifest) GetAppManifest(name string) (*AppManifest, error) {
	appManifests, err := r.GetAppManifests()
	if err != nil {
		return nil, err
	}
	if a, ok := appManifests[name]; ok {
		return a, nil
	}

	return nil, errors.Errorf("no app manifest with name %q in release %q", name, r.Name)

}

func (r *ReleaseManifest) Clone() *ReleaseManifest {
	y, _ := yaml.Marshal(r)
	var out *ReleaseManifest
	_ = yaml.Unmarshal(y, &out)
	out.appManifests = map[string]*AppManifest{}

	appManifests, _ := r.GetAppManifests()

	for name, appManifest := range appManifests {
		y, _ = yaml.Marshal(appManifest)
		var appManifestClone *AppManifest
		_ = yaml.Unmarshal(y, &appManifestClone)
		out.appManifests[name] = appManifestClone
	}

	return out
}
