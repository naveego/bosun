package bosun

import (
	"context"
	"fmt"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/google/go-github/v20/github"
	vault "github.com/hashicorp/vault/api"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/mirror"
	"github.com/naveego/bosun/pkg/zenhub"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Bosun struct {
	params               Parameters
	ws                   *Workspace
	file                 *File
	apps                 map[string]*App
	vaultClient          *vault.Client
	env                  *EnvironmentConfig
	clusterAvailable     *bool
	log                  *logrus.Entry
	environmentConfirmed *bool
	repos                map[string]*Repo
	platform             *Platform
}

type Parameters struct {
	Verbose        bool
	DryRun         bool
	Force          bool
	NoReport       bool
	ForceTests     bool
	ValueOverrides map[string]string
	FileOverrides  []string
	NoCurrentEnv   bool
	ConfirmedEnv   string
}

func New(params Parameters, ws *Workspace) (*Bosun, error) {
	b := &Bosun{
		params: params,
		ws:     ws,
		file:   ws.MergedBosunFile,
		apps:   make(map[string]*App),
		log:    pkg.Log,
		repos:  map[string]*Repo{},
	}

	if params.DryRun {
		b.log = b.log.WithField("*DRYRUN*", "")
		b.log.Info("DRY RUN")
	}

	for _, dep := range b.file.AppRefs {
		b.apps[dep.Name] = NewAppFromDependency(dep)
	}

	for _, a := range b.file.Apps {
		if a != nil {
			_, err := b.addApp(a)
			if err != nil {
				return nil, errors.Wrapf(err, "add app %q", a.Name)
			}
		}
	}

	if !params.NoCurrentEnv {
		err := b.configureCurrentEnv()
		if err != nil {
			return nil, err
		}
	}

	return b, nil
}

func (b *Bosun) addApp(config *AppConfig) (*App, error) {
	if config.Name == "" {
		return nil, errors.New("cannot accept an app with no name")
	}
	app := NewApp(config)
	if app.RepoName == "" {
		repos := b.GetRepos()
		for _, r := range repos {
			if r.LocalRepo != nil {
				if strings.HasPrefix(app.FromPath, r.LocalRepo.Path) {
					// This app is part of this repo, whether the app knows it or not.
					app.RepoName = r.Name
					app.Repo = r
					r.Apps[app.Name] = config
				}
			}
		}
	}

	if app.RepoName != "" {
		// find or add repo for app
		repo, err := b.GetRepo(app.RepoName)
		if err != nil {
			repo = &Repo{
				Apps: map[string]*AppConfig{
					app.Name: config,
				},
				RepoConfig: RepoConfig{
					ConfigShared: ConfigShared{
						Name: app.Name,
					},
				},
			}
			b.repos[app.RepoName] = repo
			config.Parent.Repos = append(config.Parent.Repos, &repo.RepoConfig)

		}
		app.Repo = repo
		var ok bool

		if repo.LocalRepo, ok = b.ws.LocalRepos[app.RepoName]; !ok {
			localRepoPath, err := git.GetRepoPath(app.FromPath)
			if err == nil {
				repo.LocalRepo = &LocalRepo{
					Name: app.RepoName,
					Path: localRepoPath,
				}
			}
			b.ws.LocalRepos[app.RepoName] = repo.LocalRepo
		}

	}

	b.apps[config.Name] = app

	// for _, d2 := range app.DependsOn {
	// 	if _, ok := b.apps[d2.Name]; !ok {
	// 		b.apps[d2.Name] = NewAppFromDependency(&d2)
	// 	}
	// }

	return app, nil
}

func (b *Bosun) GetAppsSortedByName() []*App {
	var ms AppsSortedByName

	apps := b.GetApps()

	for _, x := range apps {
		if x.Name != "" {
			ms = append(ms, x)
		}
	}
	sort.Sort(ms)
	return ms
}

func (b *Bosun) GetApps() map[string]*App {
	out := map[string]*App{}

	for _, app := range b.apps {
		out[app.Name] = app
	}

	p, err := b.GetCurrentPlatform()
	if err == nil {
		master, err := p.GetMasterManifest()
		if err == nil {
			appManifests, err := master.GetAppManifests()
			if err == nil {

				for appName, appManifest := range appManifests {
					if _, ok := out[appName]; !ok {
						out[appName] = NewApp(appManifest.AppConfig)
					}
				}
			}
		}
	}

	return out
}

func (b *Bosun) GetAppDesiredStates() map[string]AppState {
	return b.ws.AppStates[b.env.Name]
}

func (b *Bosun) GetAppDependencyMap() map[string][]string {
	deps := map[string][]string{}
	for _, app := range b.GetApps() {
		for _, dep := range app.DependsOn {
			deps[app.Name] = append(deps[app.Name], dep.Name)
		}
	}
	return deps
}

func (b *Bosun) GetAppDependencies(name string) ([]string, error) {

	visited := map[string]bool{}

	return b.getAppDependencies(name, visited)
}

func (b *Bosun) getAppDependencies(name string, visited map[string]bool) ([]string, error) {
	visited[name] = true

	app, err := b.GetApp(name)
	if err != nil {
		return nil, err
	}

	var out []string

	for _, dep := range app.DependsOn {
		if visited[dep.Name] {
			continue
		}
		visited[dep.Name] = true
		out = append(out, dep.Name)

		children, err := b.getAppDependencies(dep.Name, visited)
		if err != nil {
			return nil, errors.Errorf("%s:%s", name, err)
		}
		out = append(out, children...)
	}
	return out, nil
}

func (b *Bosun) GetVaultClient() (*vault.Client, error) {
	var err error
	if b.vaultClient == nil {
		b.vaultClient, err = pkg.NewVaultLowlevelClient("", "")
	}
	return b.vaultClient, err
}

func (b *Bosun) GetScripts() []*Script {
	env := b.GetCurrentEnvironment()

	scripts := make([]*Script, len(env.Scripts))
	copy(scripts, env.Scripts)
	copy(scripts, b.GetMergedConfig().Scripts)
	for _, app := range b.GetAppsSortedByName() {
		for _, script := range app.Scripts {
			script.Name = fmt.Sprintf("%s-%s", app.Name, script.Name)
			scripts = append(scripts, script)
		}
	}

	return scripts
}

func (b *Bosun) GetScript(name string) (*Script, error) {
	for _, script := range b.GetScripts() {
		if script.Name == name {
			return script, nil
		}
	}

	return nil, errors.Errorf("no script found with name %q", name)
}

func (b *Bosun) GetAppWithRepo(name string) (*App, error) {
	m, ok := b.apps[name]
	if !ok {
		return nil, errors.Errorf("no app with name %q has been cloned", name)
	}
	return m, nil
}

func (b *Bosun) GetApp(name string) (*App, error) {
	m, ok := b.apps[name]
	if !ok {

		p, err := b.GetCurrentPlatform()
		if err != nil {
			return nil, errors.Errorf("no app named %q, and no platform available for finding latest release", name)
		}

		manifest, err := p.GetLatestAppManifestByName(name)
		if err != nil {
			return nil, err
		}

		return NewApp(manifest.AppConfig), nil
	}
	return m, nil
}

func (b *Bosun) ReloadApp(name string) (*App, error) {
	app, ok := b.apps[name]
	if !ok {
		return nil, errors.Errorf("no app named %q", name)
	}

	file := &File{
		AppRefs: map[string]*Dependency{},
	}

	err := pkg.LoadYaml(app.FromPath, &file)
	if err != nil {
		return nil, err
	}

	file.SetFromPath(app.FromPath)

	for _, updatedApp := range file.Apps {
		if updatedApp.Name == name {
			app, err = b.addApp(updatedApp)
			return app, err
		}
	}

	return nil, errors.Errorf("could not find app in source file at %q", app.FromPath)
}

func (b *Bosun) GetOrAddAppForPath(path string) (*App, error) {
	for _, m := range b.apps {
		if m.FromPath == path {
			return m, nil
		}
	}

	err := b.ws.importFileFromPath(path)

	if err != nil {
		return nil, err
	}

	b.log.WithField("path", path).Debug("New microservice found at path.")

	imported := b.ws.ImportedBosunFiles[path]

	var name string
	for _, m := range imported.Apps {
		_, err = b.addApp(m)
		if err != nil {
			return nil, err
		}
		name = m.Name
	}

	m, _ := b.GetApp(name)
	return m, nil
}

func (b *Bosun) useEnvironment(env *EnvironmentConfig) error {

	b.ws.CurrentEnvironment = env.Name
	b.env = env

	err := b.env.ForceEnsure(b.NewContext())
	if err != nil {
		return errors.Errorf("ensure environment %q: %s", b.env.Name, err)
	}

	return nil
}

func (b *Bosun) UseEnvironment(name string) error {

	env, err := b.GetEnvironment(name)
	if err != nil {
		return err
	}

	return b.useEnvironment(env)
}

func (b *Bosun) GetCurrentEnvironment() *EnvironmentConfig {
	if b.env == nil {
		err := b.configureCurrentEnv()
		if err != nil {
			panic(errors.Errorf("environment was not initialized; initializing environment caused error: %s", err))
		}
	}

	return b.env
}

func (b *Bosun) SetDesiredState(app string, state AppState) {
	env := b.env
	if b.ws.AppStates == nil {
		b.ws.AppStates = AppStatesByEnvironment{}
	}
	m, ok := b.ws.AppStates[env.Name]
	if !ok {
		m = AppStateMap{}
		b.ws.AppStates[env.Name] = m
	}
	m[app] = state
}

func (b *Bosun) Save() error {

	ws := b.ws
	data, err := yaml.Marshal(ws)
	if err != nil {
		return errors.Wrap(err, "marshalling for save")
	}

	err = ioutil.WriteFile(ws.Path, data, 0600)
	if err != nil {
		return errors.Wrap(err, "writing for save")
	}

	return nil
}

func (b *Bosun) SetInWorkspace(path string, value interface{}) error {

	ws := b.ws
	data, err := yaml.Marshal(ws)
	if err != nil {
		return errors.Wrap(err, "marshalling for save")
	}
	v, err := ReadValues(data)
	if err != nil {
		panic(err)
	}

	err = v.SetAtPath(path, value)
	if err != nil {
		return err
	}

	yml, err := v.YAML()
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(ws.Path, []byte(yml), 0600)
	if err != nil {
		return errors.Wrap(err, "writing for save")
	}

	return nil
}

func (b *Bosun) GetInWorkspace(path string) (interface{}, error) {

	ws := b.ws
	data, err := yaml.Marshal(ws)
	if err != nil {
		return Values{}, errors.Wrap(err, "marshalling for save")
	}
	v, err := ReadValues(data)
	if err != nil {
		panic(err)
	}

	return v.GetAtPath(path)
}

func (b *Bosun) GetWorkspace() *Workspace {
	return b.ws
}

func (b *Bosun) GetMergedConfig() File {
	return *b.file
}

func (b *Bosun) AddImport(file string) bool {
	for _, i := range b.ws.Imports {
		if i == file {
			return false
		}
	}
	b.ws.Imports = append(b.ws.Imports, file)
	return true
}

func (b *Bosun) IsClusterAvailable() bool {
	env := b.GetCurrentEnvironment()
	if b.clusterAvailable == nil {
		b.log.Debugf("Checking if cluster %q is available...", env.Cluster)
		resultCh := make(chan bool)
		cmd := exec.Command("kubectl", "cluster-info")
		go func() {
			err := cmd.Run()
			if err != nil {
				resultCh <- false
			} else {
				resultCh <- true
			}
		}()

		select {
		case result := <-resultCh:
			b.clusterAvailable = &result
			b.log.Debugf("Cluster is available: %t", result)
		case <-time.After(2 * time.Second):
			b.log.Warn("Cluster did not respond quickly, I will assume it is unavailable.")
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			b.clusterAvailable = github.Bool(false)
		}
	}

	return *b.clusterAvailable
}

func (b *Bosun) GetEnvironment(name string) (*EnvironmentConfig, error) {
	for _, env := range b.file.Environments {
		if env.Name == name {
			return env, nil
		}
	}
	return nil, errors.Errorf("no environment named %q", name)
}

func (b *Bosun) GetEnvironments() []*EnvironmentConfig {
	return b.file.Environments
}

func (b *Bosun) GetValueSet(name string) (*ValueSet, error) {
	for _, vs := range b.file.ValueSets {
		if vs.Name == name {
			return vs, nil
		}
	}
	return nil, errors.Errorf("no valueSet named %q", name)
}

func (b *Bosun) GetValueSetSlice(names []string) ([]ValueSet, error) {
	var out []ValueSet
	want := map[string]bool{}
	for _, name := range names {
		want[name] = false
	}

	for _, vs := range b.file.ValueSets {
		if _, wanted := want[vs.Name]; wanted {
			out = append(out, *vs)
			want[vs.Name] = true
		}
	}

	for name, found := range want {
		if !found {
			return nil, errors.Errorf("wanted value set %q was not found", name)
		}
	}

	return out, nil
}

func (b *Bosun) GetValueSetsForEnv(env *EnvironmentConfig) ([]*ValueSet, error) {
	vss := map[string]*ValueSet{}
	for _, vs := range b.file.ValueSets {
		vss[vs.Name] = vs
	}

	var out []*ValueSet
	for _, name := range env.ValueSets {
		vs, ok := vss[name]
		if !ok {
			return nil, errors.Errorf("no valueSet with name %q", name)
		}
		out = append(out, vs)
	}

	mirror.Sort(out, func(a, b *ValueSet) bool {
		return a.Name < b.Name
	})

	return out, nil
}

func (b *Bosun) GetValueSets() []*ValueSet {
	out := make([]*ValueSet, len(b.file.ValueSets))
	copy(out, b.file.ValueSets)

	mirror.Sort(out, func(a, b *ValueSet) bool {
		return a.Name < b.Name
	})

	return out
}

func (b *Bosun) NewContext() BosunContext {

	dir, _ := os.Getwd()

	return BosunContext{
		Bosun: b,
		Env:   b.GetCurrentEnvironment(),
		Log:   b.log,
	}.WithDir(dir).WithContext(context.Background())

}

func (b *Bosun) GetCurrentReleaseManifest(loadAppManifests bool) (*ReleaseManifest, error) {
	var err error
	if b.ws.CurrentRelease == "" {
		return nil, errors.New("current release not set, call `bosun release use {name}` to set current release")
	}

	p, err := b.GetCurrentPlatform()
	if err != nil {
		return nil, err
	}

	rm, err := p.GetReleaseManifestByName(b.ws.CurrentRelease)
	return rm, err
}

func (b *Bosun) GetCurrentReleaseMetadata() (*ReleaseMetadata, error) {
	var err error
	if b.ws.CurrentRelease == "" {
		return nil, errors.New("current release not set, call `bosun release use {name}` to set current release")
	}

	p, err := b.GetCurrentPlatform()
	if err != nil {
		return nil, err
	}

	rm, err := p.GetReleaseMetadataByName(b.ws.CurrentRelease)
	return rm, err
}

func (b *Bosun) GetCurrentPlatform() (*Platform, error) {
	if b.platform != nil {
		return b.platform, nil
	}

	switch len(b.file.Platforms) {
	case 0:
		return nil, errors.New("no platforms found")
	case 1:
		b.platform = b.file.Platforms[0]
		return b.platform, nil
	default:
		if b.ws.CurrentPlatform == "" {
			return nil, errors.New("no current platform selected; use `bosun platform use-platform` to set it")
		}
		for _, p := range b.file.Platforms {
			if p.Name == b.ws.CurrentPlatform {
				b.platform = p
				return b.platform, nil
			}
		}
		return nil, errors.Errorf("current platform %q is not found", b.ws.CurrentPlatform)
	}
}

func (b *Bosun) GetPlatform(name string) (*Platform, error) {
	for _, p := range b.file.Platforms {
		if p.Name == name {
			b.platform = p
			return b.platform, nil
		}
	}
	return nil, errors.Errorf("current platform %q is not found", b.ws.CurrentPlatform)
}

func (b *Bosun) GetPlatforms() ([]*Platform, error) {
	out := make([]*Platform, len(b.file.Platforms))
	copy(out, b.file.Platforms)
	mirror.Sort(out, func(a, b *Platform) bool {
		return a.Name < b.Name
	})

	return out, nil
}

func (b *Bosun) UsePlatform(name string) error {
	for _, p := range b.file.Platforms {
		if p.Name == name {
			b.ws.CurrentPlatform = name
			return nil
		}
	}
	return errors.Errorf("no platform named %q", name)
}

func (b *Bosun) UseRelease(name string) error {

	p, err := b.GetCurrentPlatform()
	if err != nil {
		return err
	}
	_, err = p.GetReleaseMetadataByName(name)
	if err != nil {
		return err
	}

	b.ws.CurrentRelease = name
	return nil
}

func (b *Bosun) GetGitRoots() []string {
	return b.ws.GitRoots
}

func (b *Bosun) AddGitRoot(s string) {
	b.ws.GitRoots = append(b.ws.GitRoots, s)
}

// TidyWorkspace updates the ClonePaths in the workspace based on the apps found in the imported files.
func (b *Bosun) TidyWorkspace() {
	ctx := b.NewContext()
	log := ctx.Log
	var importMap = map[string]struct{}{}

	for _, repo := range b.GetRepos() {
		if repo.CheckCloned() != nil {
			for _, root := range b.ws.GitRoots {
				clonedFolder := filepath.Join(root, repo.Name)
				if _, err := os.Stat(clonedFolder); err != nil {
					if os.IsNotExist(err) {
						log.Debugf("Repo %s not found at %s", repo.Name, clonedFolder)
					} else {
						log.Warnf("Error looking for app %s: %s", repo.Name, err)
					}
				}
				bosunFilePath := filepath.Join(clonedFolder, "bosun.yaml")
				if _, err := os.Stat(bosunFilePath); err != nil {
					if os.IsNotExist(err) {
						log.Warnf("Repo %s seems to be cloned to %s, but there is no bosun.yaml file in that folder", repo.Name, clonedFolder)
					} else {
						log.Warnf("Error looking for bosun.yaml in repo %s: %s", repo.Name, err)
					}
				} else {
					log.Infof("Found cloned repo %s at %s, will add to known local repos.", repo.Name, bosunFilePath)
					localRepo := &LocalRepo{
						Name: repo.Name,
						Path: clonedFolder,
					}
					b.AddLocalRepo(localRepo)
					break
				}
			}
		}
	}

	for _, app := range b.GetApps() {
		if app.IsRepoCloned() {
			importMap[app.FromPath] = struct{}{}
			log.Debugf("App %s found at %s", app.Name, app.FromPath)

			repo, err := b.GetRepo(app.RepoName)
			if err != nil || repo.LocalRepo == nil {
				log.Infof("App %s is cloned but its repo is not registered. Registering repo %s...", app.Name, app.RepoName)
				path, err := app.GetLocalRepoPath()
				if err != nil {
					log.WithError(err).Errorf("Error getting local repo path for %s.", app.Name)
				}
				b.AddLocalRepo(&LocalRepo{
					Name: app.RepoName,
					Path: path,
				})
			}

			continue
		}
		log.Debugf("Found app with no cloned repo: %s from %s", app.Name, app.RepoName)
		for _, root := range b.ws.GitRoots {
			clonedFolder := filepath.Join(root, app.RepoName)
			if _, err := os.Stat(clonedFolder); err != nil {
				if os.IsNotExist(err) {
					log.Debugf("App %s not found at %s", app.Name, clonedFolder)
				} else {
					log.Warnf("Error looking for app %s: %s", app.Name, err)
				}
			}
			bosunFilePath := filepath.Join(clonedFolder, "bosun.yaml")
			if _, err := os.Stat(bosunFilePath); err != nil {
				if os.IsNotExist(err) {
					log.Warnf("App %s seems to be cloned to %s, but there is no bosun.yaml file in that folder", app.Name, clonedFolder)
				} else {
					log.Warnf("Error looking for bosun.yaml for app %s: %s", app.Name, err)
				}
			} else {
				log.Infof("Found bosun.yaml for app ref %s at %s, will add to imports.", app.Name, bosunFilePath)
				b.AddImport(bosunFilePath)
				break
			}
		}

	}

	for _, importPath := range b.ws.Imports {
		if _, err := os.Stat(importPath); os.IsNotExist(err) {
			log.Infof("Import path %s points to a file which no longer exists. It will be removed.", importPath)
		} else {
			importMap[importPath] = struct{}{}
		}
	}
	var imports []string
	for k := range importMap {
		imports = append(imports, k)
	}

	b.ws.Imports = imports
}

func (b *Bosun) configureCurrentEnv() error {
	if b.ws.CurrentEnvironment == "" {
		switch len(b.file.Environments) {
		case 0:
			b.log.Warn("No environments found, using a dummy environment.")
			return b.useEnvironment(&EnvironmentConfig{
				Name:     "",
				FromPath: b.ws.Path,
			})
		case 1:
			// if only one environment exists, it's the current one
			b.ws.CurrentEnvironment = b.file.Environments[0].Name
		default:
			var envNames []string
			for _, env := range b.file.Environments {
				envNames = append(envNames, env.Name)
			}
			return errors.Errorf("no environment set (available: %v)", envNames)
		}
	}

	if b.ws.CurrentEnvironment != "" {

		env, err := b.GetEnvironment(b.ws.CurrentEnvironment)
		if err != nil {
			return errors.Errorf("get environment %q: %s", b.ws.CurrentEnvironment, err)
		}

		// set the current environment.
		// this will also set environment vars based on it.
		return b.useEnvironment(env)
	}

	return errors.New("no current environment set in workspace")
}

// Confirm environment checks that the environment has been confirmed by the
// user if the environment is marked as protected.
func (b *Bosun) ConfirmEnvironment() error {

	if !b.env.Protected {
		return nil
	}

	if b.environmentConfirmed == nil {

		envName := b.GetCurrentEnvironment().Name
		if b.params.ConfirmedEnv != "" {
			if b.params.ConfirmedEnv == envName {
				b.environmentConfirmed = to.BoolPtr(true)
			} else {
				return errors.Errorf("The --confirm-env flag was set to %q, but you are targeting the %q environment!\nSwitch environments or unset the flag.", b.params.ConfirmedEnv, b.env.Name)

			}
		}

		confirmed := pkg.RequestConfirmFromUser("Do you really want to run this command against the %q environment?", envName)
		b.environmentConfirmed = &confirmed
	}

	if *b.environmentConfirmed {
		return nil
	}

	return errors.Errorf("The %q environment is protected, so you must confirm that you want to perform this action.\n(you can do this by setting the --confirm-env to the name of the environment)", b.env.Name)
}

func (b *Bosun) GetTools() []*ToolDef {
	return b.ws.MergedBosunFile.Tools
}
func (b *Bosun) GetTool(name string) (ToolDef, error) {
	for _, tool := range b.ws.MergedBosunFile.Tools {
		if tool.Name == name {
			return *tool, nil
		}
	}
	return ToolDef{}, errors.Errorf("no tool named %q is known", name)
}

func (b *Bosun) RequireTool(name string) error {
	tool, err := b.GetTool(name)
	if err != nil {
		return err
	}

	if _, err := tool.GetExecutable(); err != nil {
		return errors.Wrapf(err, "required tool %q is not installed", name)
	}
	return nil
}

func (b *Bosun) EnsureTool(name string) error {
	tool, err := b.GetTool(name)
	if err != nil {
		return err
	}

	if _, err := tool.GetExecutable(); err == nil {
		return nil
	}

	installer, err := tool.GetInstaller()
	if err != nil {
		return errors.Errorf("required tool %q is not installable: %s", name, err)
	}
	ctx := b.NewContext()

	err = installer.Execute(ctx)
	return err
}

func (b *Bosun) GetTestSuiteConfigs() []*E2ESuiteConfig {
	return b.ws.MergedBosunFile.TestSuites
}

func (b *Bosun) GetTestSuite(name string) (*E2ESuite, error) {
	var suite *E2ESuite
	var err error
	for _, c := range b.GetTestSuiteConfigs() {
		if c.Name == name {
			ctx := b.NewContext()
			suite, err = NewE2ESuite(ctx, c)
			return suite, err
		}
	}

	return nil, errors.Errorf("no test suite found with name %q", name)
}

func (b *Bosun) GetRepo(name string) (*Repo, error) {
	repos := b.GetRepos()
	for _, repo := range repos {
		if repo.Name == name {
			return repo, nil
		}
	}
	return nil, errors.Errorf("no repo with name %q", name)
}

func (b *Bosun) GetRepos() []*Repo {

	if len(b.repos) == 0 {
		b.repos = map[string]*Repo{}
		for _, repoConfig := range b.ws.MergedBosunFile.Repos {
			for _, app := range b.ws.MergedBosunFile.Apps {
				if app.RepoName == repoConfig.Name {
					var repo *Repo
					var ok bool
					if repo, ok = b.repos[repoConfig.Name]; !ok {
						repo = &Repo{
							RepoConfig: *repoConfig,
							Apps:       map[string]*AppConfig{},
						}
						if lr, ok := b.ws.LocalRepos[repo.Name]; ok {
							repo.LocalRepo = lr
						}
						b.repos[repo.Name] = repo
					}
					repo.Apps[app.Name] = app
				}
			}
		}

	}

	var names []string
	for name := range b.repos {
		names = append(names, name)
	}

	sort.Strings(names)

	var out []*Repo

	for _, name := range names {
		out = append(out, b.repos[name])
	}

	return out
}

func (b *Bosun) AddLocalRepo(localRepo *LocalRepo) {
	if b.ws.LocalRepos == nil {
		b.ws.LocalRepos = map[string]*LocalRepo{}
	}
	b.ws.LocalRepos[localRepo.Name] = localRepo

	if repo, ok := b.repos[localRepo.Name]; ok {
		repo.LocalRepo = localRepo
	}
}

func (b *Bosun) GetIssueService() (issues.IssueService, error) {

	p, err := b.GetCurrentPlatform()
	if err != nil {
		return nil, nil, errors.Wrap(err, "get current platform")
	}
	if p.ZenHubConfig == nil {
		p.ZenHubConfig = &zenhub.Config{
			StoryBoardName: "Release Planning",
			TaskBoardName:  "Development",
		}
	}
	zc := *p.ZenHubConfig

	zc.GithubToken, err = b.GetGithubToken()
	if err != nil {
		return nil, nil, err
	}

	gc := &git.Config{
		GithubToken: zc.GithubToken,
	}

	zc.ZenhubToken, err = b.GetZenhubToken()
	if err != nil {
		return nil, errors.Wrap(err, "get zenhub token")
	}

	repoPath, err := git.GetCurrentRepoPath()
	if err != nil {
		return nil, err
	}

	g, err := git.NewGitWrapper(repoPath)
	if err != nil {
		return nil, err
	}

	gis, err := git.NewIssueService(*gc, g, pkg.Log.WithField("cmp", "github"))
	if err != nil {
		return nil, errors.Wrapf(err, "get story service with tokens %q, %q", zc.GithubToken, zc.ZenhubToken)
	}

	svc, err := zenhub.NewIssueService(gis, zc, pkg.Log.WithField("cmp", "zenhub"))
	if err != nil {
		return nil, nil,errors.Wrapf(err, "get story service with tokens %q, %q", zc.GithubToken, zc.ZenhubToken)
	}
	return gis, svc, nil

}

func (b *Bosun) GetZenhubToken() (string, error) {
	// b := cmd.MustGetBosun()
	ws := b.GetWorkspace()
	ctx := b.NewContext().WithDir(ws.Path)
	if ws.ZenhubToken == nil {
		fmt.Println("Zenhub token was not found. Please generate a new Zenhub token. https://app.zenhub.com/dashboard/tokens")
		fmt.Println(`Simple example: echo "9uha09h39oenhsir98snegcu"`)
		fmt.Println(`Better example: cat $HOME/.tokens/zenhub.token"`)
		fmt.Println(`Secure example: lpass show "Tokens/GithubCLIForBosun" --notes"`)
		script := pkg.RequestStringFromUser("Command")

		ws.ZenhubToken = &CommandValue{
			Command: Command{
				Script: script,
			},
		}

		_, err := ws.ZenhubToken.Resolve(ctx)
		if err != nil {
			return "", errors.Errorf("script failed: %s\nscript:\n%s", err, script)
		}

		err = b.Save()
		if err != nil {
			return "", errors.Errorf("save failed: %s", err)
		}
	}

	token, err := ws.ZenhubToken.Resolve(ctx)
	if err != nil {
		return "", err
	}

	err = os.Setenv("ZENHUB_TOKEN", token)
	if err != nil {
		return "", err
	}

	token, ok := os.LookupEnv("ZENHUB_TOKEN")
	if !ok {
		return "", errors.Errorf("ZENHUB_TOKEN must be set")
	}

	return token, nil
}

func (b *Bosun) GetGithubToken() (string, error) {
	ws := b.ws
	var err error

	token, ok := os.LookupEnv("GITHUB_TOKEN")
	if !ok {

		ctx := b.NewContext().WithDir(ws.Path)
		if ws.GithubToken == nil {
			fmt.Println("Github token was not found. Please provide a command that can be run to obtain a github token.")
			fmt.Println(`Simple example: echo "9uha09h39oenhsir98snegcu"`)
			fmt.Println(`Better example: cat $HOME/.tokens/github.token"`)
			fmt.Println(`Secure example: lpass show "Tokens/GithubCLIForBosun" --notes"`)
			script := pkg.RequestStringFromUser("Command")

			ws.GithubToken = &CommandValue{
				Command: Command{
					Script: script,
				},
			}

			_, err := ws.GithubToken.Resolve(ctx)
			if err != nil {
				return "", errors.Errorf("script failed: %s\nscript:\n%s", err, script)
			}

			err = b.Save()
			if err != nil {
				return "", errors.Errorf("save failed: %s", err)
			}
		}

		token, err = ws.GithubToken.Resolve(ctx)
		if err != nil {
			return "", err
		}

		err = os.Setenv("GITHUB_TOKEN", token)
		if err != nil {
			return "", err
		}
	}

	return token, nil
}
