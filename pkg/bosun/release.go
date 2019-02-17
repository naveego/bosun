package bosun

import (
	"bufio"
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"io"
	"os/exec"
	"sort"
	"strings"
)

type ReleaseConfig struct {
	Name              string                       `yaml:"name"`
	Description       string                       `yaml:"description"`
	FromPath          string                       `yaml:"fromPath"`
	AppReleaseConfigs map[string]*AppReleaseConfig `yaml:"apps"`
	Parent            *File                        `yaml:"-"`
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

// IsTransient returns true if r is nil or has Transient set to true.
func (r *Release) IsTransient() bool {
	return r == nil || r.Transient
}

func NewRelease(ctx BosunContext, r *ReleaseConfig) (*Release, error) {
	var err error
	if r.AppReleaseConfigs == nil {
		r.AppReleaseConfigs = map[string]*AppReleaseConfig{}
	}
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

func (r *ReleaseConfig) SetParent(f *File) {
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

func (a *AppRelease) Validate(ctx BosunContext) []error {

	var errs []error

	out, err := pkg.NewCommand("helm", "search", a.Chart, "-v", a.Version).RunOut()
	if err != nil {
		errs = append(errs, errors.Errorf("search for %s@%s failed: %s", a.Chart, a.Version, err))
	}
	if !strings.Contains(out, a.Chart) {
		errs = append(errs, errors.Errorf("chart %s@%s not found", a.Chart, a.Version))
	}

	if !a.AppRepo.BranchForRelease {
		return errs
	}

	imageName := fmt.Sprintf("%s:%s", a.Image, a.ImageTag)
	err = checkImageExists(imageName)

	if err != nil {
		errs = append(errs, errors.Errorf("image: %s", err))
	}

	// if a.AppRepo.IsRepoCloned() {
	// 	appBranch := a.AppRepo.GetBranch()
	// 	if appBranch != a.Branch {
	// 		errs = append(errs, errors.Errorf("app was added to release from branch %s, but is currently on branch %s", a.Branch, appBranch))
	// 	}
	//
	// 	appCommit := a.AppRepo.GetCommit()
	// 	if appCommit != a.Commit {
	// 		errs = append(errs, errors.Errorf("app was added to release at commit %s, but is currently on commit %s", a.Commit, appCommit))
	// 	}
	// }

	return errs
}

func checkImageExists(name string) error {
	cmd := exec.Command("docker", "pull", name)
	stdout, err := cmd.StdoutPipe()
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	reader := io.MultiReader(stdout, stderr)
	scanner := bufio.NewScanner(reader)

	if err := cmd.Start(); err != nil {
		return err
	}

	defer cmd.Process.Kill()

	var lines []string

	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if strings.Contains(line, "Pulling from") {
			return nil
		}
		if strings.Contains(line, "Error") {
			return errors.New(line)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	cmd.Process.Kill()

	state, err := cmd.Process.Wait()
	if err != nil {
		return err
	}

	if !state.Success() {
		return errors.Errorf("Pull failed: %s\n%s", state.String(), strings.Join(lines, "\n"))
	}

	return nil
}

func (r *Release) IncludeDependencies(ctx BosunContext) error {
	ctx = ctx.WithRelease(r)
	deps := ctx.Bosun.GetAppDependencyMap()
	var appNames []string
	for _, app := range r.AppReleaseConfigs {
		appNames = append(appNames, app.Name)
	}

	// this is inefficient but it gets us all the dependencies
	topology, err := GetDependenciesInTopologicalOrder(deps, appNames...)

	if err != nil {
		return errors.Errorf("repos could not be sorted in dependency order: %s", err)
	}

	for _, dep := range topology {
		if r.AppReleaseConfigs[dep] == nil {
			app, err := ctx.Bosun.GetApp(dep)
			if err != nil {
				return errors.Errorf("an app or dependency %q could not be found: %s", dep, err)
			} else {
				err = r.IncludeApp(ctx, app)
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
	dependencies := map[string][]string{}
	for _, app := range r.AppReleaseConfigs {
		requestedAppNames = append(requestedAppNames, app.Name)
		for _, dep := range app.DependsOn {
			dependencies[app.Name] = append(dependencies[app.Name], dep)
		}
	}

	topology, err := GetDependenciesInTopologicalOrder(dependencies, requestedAppNames...)

	if err != nil {
		return errors.Errorf("repos could not be sorted in dependency order: %s", err)
	}

	var toDeploy []*AppRelease

	for _, dep := range topology {
		app, ok := r.AppReleases[dep]
		if !ok {
			if r.Transient {
				continue
			}

			return errors.Errorf("an app specifies a dependency that could not be found: %q", dep)
		}

		if app.DesiredState.Status == StatusUnchanged {
			ctx.WithAppRelease(app).Log.Infof("Skipping deploy because desired state was %q.", StatusUnchanged)
			continue
		}

		toDeploy = append(toDeploy, app)
	}

	for _, app := range toDeploy {

		app.DesiredState.Status = StatusDeployed
		if app.DesiredState.Routing == "" {
			app.DesiredState.Routing = RoutingCluster
		}

		ctx.Bosun.SetDesiredState(app.Name, app.DesiredState)

		app.DesiredState.Force = ctx.GetParams().Force

		err = app.Reconcile(ctx)

		if err != nil {
			return err
		}
	}

	err = ctx.Bosun.Save()
	return err
}

func (r *Release) IncludeApp(ctx BosunContext, app *AppRepo) error {

	var err error
	var config *AppReleaseConfig
	if r.AppReleaseConfigs == nil {
		r.AppReleaseConfigs = map[string]*AppReleaseConfig{}
	}

	ctx = ctx.WithRelease(r)

	config, err = app.GetAppReleaseConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "make app release")
	}
	r.AppReleaseConfigs[app.Name] = config

	r.AppReleases[app.Name], err = NewAppRelease(ctx, config)

	return nil
}
