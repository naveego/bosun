package bosun

import (
	"context"
	"fmt"
	"github.com/google/go-github/github"
	vault "github.com/hashicorp/vault/api"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/git"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"os/exec"
	"sort"
	"time"
)

type Bosun struct {
	params           Parameters
	ws               *Workspace
	file             *File
	repos            map[string]*AppRepo
	release          *Release
	vaultClient      *vault.Client
	env              *EnvironmentConfig
	clusterAvailable *bool
	log              *logrus.Entry
}

type Parameters struct {
	Verbose        bool
	DryRun         bool
	Force          bool
	NoReport       bool
	ForceTests     bool
	ValueOverrides map[string]string
	FileOverrides  []string
}

func New(params Parameters, ws *Workspace) (*Bosun, error) {
	var err error
	b := &Bosun{
		params: params,
		ws:     ws,
		file:   ws.MergedBosunFile,
		repos:  make(map[string]*AppRepo),
		log:    pkg.Log,
	}

	if params.DryRun {
		b.log = b.log.WithField("*DRYRUN*", "")
		b.log.Info("DRY RUN")
	}

	for _, dep := range b.file.AppRefs {
		b.repos[dep.Name] = NewRepoFromDependency(dep)
	}

	for _, a := range b.file.Apps {
		if a != nil {
			b.addApp(a)
		}
	}

	// if only one environment exists, it's the current one
	if ws.CurrentEnvironment == "" {
		if len(b.file.Environments) == 1 {
			ws.CurrentEnvironment = b.file.Environments[0].Name
		} else {
			var envNames []string
			for _, env := range b.file.Environments {
				envNames = append(envNames, env.Name)
			}
			return nil, errors.Errorf("no environment set (available: %v)", envNames)
		}
	}

	if ws.CurrentEnvironment != "" {

		env, err := b.GetEnvironment(ws.CurrentEnvironment)
		if err != nil {
			return nil, errors.Errorf("get environment %q: %s", ws.CurrentEnvironment, err)
		}

		// set the current environment.
		// this will also set environment vars based on it.
		err = b.useEnvironment(env)
		if err != nil {
			return nil, err
		}
	}

	for _, r := range b.file.Releases {
		if r.Name == b.ws.Release {
			b.release, err = NewRelease(b.NewContext(), r)
			if err != nil {
				return nil, errors.Errorf("creating release from config %q: %s", r.Name, err)
			}
		}
	}

	return b, nil
}

func (b *Bosun) addApp(config *AppRepoConfig) *AppRepo {
	app := NewApp(config)
	b.repos[config.Name] = app

	for _, d2 := range app.DependsOn {
		if _, ok := b.repos[d2.Name]; !ok {
			b.repos[d2.Name] = NewRepoFromDependency(&d2)
		}
	}

	return app
}

func (b *Bosun) GetAppsSortedByName() ReposSortedByName {
	var ms ReposSortedByName

	for _, x := range b.repos {
		ms = append(ms, x)
	}
	sort.Sort(ms)
	return ms
}

func (b *Bosun) GetApps() map[string]*AppRepo {
	return b.repos
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

func (b *Bosun) GetApp(name string) (*AppRepo, error) {
	m, ok := b.repos[name]
	if !ok {
		return nil, errors.Errorf("no service named %q", name)
	}
	return m, nil
}

func (b *Bosun) GetOrAddAppForPath(path string) (*AppRepo, error) {
	for _, m := range b.repos {
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
		panic(errors.Errorf("environment not initialized; current environment is %s", b.ws.CurrentEnvironment))
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

	err = ioutil.WriteFile(ws.Path, data, 0700)
	if err != nil {
		return errors.Wrap(err, "writing for save")
	}

	return nil
}

func (b *Bosun) Getworkspace() Workspace {
	return *b.ws
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
	if b.release == nil {
		return nil, errors.New("current release not set, call `bosun release use {name}` to set current release")
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

func (b *Bosun) UseRelease(name string) error {
	var rc *ReleaseConfig
	var err error
	for _, rc = range b.file.Releases {
		if rc.Name == name {
			break
		}
	}
	if rc == nil {
		return errors.Errorf("no release with name %q", name)
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


// SyncClonePaths updates the ClonePaths in the workspace based on the apps found in the imported files.
func (b *Bosun) SyncClonePaths() {
	clonePaths := make(map[string]string)
	apps := b.GetApps()
	for _, app := range apps {
		path, _ := git.GetRepoPath( app.FromPath)
		clonePaths[app.Repo] = path
	}
}