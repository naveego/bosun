package bosun

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/templating"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/util/worker"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
	"strings"
)

type ReleaseMetadata struct {
	Version     semver.Version `yaml:"version"`
	Branch      string         `yaml:"branch"`
	Description string         `yaml:"description"`
}

func (r ReleaseMetadata) String() string {
	return r.Version.String()
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
	AppMetadata                map[string]*AppMetadata    `yaml:"apps"`
	ValueSets                  *values.ValueSetCollection `yaml:"valueSets,omitempty"`
	Platform                   *Platform                  `yaml:"-"`
	toDelete                   []string                   `yaml:"-"`
	dirty                      bool                       `yaml:"-"`
	dir                        string                     `yaml:"-"`
	appManifests               map[string]*AppManifest    `yaml:"-" json:"-"`
	deleted                    bool                       `yaml:"-"`
	Slot                       string                     `yaml:"-"`
	repoRef                    issues.RepoRef             `yaml:"-"`
	isCurrentRelease           bool
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

	r.init()

	return err
}

func (r *ReleaseManifest) Save(ctx BosunContext) error {
	slot := r.Slot
	if slot != SlotStable && slot != SlotUnstable {
		ctx.Log().Infof("Skipping save of slot %q", slot)
		return nil
	}

	if !r.dirty {
		ctx.Log().Debugf("Skipping save of manifest slot %q because it wasn't dirty.", slot)
		return nil
	}
	ctx.Log().Infof("Saving manifest slot %q because it was dirty.", slot)
	r.Slot = slot
	dir := r.dir
	err := os.RemoveAll(dir)
	if err != nil {
		return err
	}

	if r.deleted {
		return nil
	}

	err = os.MkdirAll(dir, 0700)
	if err != nil {
		return errors.Wrapf(err, "create directory for release %q", slot)
	}

	err = writeYaml(filepath.Join(dir, ManifestFileName), r)
	if err != nil {
		return err
	}

	appManifests, err := r.GetAppManifests()
	if err != nil {
		return err
	}

	for _, appManifest := range appManifests {
		r.AppMetadata[appManifest.Name] = appManifest.AppMetadata

		appManifest.AppConfig.ProviderInfo = ""

		_, err = appManifest.Save(dir)
		if err != nil {
			return errors.Wrapf(err, "write app %q", appManifest.Name)
		}
	}

	for _, toDelete := range r.toDelete {
		_ = os.Remove(filepath.Join(dir, toDelete+".yaml"))
		_ = os.RemoveAll(filepath.Join(dir, toDelete))
	}

	return nil
}

func (r *ReleaseMetadata) GetBranchParts() git.BranchParts {
	return git.BranchParts{
		git.BranchPartVersion: r.Version.String(),
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

func (r *ReleaseManifest) GetAppManifests() (map[string]*AppManifest, error) {

	if r.appManifests == nil {
		core.Log.Debugf("Getting app manifests...")
		appManifests := map[string]*AppManifest{}

		allAppMetadata := r.GetAllAppMetadata()
		for appName := range allAppMetadata {
			appManifest, err := LoadAppManifestFromPathAndName(r.dir, appName)
			if err != nil {
				return nil, errors.Wrapf(err, "load app manifest for app  %q", appName)
			}

			// appManifest.AppMetadata = appMetadata

			appManifests[appName] = appManifest
			r.setReleaseBasedFields(appManifest.AppConfig)
		}

		r.appManifests = appManifests
		core.Log.Debugf("Got %d app manifests.", len(r.appManifests))
	}
	return r.appManifests, nil
}

// init ensures the instance is ready to use.
func (r *ReleaseManifest) init() {
	if r.AppMetadata == nil {
		r.AppMetadata = map[string]*AppMetadata{}
	}
}

func (r *ReleaseManifest) Headers() []string {
	return []string{"Name", "Version", "Deploying", "Parent Release", "Commit Hash"}
}

func (r *ReleaseManifest) Rows() [][]string {
	var out [][]string
	for _, name := range util.SortedKeys(r.appManifests) {
		deploy := r.isAppPinnedToThisRelease(name)
		app := r.appManifests[name]

		parentRelease := ""
		if app.PinnedReleaseVersion == nil {
			parentRelease = "Unknown"
		} else if app.PinnedReleaseVersion.Equal(r.Version) {
			parentRelease = color.GreenString("%s *", r.Version)
		} else {
			parentRelease = app.PinnedReleaseVersion.String()
		}

		deploying := ""
		if deploy {
			deploying = color.GreenString("YES")
		}

		out = append(out, []string{app.Name, app.Version.String(), deploying, parentRelease, app.Hashes.Commit})
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

	var err error
	name := app.Name

	appConfig := app.AppConfig

	if appConfig.BranchForRelease {
		log := ctx.Log().WithField("app", appConfig.Name)
		if !app.IsRepoCloned() {

			app, err = ctx.Bosun.workspaceAppProvider.ProvideApp(AppProviderRequest{Name: name, Branch: fromBranch})
			if err != nil {
				return nil, errors.New("app to bump %q could not be acquired from workspace provider")
			}
		}

		localRepo := app.Repo.LocalRepo
		if localRepo.IsDirty() {
			return nil, errors.Errorf("repo at %q is dirty, commit or stash your changes before adding it to the release", localRepo.Path)
		}

		log.Infof("Ensuring release branch and version correct for app %q...", name)

		branchExists, branchingErr := localRepo.DoesBranchExist(ctx, toBranch)
		if branchingErr != nil {
			return nil, branchingErr
		}
		if branchExists {
			log.Info("Release branch already exists, switching to it.")
			branchingErr = localRepo.SwitchToBranchAndPull(ctx.Services(), toBranch)
			if branchingErr != nil {
				return nil, errors.Wrap(branchingErr, "switching to release branch")
			}
		} else {
			log.Info("Creating release branch...")
			branchingErr = localRepo.SwitchToNewBranch(ctx, fromBranch, toBranch)
			if branchingErr != nil {
				return nil, errors.Wrap(branchingErr, "creating release branch")
			}
		}

		if bump != "none" {
			if expectedVersion.LessThan(app.Version) {
				log.Warnf("Skipping version bump %q because version on branch is already %s (source branch is version %s).", bump, app.Version, expectedVersion)
			} else {
				log.Infof("Applying version bump %q to source branch version %s.", bump, app.Version)

				branchingErr = app.BumpVersion(ctx, string(bump))
				if branchingErr != nil {
					return nil, errors.Wrap(branchingErr, "bumping version")
				}
			}
		}

		app.AddReleaseToHistory(r.Version.String())
		branchingErr = app.FileSaver.Save()
		if branchingErr != nil {
			return nil, errors.Wrap(branchingErr, "saving after adding release to app history")
		}

		branchingErr = app.Repo.LocalRepo.Commit("chore(release): add release to history", app.FromPath)
		if branchingErr != nil &&
			!strings.Contains(branchingErr.Error(), "no changes added to commit") &&
			!strings.Contains(branchingErr.Error(), "nothing to commit") {
			return nil, branchingErr
		}

		branchingErr = localRepo.Push()
		if branchingErr != nil {
			return nil, errors.Wrap(branchingErr, "pushing branch")
		}

		log.Info("App has been branched and bumped correctly.")

		app, branchingErr = ctx.Bosun.ReloadApp(app.Name)
		if branchingErr != nil {
			return nil, errors.Wrap(branchingErr, "reload app after switching to new branch")
		}
	}

	return app, nil
}

func (r *ReleaseManifest) IsMutable() error {
	switch r.Slot {
	case SlotPrevious:
		return errors.New("cannot modify a previous release")

	case SlotStable:
		if !r.isCurrentRelease {
			return errors.New("you can only modify the stable release when you are on a release branch")
		}
	}
	return nil
}

func (r *ReleaseManifest) RefreshApps(ctx BosunContext, apps ...*App) error {

	err := r.IsMutable()
	if err != nil {
		return err
	}

	requestedApps := map[string]*App{}

	for _, app := range apps {
		requestedApps[app.Name] = app
	}

	allAppManifests, err := r.GetAppManifests()
	if err != nil {
		return err
	}
	queue := worker.NewKeyedWorkQueue(ctx.Log(), 10)

	if len(requestedApps) == 0 {
		for appName := range allAppManifests {
			wsApp, wsErr := ctx.Bosun.GetAppFromWorkspace(appName)
			if wsErr != nil {
				ctx.Log().WithError(wsErr).Warnf("Could not get app %q from workspace, it will not be refreshed", appName)
			} else {
				requestedApps[appName] = wsApp
			}
		}
	}

	switch r.Slot {
	case SlotUnstable:

		for k := range allAppManifests {
			app := allAppManifests[k]
			log := ctx.Log().WithField("app", app.Name)

			if _, ok := requestedApps[app.Name]; ok || len(requestedApps) == 0 {
				queue.Dispatch(app.Repo, func() {
					err = r.RefreshApp(ctx, app.Name, app.Branch)
					if err != nil {
						log.WithError(err).Errorf("Unable to refresh %q", app.Name)
					}
				})
			}
		}
		queue.Wait()

	case SlotStable:

		for appName, app := range requestedApps {

			currentAppManifest, isInRelease := allAppManifests[appName]
			if !isInRelease {
				return errors.Errorf("app %q is not known in the stable release; use `bosun release add %s` to add it", appName, appName)
			}

			appBranch, appErr := r.GetReleaseBranchName(app.Branching.WithDefaultsFrom(ctx.GetPlatform().Branching))
			if appErr != nil {
				return errors.Wrapf(appErr, "determine release branch name for app %q", app.Name)
			}

			if currentAppManifest.Branch != appBranch {
				return errors.Errorf("app %q has not been added to the stable release (release is using version %s from branch %s and release %s); use `bosun release add stable %s` to add it", appName,
					currentAppManifest.Version,
					currentAppManifest.Branch,
					currentAppManifest.PinnedReleaseVersion,
					appName)
			}

			app.branch = appBranch
		}

		for _, app := range requestedApps {

			// app := allAppManifests[appName]
			log := ctx.Log().WithField("app", app.Name)

			var appErr error

			log.Infof("Refreshing on stable slot from branch %q", app.branch)

			queue.Dispatch(app.Repo.Name, func() {

				appErr = r.RefreshApp(ctx, app.Name, app.branch)
				if appErr != nil {
					log.WithError(appErr).Errorf("Unable to refresh %q", app.Name)
				}
			})

		}
		queue.Wait()

	default:
		return errors.Errorf("unsupported slot %q", r.Slot)
	}
	return nil
}

func (r *ReleaseManifest) RefreshApp(ctx BosunContext, name string, branch string) error {

	if err := r.IsMutable(); err != nil {
		return err
	}

	b := ctx.Bosun
	app, err := b.workspaceAppProvider.ProvideApp(AppProviderRequest{Name: name, Branch: branch})

	if err != nil {
		return errors.Wrapf(err, "get local version of app %s to refresh", name)
	}
	ctx = ctx.WithApp(app)

	currentAppManifest, err := r.GetAppManifest(name)
	if err != nil {
		ctx.Log().Warnf("Could not get current manifest for %q from release %q: %s", r.String(), name, err)
	}

	updatedAppManifest, err := app.GetManifestFromBranch(ctx, branch, true)
	if err != nil {
		return errors.Wrapf(err, "get manifest for %q from branch %q", name, branch)
	}

	hasChanges := currentAppManifest == nil
	changeDetails := "new app"
	if currentAppManifest != nil {
		if currentAppManifest.Version.String() != updatedAppManifest.Version.String() {
			hasChanges = true
			changeDetails = fmt.Sprintf("version changed: %s => %s", currentAppManifest.Version, updatedAppManifest.Version)
		} else {
			changeDetails, hasChanges = currentAppManifest.Hashes.Changes(updatedAppManifest.Hashes)
		}
	}

	if !hasChanges {
		if !ctx.GetParameters().Force {
			ctx.Log().Infof("No changes detected in branch %q, keeping app at %s (use --force to override).", branch, currentAppManifest.Version)
			return nil
		} else {
			ctx.Log().Infof("No changes detected, but will update app in release manifest because of --force flag")
		}
	} else {
		ctx.Log().Infof("Changes detected, will update app in release manifest: %s", changeDetails)
	}

	err = r.AddOrReplaceApp(updatedAppManifest, false)

	if err != nil {
		return err
	}

	return nil
}

// SyncApp refreshes the app's manifest from the release branch of that app.
func (r *ReleaseManifest) SyncApp(ctx BosunContext, name string) error {
	if err := r.IsMutable(); err != nil {
		return err
	}

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
	r.toDelete = append(r.toDelete, appName)
}

func (r *ReleaseManifest) AddOrReplaceApp(manifest *AppManifest, addToDefaultDeploys bool) error {
	if err := r.IsMutable(); err != nil {
		return err
	}

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

	err = r.updateAppHashes(manifest)
	if err != nil {
		return err
	}

	manifest.PinToRelease(r.ReleaseMetadata)

	appManifests[manifest.Name] = manifest

	r.AppMetadata[manifest.Name] = manifest.AppMetadata
	return nil
}

// Updates app manifest hashes to include those which are relevant within the release.
func (r *ReleaseManifest) updateAppHashes(manifest *AppManifest) error {
	err := manifest.UpdateHashes()
	if err != nil {
		return err
	}
	if manifest.RepoRef() == r.repoRef {
		// Ignore commit hash, because it's part of the release directory
		manifest.Hashes.Commit = ""
	}
	return nil
}

func (r *ReleaseManifest) PrepareAppForRelease(ctx BosunContext, app *App, bump semver.Bump, branch string) (*AppManifest, error) {

	if branch == "" {
		branch = app.AppConfig.Branching.Develop
	}

	if r.isCurrentRelease {
		releaseBranch, err := r.ReleaseMetadata.GetReleaseBranchName(app.AppConfig.Branching.WithDefaultsFrom(ctx.GetPlatform().Branching))
		if err != nil {
			return nil, errors.Wrap(err, "create release branch name")
		}

		bumpedApp, err := r.BumpForRelease(ctx, app, branch, releaseBranch, bump, app.Version)
		if err != nil {
			return nil, errors.Wrapf(err, "upgrading app %s", app.Name)
		}
		appManifest, err := bumpedApp.GetManifestFromBranch(ctx, releaseBranch, true)
		if err != nil {
			return nil, errors.Wrapf(err, "get latest version of manifest from app")
		}
		return appManifest, err
	} else if r.Slot == SlotUnstable {

		appManifest, err := app.GetManifestFromBranch(ctx, branch, true)
		if err != nil {
			return nil, errors.Wrapf(err, "get latest version of manifest from app")
		}

		return appManifest, nil
	} else {
		return nil, errors.New("you may only prepare apps for adding to unstable or on a release branch")
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

	return nil, errors.Errorf("no app manifest with name %q in release %q", name, r)

}

func (r *ReleaseManifest) TryGetAppManifest(name string) (*AppManifest, bool, error) {
	appManifests, err := r.GetAppManifests()
	if err != nil {
		return nil, false, err
	}

	a, ok := appManifests[name]

	return a, ok, nil

}

func (r *ReleaseManifest) Clone() *ReleaseManifest {
	y, _ := yaml.Marshal(r)
	var out *ReleaseManifest
	_ = yaml.Unmarshal(y, &out)
	out.appManifests = map[string]*AppManifest{}

	appManifests, err := r.GetAppManifests()
	if err != nil {
		panic(err)
	}

	for name, appManifest := range appManifests {
		y, _ = yaml.Marshal(appManifest)
		var appManifestClone *AppManifest
		_ = yaml.Unmarshal(y, &appManifestClone)
		out.appManifests[name] = appManifestClone
	}

	return out
}

func (r *ReleaseManifest) GetChangeDetectionHash() (string, error) {
	apps, err := r.GetAppManifests()
	if err != nil {
		return "", err
	}

	releaseHash, err := util.HashToStringViaYaml(r)
	if err != nil {
		return "", err
	}

	appHash, err := util.HashToStringViaYaml(apps)
	if err != nil {
		return "", err
	}

	hash := util.HashBytesToString([]byte(releaseHash + appHash))

	return hash, nil

}

func (r *ReleaseManifest) setReleaseBasedFields(app *AppConfig) {
	if app.RepoName == r.repoRef.String() {
		app.FilesOnly = true
	}
}

// isAppPinnedToThisRelease returns true if the named app is pinned to this release
func (r *ReleaseManifest) isAppPinnedToThisRelease(name string) bool {
	for n, a := range r.AppMetadata {
		if n == name {
			if a.PinnedReleaseVersion.EqualSafe(r.Version) {
				return true
			}
		}
	}
	return false
}

func (r *ReleaseManifest) GetAppManifestsPinnedToRelease() (map[string]*AppManifest, error) {

	manifests, err := r.GetAppManifests()
	if err != nil {
		return nil, err
	}

	out := map[string]*AppManifest{}

	for name, manifest := range manifests {
		if r.isAppPinnedToThisRelease(name) {
			out[name] = manifest
		}
	}

	return out, nil
}
