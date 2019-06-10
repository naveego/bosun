package bosun

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
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
	*ReleaseMetadata  `yaml:"metadata"`
	DefaultDeployApps map[string]bool         `yaml:"defaultDeployApps"`
	AppMetadata       map[string]*AppMetadata `yaml:"apps"`
	AppManifests      map[string]*AppManifest `yaml:"-" json:"-"`
	Plan              *ReleasePlan            `yaml:"plan"`
	Platform          *Platform               `yaml:"-"`
	toDelete          []string                `yaml:"-"`
	dirty             bool                    `yaml:"-"`
}

func NewReleaseManifest(metadata *ReleaseMetadata) *ReleaseManifest {
	r := &ReleaseManifest{ReleaseMetadata: metadata}
	r.init()
	return r
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

// init ensures the instance is ready to use.
func (r *ReleaseManifest) init() {
	if r.AppManifests == nil {
		r.AppManifests = map[string]*AppManifest{}
	}
	if r.AppMetadata == nil {
		r.AppMetadata = map[string]*AppMetadata{}
	}
}

func (r *ReleaseManifest) Headers() []string {
	return []string{"Name", "Version", "From Release", "Commit Hash", "Deploying"}
}

func (r *ReleaseManifest) Rows() [][]string {
	var out [][]string
	for _, name := range util.SortedKeys(r.AppMetadata) {
		deploy := r.DefaultDeployApps[name]
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

// UpgradeApp upgrades the named app by creating a release branch and bumping the version
// in that branch based on the bump parameter. If the bump parameter is "none" then the app
// won't be bumped.
func (r *ReleaseManifest) UpgradeApp(ctx BosunContext, name, fromBranch, toBranch, bump string) error {
	r.init()
	r.MarkDirty()

	b := ctx.Bosun
	app, err := b.GetApp(name)
	if err != nil {
		return err
	}

	appConfig := app.AppConfig

	if appConfig.BranchForRelease {

		ctx.Log.Infof("Upgrade requested for for app %q; creating release branch and upgrading manifest...", name)

		if !app.IsRepoCloned() {
			return errors.New("repo is not cloned but must be branched for release; what is going on?")
		}

		localRepo := app.Repo.LocalRepo
		if localRepo.IsDirty() {
			return errors.New("repo is dirty, commit or stash your changes before adding it to the release")
		}

		ctx.Log.Info("Creating branch if needed...")

		err = localRepo.Branch(ctx, fromBranch, toBranch)

		if err != nil {
			return errors.Wrap(err, "create branch for release")
		}

		if bump != "none" {

			err = app.BumpVersion(ctx, bump)
			if err != nil {
				return errors.Wrap(err, "bumping version")
			}

			err = localRepo.Commit(fmt.Sprintf("chore(version): Bump version to %s for release %s.", app.Version, r.Name), ".")
			if err != nil {
				return errors.Wrap(err, "committing bumped version")
			}
		}

		ctx.Log.Info("Branching and version bumping completed.")

		app, err = ctx.Bosun.ReloadApp(app.Name)
		if err != nil {
			return errors.Wrap(err, "reload app after switching to new branch")
		}
	}

	appManifest, err := app.GetManifest(ctx)
	if err != nil {
		return err
	}

	appManifest.PinToRelease(r.ReleaseMetadata)

	r.AddApp(appManifest, true)

	return nil
}

func (r *ReleaseManifest) RefreshApp(ctx BosunContext, name string) error {

	b := ctx.Bosun
	app, err := b.GetApp(name)
	if err != nil {
		return err
	}
	ctx = ctx.WithApp(app)
	currentAppManifest, err := r.GetAppManifest(name)
	if err != nil {
		return err
	}

	if app.IsRepoCloned() {

		appReleaseBranch := currentAppManifest.Branch
		currentBranch := app.GetBranchName()

		if appReleaseBranch != string(currentBranch) {
			defer func() {
				e := app.CheckOutBranch(string(currentBranch))
				if e != nil {
					ctx.Log.WithError(e).Warnf("Returning to branch %q failed.", currentBranch)
				}
			}()
			err = app.CheckOutBranch(appReleaseBranch)
			if err != nil {
				return errors.Wrapf(err, "could not check out %q branch for app %q", appReleaseBranch, name)
			}
		}

		err = app.Repo.Pull(ctx)
		if err != nil {
			return errors.Wrapf(err, "pull app %q", name)
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
		appManifest.PinToRelease(r.ReleaseMetadata)
		r.AddApp(appManifest, true)
	} else {
		ctx.Log.Debug("No changes detected.")
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

	r.AppManifests[appManifest.Name] = appManifest

	return nil
}

func (r *ReleaseManifest) ExportDiagram() string {
	export := `# dot -Tpng myfile.dot >myfile.png
digraph g {
  rankdir="LR";
  node[style="rounded",shape="box"]
  edge[splines="curved"]`
	for _, app := range r.AppManifests {

		export += fmt.Sprintf("%q;\n", app.Name)
		for _, dep := range app.AppConfig.DependsOn {
			export += fmt.Sprintf("%q -> %q;\n", app.Name, dep.Name)
		}
	}

	export += "}"
	return export
}

func (r *ReleaseManifest) RemoveApp(appName string) {
	r.MarkDirty()
	r.init()
	delete(r.AppMetadata, appName)
	delete(r.AppManifests, appName)
	delete(r.DefaultDeployApps, appName)
	r.toDelete = append(r.toDelete, appName)
}

func (r *ReleaseManifest) AddApp(manifest *AppManifest, addToDefaultDeploys bool) {
	r.MarkDirty()
	r.init()
	r.AppManifests[manifest.Name] = manifest
	r.AppMetadata[manifest.Name] = manifest.AppMetadata
	if addToDefaultDeploys {
		if r.DefaultDeployApps == nil {
			r.DefaultDeployApps = map[string]bool{}
		}
		r.DefaultDeployApps[manifest.Name] = true
	}
}

func (r *ReleaseManifest) MarkDirty() {
	r.dirty = true
}

func (r *ReleaseManifest) GetAppManifest(name string) (*AppManifest, error) {
	if a, ok := r.AppManifests[name]; ok {
		return a, nil
	}

	return nil, errors.Errorf("no app manifest with name %q in release %q", name, r.Name)

}
