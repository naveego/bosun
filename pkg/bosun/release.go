package bosun

import (
	"bufio"
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type ReleaseConfig struct {
	Name              string                       `yaml:"name" json:"name"`
	Version           string                       `yaml:"version" json:"version"`
	Description       string                       `yaml:"description" json:"description"`
	FromPath          string                       `yaml:"fromPath" json:"fromPath"`
	AppReleaseConfigs map[string]*AppReleaseConfig `yaml:"apps" json:"apps"`
	Exclude           map[string]bool              `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	IsPatch           bool                         `yaml:"isPatch,omitempty" json:"isPatch,omitempty"`
	Parent            *File                        `yaml:"-" json:"-"`
	BundleFiles       map[string]*BundleFile       `yaml:"bundleFiles,omitempty"`
}

type BundleFile struct {
	App     string `yaml:"namespace"`
	Path    string `yaml:"path"`
	Content []byte `yaml:"-"`
}

func (r *ReleaseConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rcm ReleaseConfig
	var proxy rcm
	err := unmarshal(&proxy)
	if err == nil {
		if proxy.Exclude == nil {
			proxy.Exclude = map[string]bool{}
		}
		*r = ReleaseConfig(proxy)
	}
	return err
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

	if !a.App.BranchForRelease {
		return errs
	}

	for _, imageName := range a.ImageNames {
		imageName = fmt.Sprintf("%s:%s", imageName, a.ImageTag)
		err = checkImageExists(ctx, imageName)

		if err != nil {
			errs = append(errs, errors.Errorf("image %q: %s", imageName, err))
		}
	}

	// if a.App.IsCloned() {
	// 	appBranch := a.App.GetBranchName()
	// 	if appBranch != a.Branch {
	// 		errs = append(errs, errors.Errorf("app was added to release from branch %s, but is currently on branch %s", a.Branch, appBranch))
	// 	}
	//
	// 	appCommit := a.App.GetCommit()
	// 	if appCommit != a.Commit {
	// 		errs = append(errs, errors.Errorf("app was added to release at commit %s, but is currently on commit %s", a.Commit, appCommit))
	// 	}
	// }

	return errs
}

func checkImageExists(ctx BosunContext, name string) error {

	ctx.UseMinikubeForDockerIfAvailable()

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
			if _, ok := r.Exclude[dep]; ok {
				ctx.Log.Warnf("Dependency %q is not being added because it is in the exclude list. "+
					"Add it using the `add` command if want to override this exclusion.", dep)
				continue
			}
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
			if excluded := r.Exclude[dep]; excluded {
				continue
			}

			return errors.Errorf("an app specifies a dependency that could not be found: %q (excluded: %v)", dep, r.Exclude)
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

func (r *Release) IncludeApp(ctx BosunContext, app *App) error {

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

func (r *Release) AddBundleFile(app string, path string, content []byte) string {
	key := fmt.Sprintf("%s|%s", app, path)
	shortPath := safeFileNameRE.ReplaceAllString(strings.TrimLeft(path, "./\\"), "_")
	bf := &BundleFile{
		App:     app,
		Path:    shortPath,
		Content: content,
	}
	if r.BundleFiles == nil {
		r.BundleFiles = map[string]*BundleFile{}
	}
	r.BundleFiles[key] = bf
	return filepath.Join("./", r.Name, app, shortPath)
}

// GetBundleFileContent returns the content and path to a bundle file, or an error if it fails.
func (r *Release) GetBundleFileContent(app, path string) ([]byte, string, error) {
	key := fmt.Sprintf("%s|%s", app, path)
	bf, ok := r.BundleFiles[key]
	if !ok {
		return nil, "", errors.Errorf("no bundle for app %q and path %q", app, path)
	}

	bundleFilePath := filepath.Join(filepath.Dir(r.FromPath), r.Name, bf.App, bf.Path)
	content, err := ioutil.ReadFile(bundleFilePath)
	return content, bundleFilePath, err
}

func (r *ReleaseConfig) SaveBundle() error {
	bundleDir := filepath.Join(filepath.Dir(r.FromPath), r.Name)

	err := os.MkdirAll(bundleDir, 0770)
	if err != nil {
		return err
	}

	for _, bf := range r.BundleFiles {
		if bf.Content == nil {
			continue
		}

		appDir := filepath.Join(bundleDir, bf.App)
		err := os.MkdirAll(bundleDir, 0770)
		if err != nil {
			return err
		}

		bundleFilepath := filepath.Join(appDir, bf.Path)
		err = ioutil.WriteFile(bundleFilepath, bf.Content, 0770)
		if err != nil {
			return errors.Wrapf(err, "writing bundle file for app %q, path %q", bf.App, bf.Path)
		}
	}

	return nil
}

var safeFileNameRE = regexp.MustCompile(`([^A-z0-9_.]+)`)
