package bosun

import (
	"bufio"
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

//
// type ReleaseConfig struct {
// 	Name        string           `yaml:"name" json:"name"`
// 	Version     string           `yaml:"version" json:"version"`
// 	Description string           `yaml:"description" json:"description"`
// 	FromPath    string           `yaml:"fromPath" json:"fromPath"`
// 	Manifest    *ReleaseManifest `yaml:"manifest"`
// 	// AppReleaseConfigs map[string]*AppReleaseConfig `yaml:"apps" json:"apps"`
// 	Exclude     map[string]bool        `yaml:"exclude,omitempty" json:"exclude,omitempty"`
// 	IsPatch     bool                   `yaml:"isPatch,omitempty" json:"isPatch,omitempty"`
// 	Parent      *File                  `yaml:"-" json:"-"`
// 	BundleFiles map[string]*BundleFile `yaml:"bundleFiles,omitempty"`
// }

type BundleFile struct {
	App     string `yaml:"namespace"`
	Path    string `yaml:"path"`
	Content []byte `yaml:"-"`
}

//
// func (r *ReleaseConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
// 	type rcm ReleaseConfig
// 	var proxy rcm
// 	err := unmarshal(&proxy)
// 	if err == nil {
// 		if proxy.Exclude == nil {
// 			proxy.Exclude = map[string]bool{}
// 		}
// 		*r = ReleaseConfig(proxy)
// 	}
// 	return err
// }
//
// func (r *ReleaseConfig) Save() error {
//
// 	return nil
// }

type Deploy struct {
	*DeploySettings
	AppDeploys map[string]*AppDeploy
	Filtered   map[string]bool // contains any app deploys which were part of the release but which were filtered out
}

type DeploySettings struct {
	Environment        *EnvironmentConfig
	ValueSets          []values.ValueSet
	Manifest           *ReleaseManifest
	Apps               map[string]*App
	AppDeploySettings  map[string]AppDeploySettings
	UseLocalContent    bool
	Filter             *filter.Chain // If set, only apps which match the filter will be deployed.
	IgnoreDependencies bool
	ForceDeployApps    map[string]bool
	Recycle            bool
}

func (d DeploySettings) WithValueSets(valueSets ...values.ValueSet) DeploySettings {
	d.ValueSets = append(d.ValueSets, valueSets...)
	return d
}

func (d DeploySettings) GetImageTag(appMetadata *AppMetadata) string {
	if d.Manifest != nil {
		if d.Manifest.Name == SlotUnstable {
			return "latest"
		}

		// If app is not pinned, use the version from this release.
		if appMetadata.PinnedReleaseVersion == nil {
			return fmt.Sprintf("%s-%s", appMetadata.Version, d.Manifest.Version.String())
		}
	}

	if appMetadata.PinnedReleaseVersion == nil {
		return appMetadata.Version.String()
	}

	return fmt.Sprintf("%s-%s", appMetadata.Version, appMetadata.PinnedReleaseVersion)
}

type AppDeploySettings struct {
	Environment     *EnvironmentConfig
	ValueSets       []values.ValueSet
	UseLocalContent bool // if true, the app will be deployed using the local chart
}

func (d DeploySettings) GetAppDeploySettings(name string) AppDeploySettings {
	appSettings := d.AppDeploySettings[name]

	var valueSets []values.ValueSet

	// If the release manifest has value sets defined, they should be at a lower priority than the app value sets.
	if d.Manifest != nil {
		if d.Manifest.ValueSets != nil {
			valueSet := d.Manifest.ValueSets.ExtractValueSetByName(d.Environment.Name)
			valueSets = append(valueSets, valueSet)
		}
	}

	// Then add value sets from the deploy
	valueSets = append(valueSets, d.ValueSets...)

	// combine deploy and app value sets, with app value sets at a higher priority:
	valueSets = append(valueSets, appSettings.ValueSets...)

	appSettings.ValueSets = valueSets

	appSettings.Environment = d.Environment
	appSettings.UseLocalContent = d.UseLocalContent

	return appSettings
}

func NewDeploy(ctx BosunContext, settings DeploySettings) (*Deploy, error) {
	deploy := &Deploy{
		DeploySettings: &settings,
		AppDeploys:     AppDeployMap{},
		Filtered:       map[string]bool{},
	}

	if settings.Manifest != nil {
		appManifests, err := settings.Manifest.GetAppManifests()
		if err != nil {
			return nil, err
		}
		for _, manifest := range appManifests {
			if !settings.Manifest.UpgradedApps[manifest.Name] {
				if !settings.ForceDeployApps[manifest.Name] {
					ctx.Log().Debugf("Skipping %q because it is not default nor forced.", manifest.Name)
					deploy.Filtered[manifest.Name] = true
					continue
				}
			}

			appDeploy, err := NewAppDeploy(ctx, settings, manifest)
			if err != nil {
				return nil, errors.Wrapf(err, "create app deploy from manifest for %q", manifest.Name)
			}
			deploy.AppDeploys[appDeploy.Name] = appDeploy
		}
	} else if len(settings.Apps) > 0 {

		for _, app := range settings.Apps {
			appManifest, err := app.GetManifest(ctx)
			if err != nil {
				return nil, errors.Wrapf(err, "create app manifest for %q", app.Name)
			}
			appDeploy, err := NewAppDeploy(ctx, settings, appManifest)
			if err != nil {
				return nil, errors.Wrapf(err, "create app deploy from manifest for %q", appManifest.Name)
			}
			deploy.AppDeploys[appDeploy.Name] = appDeploy
		}
	} else {
		return nil, errors.New("either settings.Manifest or settings.Apps must be populated")
	}

	if settings.Filter != nil {
		appDeploys := deploy.AppDeploys
		filtered, err := settings.Filter.FromErr(appDeploys)
		if err != nil {
			return nil, errors.Wrap(err, "all apps were filtered out")
		}
		deploy.AppDeploys = filtered.(map[string]*AppDeploy)
		for name := range appDeploys {
			if _, ok := deploy.AppDeploys[name]; !ok {
				ctx.Log().Warnf("App %q was filtered out of the release.", name)
				deploy.Filtered[name] = true
			}
		}
	}

	return deploy, nil

}

type AppDeployMap map[string]*AppDeploy

func (a AppDeployMap) GetAppsSortedByName() AppReleasesSortedByName {
	var out AppReleasesSortedByName
	for _, app := range a {
		out = append(out, app)
	}

	sort.Sort(out)
	return out
}

func (a *AppDeploy) Validate(ctx BosunContext) []error {

	var errs []error

	out, err := pkg.NewShellExe("helm", "search", a.Chart(ctx), "-v", a.Version.String()).RunOut()
	if err != nil {
		errs = append(errs, errors.Errorf("search for %s@%s failed: %s", a.AppConfig.Chart, a.Version, err))
	}
	if !strings.Contains(out, a.AppConfig.Chart) {
		errs = append(errs, errors.Errorf("chart %s@%s not found", a.AppConfig.Chart, a.Version))
	}

	if !a.AppConfig.BranchForRelease {
		return errs
	}

	values, err := a.GetResolvedValues(ctx)
	if err != nil {
		return []error{err}
	}

	for _, imageConfig := range a.AppConfig.GetImages() {

		tag, ok := values.Values["tag"].(string)
		if !ok {
			tag = a.Version.String()
		}

		imageName := imageConfig.GetFullNameWithTag(tag)
		err = checkImageExists(ctx, imageName)

		if err != nil {
			errs = append(errs, errors.Errorf("image %q: %s", imageConfig, err))
		}
	}

	// if a.App.IsCloned() {
	// 	appBranch := a.App.GetBranchName()
	// 	if appBranch != a.Branch {
	// 		errs = append(errs, errors.Errorf("app was added to release from branch %s, but is currently on branch %s", a.Branch, appBranch))
	// 	}
	//
	// 	appCommit := a.App.GetCommit()
	// 	if appCommit != a.GetCurrentCommit {
	// 		errs = append(errs, errors.Errorf("app was added to release at commit %s, but is currently on commit %s", a.GetCurrentCommit, appCommit))
	// 	}
	// }

	return errs
}

func checkImageExists(ctx BosunContext, name string) error {

	cmd := exec.Command("docker", "pull", name)
	stdout, err := cmd.StdoutPipe()
	stderr, err := cmd.StderrPipe()
	//cmd.Env = ctx.GetMinikubeDockerEnv()
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

//
// func (r *Deploy) IncludeDependencies(ctx BosunContext) error {
// 	ctx = ctx.WithRelease(r)
// 	deps := ctx.Bosun.GetAppDependencyMap()
// 	var appNames []string
// 	for _, app := range r.AppReleaseConfigs {
// 		appNames = append(appNames, app.Name)
// 	}
//
// 	// this is inefficient but it gets us all the dependencies
// 	topology, err := GetDependenciesInTopologicalOrder(deps, appNames...)
//
// 	if err != nil {
// 		return errors.Errorf("repos could not be sorted in dependency order: %s", err)
// 	}
//
// 	for _, dep := range topology {
// 		if r.AppReleaseConfigs[dep] == nil {
// 			if _, ok := r.Exclude[dep]; ok {
// 				ctx.Log().Warnf("Dependency %q is not being added because it is in the exclude list. "+
// 					"Add it using the `add` command if want to override this exclusion.", dep)
// 				continue
// 			}
// 			app, err := ctx.Bosun.GetApp(dep)
// 			if err != nil {
// 				return errors.Errorf("an app or dependency %q could not be found: %s", dep, err)
// 			} else {
// 				err = r.MakeAppAvailable(ctx, app)
// 				if err != nil {
// 					return errors.Errorf("could not include app %q: %s", app.Name, err)
// 				}
// 			}
// 		}
// 	}
// 	return nil
// }

func (r *Deploy) Deploy(ctx BosunContext) error {

	var requestedAppNames []string
	dependencies := map[string][]string{}
	for _, app := range r.AppDeploys {
		requestedAppNames = append(requestedAppNames, app.Name)
		for _, dep := range app.AppConfig.DependsOn {
			dependencies[app.Name] = append(dependencies[app.Name], dep.Name)
		}
	}

	topology, err := GetDependenciesInTopologicalOrder(dependencies, requestedAppNames...)

	if err != nil {
		return errors.Errorf("repos could not be sorted in dependency order: %s", err)
	}

	var toDeploy []*AppDeploy

	for _, dep := range topology {
		app, ok := r.AppDeploys[dep]
		if !ok {
			if r.IgnoreDependencies {
				continue
			}
			if filtered := r.Filtered[dep]; filtered {
				continue
			}

			return errors.Errorf("an app specifies a dependency that could not be found: %q (filtered: %#v)", dep, r.Filtered)
		}

		if app.DesiredState.Status == StatusUnchanged {
			ctx.WithAppDeploy(app).Log().Infof("Skipping deploy because desired state was %q.", StatusUnchanged)
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

		app.DesiredState.Force = ctx.GetParameters().Force

		err = app.Reconcile(ctx)

		if err != nil {
			return err
		}
		if r.Recycle {
			err = app.Recycle(ctx)
			if err != nil {
				return err
			}
		}
	}

	err = ctx.Bosun.Save()
	return err
}

//
// func (r *Deploy) MakeAppAvailable(ctx BosunContext, app *App) error {
//
// 	var err error
// 	var config *AppReleaseConfig
// 	if r.AppReleaseConfigs == nil {
// 		r.AppReleaseConfigs = map[string]*AppReleaseConfig{}
// 	}
//
// 	ctx = ctx.WithRelease(r)
//
// 	config, err = app.GetAppReleaseConfig(ctx)
// 	if err != nil {
// 		return errors.Wrap(err, "make app release")
// 	}
// 	r.AppReleaseConfigs[app.Name] = config
//
// 	r.Apps[app.Name], err = NewAppRelease(ctx, config)
//
// 	return nil
// }
//
// func (r *Deploy) AddBundleFile(app string, path string, content []byte) string {
// 	key := fmt.Sprintf("%s|%s", app, path)
// 	shortPath := safeFileNameRE.ReplaceAllString(strings.TrimLeft(path, "./\\"), "_")
// 	bf := &BundleFile{
// 		App:     app,
// 		Path:    shortPath,
// 		Content: content,
// 	}
// 	if r.BundleFiles == nil {
// 		r.BundleFiles = map[string]*BundleFile{}
// 	}
// 	r.BundleFiles[key] = bf
// 	return filepath.Join("./", r.Name, app, shortPath)
// }
//
// // GetBundleFileContent returns the content and path to a bundle file, or an error if it fails.
// func (r *Deploy) GetBundleFileContent(app, path string) ([]byte, string, error) {
// 	key := fmt.Sprintf("%s|%s", app, path)
// 	bf, ok := r.BundleFiles[key]
// 	if !ok {
// 		return nil, "", errors.Errorf("no bundle for app %q and path %q", app, path)
// 	}
//
// 	bundleFilePath := filepath.Join(filepath.Dir(r.FromPath), r.Name, bf.App, bf.Path)
// 	content, err := ioutil.ReadFile(bundleFilePath)
// 	return content, bundleFilePath, err
// }
//
// func (r *ReleaseConfig) SaveBundle() error {
// 	bundleDir := filepath.Join(filepath.Dir(r.FromPath), r.Name)
//
// 	err := os.MkdirAll(bundleDir, 0770)
// 	if err != nil {
// 		return err
// 	}
//
// 	for _, bf := range r.BundleFiles {
// 		if bf.Content == nil {
// 			continue
// 		}
//
// 		appDir := filepath.Join(bundleDir, bf.App)
// 		err := os.MkdirAll(bundleDir, 0770)
// 		if err != nil {
// 			return err
// 		}
//
// 		bundleFilepath := filepath.Join(appDir, bf.Path)
// 		err = ioutil.WriteFile(bundleFilepath, bf.Content, 0770)
// 		if err != nil {
// 			return errors.Wrapf(err, "writing bundle file for app %q, path %q", bf.App, bf.Path)
// 		}
// 	}
//
// 	return nil
// }

var safeFileNameRE = regexp.MustCompile(`([^A-z0-9_.]+)`)
