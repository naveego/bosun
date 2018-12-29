package bosun

import (
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"sort"
	"strings"
)

type ReleaseConfig struct {
	Name              string                       `yaml:"name"`
	FromPath          string                       `yaml:"fromPath"`
	AppReleaseConfigs map[string]*AppReleaseConfig `yaml:"repos"`
	Parent            *ConfigFragment              `yaml:"-"`
}

type Release struct {
	*ReleaseConfig
	// Indicates that this is not a real release which is stored on disk.
	// If this is true:
	// - release branch creation and checking is disabled
	// - local charts are used if available
	Transient   bool
	AppReleases AppReleaseMap
}

func NewRelease(ctx BosunContext, r *ReleaseConfig) (*Release, error) {
	var err error
	release := &Release{
		ReleaseConfig: r,
		AppReleases:   map[string]*AppRelease{},
	}
	for name, config := range r.AppReleaseConfigs {
		release.AppReleases[name], err = NewAppRelease(ctx, config)
		if err != nil {
			return nil, errors.Errorf("creating app release for config %q: %s", name, err)
		}
	}

	return release, nil
}

func (r *ReleaseConfig) SetParent(f *ConfigFragment) {
	r.FromPath = f.FromPath
	r.Parent = f
	for _, app := range r.AppReleaseConfigs {
		app.SetParent(r)
	}
}

type AppReleaseMap map[string]*AppRelease

func (a AppReleaseMap) GetAppsSortedByName() AppReleasesSortedByName {
	var out AppReleasesSortedByName
	for _, app := range a {
		out = append(out, app)
	}

	sort.Sort(out)
	return out
}

func (r *AppRelease) Validate(ctx BosunContext) []error {

	var errs []error

	out, err := pkg.NewCommand("helm", "search", r.Chart, "-v", r.Version).RunOut()
	if err != nil {
		errs = append(errs, errors.Errorf("search for %s@%s failed: %s", r.Chart, r.Version, err))
	}
	if !strings.Contains(out, r.Chart) {
		errs = append(errs, errors.Errorf("chart %s@%s not found", r.Chart, r.Version))
	}

	if !r.AppRepo.BranchForRelease {
		return errs
	}

	// TODO: validate docker image presence more efficiently
	err = pkg.NewCommand("docker", "pull",
		r.AppRepo.GetImageName(r.Version, r.ParentConfig.Name)).
		RunE()

	if err != nil {
		errs = append(errs, errors.Errorf("image not found: %s", err))
	}

	if r.AppRepo.IsRepoCloned() {
		appBranch := r.AppRepo.GetBranch()
		if appBranch != r.Branch {
			errs = append(errs, errors.Errorf("app was added to release from branch %s, but is currently on branch %s", r.Branch, appBranch))
		}

		appCommit := r.AppRepo.GetCommit()
		if appCommit != r.Commit {
			errs = append(errs, errors.Errorf("app was added to release at commit %s, but is currently on commit %s", r.Commit, appCommit))
		}
	}

	return errs
}

func (r *Release) IncludeDependencies(ctx BosunContext) error {
	allApps := ctx.Bosun.GetApps()
	var appNames []string
	for _, app := range r.AppReleaseConfigs {
		appNames = append(appNames, app.Name)
	}

	// this is inefficient but it gets us all the dependencies
	topology, err := GetDependenciesInTopologicalOrder(allApps, appNames...)

	if err != nil {
		return errors.Errorf("repos could not be sorted in dependency order: %s", err)
	}

	for _, dep := range topology {
		app, ok := allApps[dep.Name]
		if !ok {
			return errors.Errorf("an app or dependency could not be found: %q from repo %q", dep.Name, dep.Repo)
		} else {
			if r.AppReleaseConfigs[app.Name] == nil {

				err = r.IncludeApp(app)
				if err != nil {
					return errors.Errorf("could not include app %q: %s", app.Name, err)
				}
			}
		}
	}
	return nil
}

func (r *Release) Deploy(ctx BosunContext) error {

	ctx = ctx.WithRelease(r)

	var requestedAppNames []string
	requestedAppNameSet := map[string]bool{}
	for _, app := range r.AppReleaseConfigs {
		if app == nil {
			continue
		}
		requestedAppNameSet[app.Name] = true
	}
	for appName := range requestedAppNameSet {
		requestedAppNames = append(requestedAppNames, appName)
	}

	allApps := ctx.Bosun.GetApps()

	topology, err := GetDependenciesInTopologicalOrder(allApps, requestedAppNames...)

	if err != nil {
		return errors.Errorf("repos could not be sorted in dependency order: %s", err)
	}

	var toDeploy []*AppRepo

	for _, dep := range topology {
		app, ok := allApps[dep.Name]
		if !ok {
			return errors.Errorf("an app specifies a dependency that could not be found: %q from repo %q", dep.Name, dep.Repo)
		} else {
			if requestedAppNameSet[dep.Name] {
				toDeploy = append(toDeploy, app)
			} else {
				ctx.Log.Debugf("Skipping dependency %q because it was not requested.", dep.Name)
			}
		}
	}

	for _, app := range toDeploy {
		requested := requestedAppNameSet[app.Name]
		if requested {
			ctx.Log.Infof("AppRepo %q will be deployed because it was requested.", app.Name)
		} else {
			ctx.Log.Infof("AppRepo %q will be deployed because it was a dependency of a requested app.", app.Name)
		}
	}

	for _, app := range toDeploy {

		if appRelease, ok := r.AppReleaseConfigs[app.Name]; ok {
			app.ReleaseValues = appRelease.Values
		}

		app.DesiredState.Status = StatusDeployed
		if app.DesiredState.Routing == "" {
			app.DesiredState.Routing = RoutingCluster
		}

		app.DesiredState.Force = ctx.GetParams().Force

		// for transient release, use local chart if available
		if r.Transient && app.ChartPath != "" {
			app.Chart = ""
		}

		err = app.Reconcile(ctx)

		if err != nil {
			return err
		}
	}

	err = ctx.Bosun.Save()
	return err
}

func (r *Release) IncludeApp(app *AppRepo) error {

	var err error
	if r.AppReleaseConfigs == nil {
		r.AppReleaseConfigs = map[string]*AppReleaseConfig{}
	}
	r.AppReleaseConfigs[app.Name], err = app.MakeAppRelease(r)
	if err != nil {
		return errors.Wrap(err, "make app release")
	}

	return nil
}
