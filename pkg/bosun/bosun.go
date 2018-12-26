package bosun

import (
	"context"
	"fmt"
	"github.com/google/go-github/github"
	vault "github.com/hashicorp/vault/api"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"os/exec"
	"sort"
	"time"
)

type Bosun struct {
	params           Parameters
	config           *Config
	mergedFragments  *ConfigFragment
	apps             map[string]*App
	vaultClient      *vault.Client
	env              *EnvironmentConfig
	clusterAvailable *bool
}

type Parameters struct {
	Verbose bool
	DryRun  bool
	CIMode  bool
	Force   bool
	ValueOverrides map[string]string
}

func New(params Parameters, config *Config) (*Bosun, error) {
	b := &Bosun{
		params:          params,
		config:          config,
		mergedFragments: config.MergedFragments,
		apps:            make(map[string]*App),
	}

	for _, dep := range b.mergedFragments.AppRefs {
		b.apps[dep.Name] = NewAppFromDependency(dep)
	}

	for _, a := range b.mergedFragments.Apps {
		if a != nil {
			b.addApp(a)
		}
	}

	for _, r := range b.mergedFragments.Releases {
		for _, a := range r.Apps {
			if app, ok := b.apps[a.Name]; ok {
				a.App = app
			}
		}
	}

	// if only one environment exists, it's the current one
	if config.CurrentEnvironment == "" && len(b.mergedFragments.Environments) == 1 {
		config.CurrentEnvironment = b.mergedFragments.Environments[0].Name
	}

	if config.CurrentEnvironment != "" {

		env, err := b.GetEnvironment(config.CurrentEnvironment)
		if err != nil {
			return nil, errors.Errorf("get environment %q: %s", config.CurrentEnvironment, err)
		}

		// set the current environment.
		// this will also set environment vars based on it.
		err = b.useEnvironment(env)
	}

	ctx := b.NewContext("")

	// now that environment variables are all set
	// we'll resolve all the paths in all our apps
	for _, app := range b.apps {
		app.ConfigureForEnvironment(ctx.WithDir(app.FromPath))
	}

	return b, nil
}

func (b *Bosun) addApp(config *AppConfig) *App {
	app := NewApp(config)
	b.apps[config.Name] = app
	appStates := b.config.AppStates[b.config.CurrentEnvironment]
	app.DesiredState = appStates[config.Name]

	for _, d2 := range app.DependsOn {
		if _, ok := b.apps[d2.Name]; !ok {
			b.apps[d2.Name] = NewAppFromDependency(&d2)
		}
	}

	return app
}

func (b *Bosun) GetAppsSortedByName() AppsSortedByName {
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

	err := b.config.importFragmentFromPath(path)

	if err != nil {
		return nil, err
	}

	pkg.Log.WithField("path", path).Debug("New microservice found at path.")

	imported := b.config.ImportedFragments[path]

	var name string
	for _, m := range imported.Apps {
		b.addApp(m)
		name = m.Name
	}

	m, _ := b.GetApp(name)
	return m, nil
}

func (b *Bosun) useEnvironment(env *EnvironmentConfig) error {

	b.config.CurrentEnvironment = env.Name
	b.env = env

	err := b.env.Ensure(b.NewContext(""))
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
		panic(errors.Errorf("environment not initialized; current environment is %s", b.config.CurrentEnvironment))
	}

	return b.env
}

func (b *Bosun) Save() error {

	config := b.config

	if config.AppStates == nil {
		config.AppStates = AppStatesByEnvironment{}
	}

	env := b.env

	appStates := AppStateMap{}
	for _, app := range b.apps {
		appStates[app.Name] = app.DesiredState
	}

	config.AppStates[env.Name] = appStates

	data, err := yaml.Marshal(config)
	if err != nil {
		return errors.Wrap(err, "marshalling for save")
	}

	err = ioutil.WriteFile(config.Path, data, 0700)
	if err != nil {
		return errors.Wrap(err, "writing for save")
	}

	return nil
}

func (b *Bosun) GetMergedConfig() ConfigFragment {

	return *b.mergedFragments

}

func (b *Bosun) AddImport(file string) bool {
	for _, i := range b.config.Imports {
		if i == file {
			return false
		}
	}
	b.config.Imports = append(b.config.Imports, file)
	return true
}

func (b *Bosun) IsClusterAvailable() bool {
	env := b.GetCurrentEnvironment()
	if b.clusterAvailable == nil {
		pkg.Log.Debugf("Checking if cluster %q is available...", env.Cluster)
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
			pkg.Log.Debugf("Cluster is available: %t", result)
		case <-time.After(2 * time.Second):
			pkg.Log.Warn("Cluster did not respond quickly, I will assume it is unavailable.")
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			b.clusterAvailable = github.Bool(false)
		}
	}

	return *b.clusterAvailable
}

func (b *Bosun) GetEnvironment(name string) (*EnvironmentConfig, error) {
	for _, env := range b.mergedFragments.Environments {
		if env.Name == name {
			return env, nil
		}
	}
	return nil, errors.Errorf("no environment named %q", name)
}

func (b *Bosun) NewContext(dir string) BosunContext {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return BosunContext{
		Bosun: b,
		Env:   b.GetCurrentEnvironment(),
		Log:   pkg.Log,
	}.WithDir(dir).WithContext(context.Background())

}

func (b *Bosun) GetCurrentRelease() (*Release, error) {
	if b.config.Release == "" {
		return nil, errors.New("current release not set, call `bosun release use {name}` to set current release")
	}
	return b.GetRelease(b.config.Release)
}

func (b *Bosun) GetRelease(name string) (*Release, error) {
	for _, e := range b.mergedFragments.Releases {
		if e.Name == name {
			return e, nil
		}
	}
	return nil, errors.Errorf("no release with name %q", name)

}

func (b *Bosun) GetReleases() []*Release {
	var releases []*Release
	for _, r := range b.mergedFragments.Releases {
		releases = append(releases, r)
	}
	return releases
}

func (b *Bosun) UseRelease(name string) error {
	_, err := b.GetRelease(name)
	if err != nil {
		return err
	}
	b.config.Release = name
	return nil
}

func (b *Bosun) GetGitRoots() []string {
	return b.config.GitRoots
}

func (b *Bosun) AddGitRoot(s string) {
	b.config.GitRoots = append(b.config.GitRoots, s)
}
