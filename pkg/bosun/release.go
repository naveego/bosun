package bosun

import (
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"strings"
)

type Release struct {
	Name     string                 `yaml:"name"`
	FromPath string                 `yaml:"fromPath"`
	Apps     map[string]*AppRelease `yaml:"apps"`
	Fragment *ConfigFragment        `yaml:"-"`
	// Indicates that this is not a real release which is stored on disk.
	// If this is true:
	// - release branch creation and checking is disabled
	// - local charts are used if available
	Transient bool `yaml:"-"`
}

func (r *Release) SetFragment(f *ConfigFragment) {
	r.FromPath = f.FromPath
	r.Fragment = f
	for _, app := range r.Apps {
		app.Release = r
	}
}

type AppRelease struct {
	Name      string   `yaml:"name"`
	Repo      string   `yaml:"repo"`
	RepoPath  string   `yaml:"repoPath"`
	BosunPath string   `yaml:"bosunPath"`
	Branch    string   `yaml:"branch"`
	Version   string   `yaml:"version"`
	ChartName string   `yaml:"chartName"`
	App       *App     `yaml:"-"`
	Release   *Release `yaml:"-"`
}

func (r *AppRelease) Validate(ctx BosunContext) []error {

	var errs []error

	out, err := pkg.NewCommand("helm", "search", r.ChartName, "-v", r.Version).RunOut()
	if err != nil {
		errs = append(errs, errors.Errorf("search for %s@%s failed: %s", r.ChartName, r.Version, err))
	}
	if !strings.Contains(out, r.ChartName) {
		errs = append(errs, errors.Errorf("chart %s@%s not found", r.ChartName, r.Version))
	}

	if !r.App.BranchForRelease {
		return errs
	}

	// TODO: validate docker image presence more efficiently
	err = pkg.NewCommand("docker", "pull",
		r.App.GetImageName(r.Version, r.Release.Name)).
		RunE()

	if err != nil {
		errs = append(errs, errors.Errorf("image not found: %s", err))
	}

	return errs

}

func (r *Release) IncludeDependencies(ctx BosunContext) error {
	allApps := ctx.Bosun.GetApps()
	var appNames []string
	for _, app := range r.Apps {
		appNames = append(appNames, app.Name)
	}

	// this is inefficient but it gets us all the dependencies
	topology, err := GetDependenciesInTopologicalOrder(allApps, appNames...)

	if err != nil {
		return errors.Errorf("apps could not be sorted in dependency order: %s", err)
	}

	for _, dep := range topology {
		app, ok := allApps[dep.Name]
		if !ok {
			return errors.Errorf("an app or dependency could not be found: %q from repo %q", dep.Name, dep.Repo)
		} else {
			if r.Apps[app.Name] == nil {

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
	for _, app := range r.Apps {
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
		return errors.Errorf("apps could not be sorted in dependency order: %s", err)
	}

	var toDeploy []*App

	for _, dep := range topology {
		app, ok := allApps[dep.Name]
		if !ok {
			return errors.Errorf("an app specifies a dependency that could not be found: %q from repo %q", dep.Name, dep.Repo)
		} else {
			if requestedAppNameSet[dep.Name] {
				toDeploy = append(toDeploy, app)
			} else {
				pkg.Log.Debugf("Skipping dependency %q because it was not requested.", dep.Name)
			}
		}
	}

	for _, app := range toDeploy {
		requested := requestedAppNameSet[app.Name]
		if requested {
			pkg.Log.Infof("App %q will be deployed because it was requested.", app.Name)
		} else {
			pkg.Log.Infof("App %q will be deployed because it was a dependency of a requested app.", app.Name)
		}
	}


	for _, app := range toDeploy {

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

func (r *Release) IncludeApp(app *App) error {

	var err error
	if r.Apps == nil {
		r.Apps = map[string]*AppRelease{}
	}
	r.Apps[app.Name], err = app.MakeAppRelease(r)
	if err != nil {
		return errors.Wrap(err, "make app release")
	}

	return nil
}
