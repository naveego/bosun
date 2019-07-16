package bosun

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"path/filepath"
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
	ValueSets         ValueSetMap             `yaml:"valueSets,omitempty"`
	Platform          *Platform               `yaml:"-"`
	plan              *ReleasePlan            `yaml:"-"`
	toDelete          []string                `yaml:"-"`
	dirty             bool                    `yaml:"-"`
	dir               string                  `yaml:"-"`
	appManifests      map[string]*AppManifest `yaml:"-" json:"-"`
	deleted           bool                    `yaml:"-"`
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

	if r.appManifests == nil {
		appManifests := map[string]*AppManifest{}

		allAppMetadata := r.GetAllAppMetadata()
		for appName, _ := range allAppMetadata {
			appReleasePath := filepath.Join(r.dir, appName+".yaml")
			b, err := ioutil.ReadFile(appReleasePath)
			if err != nil {
				return nil, errors.Wrapf(err, "load appRelease for app  %q", appName)
			}
			var appManifest *AppManifest
			err = yaml.Unmarshal(b, &appManifest)
			if err != nil {
				return nil, errors.Wrapf(err, "unmarshal appRelease for app  %q", appName)
			}

			appManifest.AppConfig.FromPath = appReleasePath

			appManifests[appName] = appManifest
		}

		r.appManifests = appManifests
	}
	return r.appManifests, nil
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
// in that branch based on the bump parameter (if the app's current version is expectedVersion).
// If the bump parameter is "none" then the app won't be bumped.
func (r *ReleaseManifest) UpgradeApp(ctx BosunContext, name, fromBranch, toBranch, bump string, expectedVersion semver.Version) error {
	r.init()
	r.MarkDirty()

	b := ctx.Bosun
	app, err := b.GetApp(name)
	if err != nil {
		return err
	}

	appConfig := app.AppConfig

	if appConfig.BranchForRelease {
		log := ctx.Log.WithField("app", appConfig.Name)

		log.Infof("Upgrade requested for for app %q; creating release branch and upgrading manifest...", name)

		if !app.IsRepoCloned() {
			return errors.New("repo is not cloned but must be branched for release; what is going on?")
		}

		localRepo := app.Repo.LocalRepo
		if localRepo.IsDirty() {
			return errors.New("repo is dirty, commit or stash your changes before adding it to the release")
		}

		log.Debug("Checking if release branch exists...")

		branchExists, err := localRepo.DoesBranchExist(ctx, toBranch)
		if err != nil {
			return err
		}
		if branchExists {
			log.Info("Release branch already exists, switching to it.")
			err = localRepo.SwitchToBranchAndPull(ctx, toBranch)
			if err != nil {
				return errors.Wrap(err, "switching to release branch")
			}
		} else {
			log.Info("Creating release branch...")
			err = localRepo.SwitchToNewBranch(ctx, fromBranch, toBranch)
			if err != nil {
				return errors.Wrap(err, "creating release branch")
			}
		}

		if bump != "none" && app.Version == expectedVersion {
			if app.Version != expectedVersion {
				log.Warnf("Skipping version bump %q because version on branch is already %s (source branch is version %s).", bump, expectedVersion, app.Version)
			} else {
				log.Infof("Applying version bump %q to source branch version %s.", bump, expectedVersion)

				err = app.BumpVersion(ctx, bump)
				if err != nil {
					return errors.Wrap(err, "bumping version")
				}

				err = localRepo.Commit(fmt.Sprintf("chore(version): Bump version to %s for release %s.", app.Version, r.Name), ".")
				if err != nil {
					return errors.Wrap(err, "committing bumped version")
				}
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
		err = r.AddApp(appManifest, true)
		if err != nil {
			return err
		}
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
	delete(r.DefaultDeployApps, appName)
	r.toDelete = append(r.toDelete, appName)
}

func (r *ReleaseManifest) AddApp(manifest *AppManifest, addToDefaultDeploys bool) error {
	r.MarkDirty()
	r.init()
	appManifests, err := r.GetAppManifests()
	if err != nil {
		return err
	}
	appManifests[manifest.Name] = manifest
	r.AppMetadata[manifest.Name] = manifest.AppMetadata
	if addToDefaultDeploys {
		if r.DefaultDeployApps == nil {
			r.DefaultDeployApps = map[string]bool{}
		}
		r.DefaultDeployApps[manifest.Name] = true
	}
	return nil
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
