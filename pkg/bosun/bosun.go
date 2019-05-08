package bosun

import (
	"context"
	"fmt"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/google/go-github/v20/github"
	vault "github.com/hashicorp/vault/api"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

type Bosun struct {
	params               Parameters
	ws                   *Workspace
	file                 *File
	apps                 map[string]*App
	release              *Release
	vaultClient          *vault.Client
	env                  *EnvironmentConfig
	clusterAvailable     *bool
	log                  *logrus.Entry
	environmentConfirmed *bool
	repos                map[string]*Repo
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
			b.addApp(a)
		}
	}

	if !params.NoCurrentEnv {
		b.configureCurrentEnv()
	}

	return b, nil
}

func (b *Bosun) addApp(config *AppConfig) *App {
	app := NewApp(config)
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
			config.Fragment.Repos = append(config.Fragment.Repos, repo.RepoConfig)
		}
		app.Repo = repo
	}

	b.apps[config.Name] = app

	for _, d2 := range app.DependsOn {
		if _, ok := b.apps[d2.Name]; !ok {
			b.apps[d2.Name] = NewAppFromDependency(&d2)
		}
	}

	return app
}

func (b *Bosun) GetAppsSortedByName() []*App {
	var ms AppsSortedByName

	for _, x := range b.apps {
		ms = append(ms, x)
	}
	sort.Sort(ms)
	return ms
}

func (b *Bosun) GetApps() map[string]*App {
	return b.apps
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

func (b *Bosun) GetApp(name string) (*App, error) {
	m, ok := b.apps[name]
	if !ok {
		return nil, errors.Errorf("no service named %q", name)
	}
	return m, nil
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
		b.addApp(m)
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

func (b *Bosun) NewContext() BosunContext {

	dir, _ := os.Getwd()

	return BosunContext{
		Bosun: b,
		Env:   b.GetCurrentEnvironment(),
		Log:   b.log,
	}.WithDir(dir).WithContext(context.Background())

}

func (b *Bosun) GetCurrentRelease() (*Release, error) {
	var err error
	if b.release == nil {
		if b.ws.Release == "" {
			return nil, errors.New("current release not set, call `bosun release use {name}` to set current release")
		}
		if b.ws.Release != "" {
			for _, r := range b.file.Releases {
				if r.Name == b.ws.Release {
					b.release, err = NewRelease(b.NewContext(), r)
					if err != nil {
						return nil, errors.Errorf("creating release from config %q: %s", r.Name, err)
					}
				}
			}
		}
	}
	if b.release == nil {
		return nil, errors.Errorf("current release %q could not be found, call `bosun release use {name}` to set current release", b.ws.Release)
	}

	return b.release, nil
}

func (b *Bosun) GetReleaseConfigs() []*ReleaseConfig {
	var releases []*ReleaseConfig
	for _, r := range b.file.Releases {
		releases = append(releases, r)
	}
	return releases
}

func (b *Bosun) GetReleaseConfig(name string) (*ReleaseConfig, error) {
	for _, r := range b.file.Releases {
		if r.Name == name {
			return r, nil
		}
	}
	return nil, errors.Errorf("no release with name %q", name)
}

func (b *Bosun) UseRelease(name string) error {

	rc, err := b.GetReleaseConfig(name)
	if err != nil {
		return err
	}

	b.release, err = NewRelease(b.NewContext(), rc)
	if err != nil {
		return err
	}

	b.ws.Release = name
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
				log.Infof("App %s is cloned but its repo is not registered. Registering repo %s...", app.Name, app.Repo)
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
		log.Debugf("Found app with no cloned repo: %s from %s", app.Name, app.Repo)
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
	// if only one environment exists, it's the current one
	if b.ws.CurrentEnvironment == "" {
		if len(b.file.Environments) == 1 {
			b.ws.CurrentEnvironment = b.file.Environments[0].Name
		} else {
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

func (b *Bosun) GetTools() []ToolDef {
	return b.ws.MergedBosunFile.Tools
}
func (b *Bosun) GetTool(name string) (ToolDef, error) {
	for _, tool := range b.ws.MergedBosunFile.Tools {
		if tool.Name == name {
			return tool, nil
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
							RepoConfig: repoConfig,
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
