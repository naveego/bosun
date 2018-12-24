package bosun

import (
	"context"
	"fmt"
	"github.com/google/go-github/github"
	vault "github.com/hashicorp/vault/api"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/git"
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
	deps             map[string]*Dependency
	apps             map[string]*App
	vaultClient      *vault.Client
	env              *EnvironmentConfig
	clusterAvailable *bool
}

type Parameters struct {
	Verbose bool
	DryRun  bool
	CIMode  bool
}

func New(params Parameters, rootConfig *Config) (*Bosun, error) {
	b := &Bosun{
		params:          params,
		config:          rootConfig,
		mergedFragments: rootConfig.MergedFragments,
		apps:            make(map[string]*App),
		deps:            make(map[string]*Dependency),
	}

	for _, dep := range b.mergedFragments.AppRefs {
		b.deps[dep.Name] = dep
	}

	for _, a := range b.mergedFragments.Apps {
		b.addApp(a)
	}

	// if only one environment exists, it's the current one
	if rootConfig.CurrentEnvironment == "" && len(b.mergedFragments.Environments) == 1 {
		rootConfig.CurrentEnvironment = b.mergedFragments.Environments[0].Name
	}

	if rootConfig.CurrentEnvironment != "" {

		env, err := b.GetEnvironment(rootConfig.CurrentEnvironment)
		if err != nil {
			return nil, errors.Errorf("get environment %q: %s", rootConfig.CurrentEnvironment, err)
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

	dep, ok := b.deps[config.Name]
	if !ok {
		dep = &Dependency{Name:config.Name, Repo:config.Repo}
		b.deps[config.Name] = dep
	}

	dep.App = app

	for _, d2 := range app.DependsOn {
		if _, ok := b.deps[d2.Name]; !ok {
			b.deps[d2.Name] = &d2
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

func (b *Bosun) GetDeps() Dependencies {
	var deps Dependencies
	for _, dep := range b.deps {
		deps = append(deps, *dep)
	}
	sort.Sort(deps)
	return deps
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

func (b *Bosun) Reconcile(app *App) error {
	log := pkg.Log.WithField("app", app.Name)

	if !app.HasChart() {
		log.Info("No chart defined for this app.")
		return nil
	}

	ctx := b.NewContext(app.FromPath).WithLog(log)

	err := app.LoadActualState(true, ctx)
	if err != nil {
		return errors.Errorf("error checking actual state for %q: %s", app.Name, err)
	}

	env := b.GetCurrentEnvironment()
	reportDeploy :=
		b.params.CIMode &&
			env.IsLocal &&
			app.DesiredState.Status == StatusDeployed &&
			!b.params.DryRun

	values, err := app.GetValuesMap(ctx)
	if err != nil {
		return errors.Errorf( "create values map for app %q: %s", app.Name, err)
	}

	ctx = ctx.WithValues(values)


	log.Info("Planning reconciliation...")

	plan, err := app.PlanReconciliation(ctx)

	if err != nil {
		return err
	}

	if len(plan) == 0 {
		log.Info("No actions needed to reconcile state.")
		return nil
	}

	if reportDeploy {
		log.Info("Deploy progress will be reported to github.")
		// create the deployment
		deployID, err := git.CreateDeploy(ctx.Dir, env.Name)

		// ensure that the deployment is updated when we return.
		defer func() {
			if err != nil {
				git.UpdateDeploy(ctx.Dir, deployID, "failure")
			} else {
				git.UpdateDeploy(ctx.Dir, deployID, "success")
			}
		}()

		if err != nil {
			return err
		}
	}

	for _, step := range plan {
		log.WithField("step", step.Name).WithField("description", step.Description).Info("Planned step.")
	}

	log.Info("Planning complete.")

	log.Debug("Executing plan...")

	for _, step := range plan {
		stepCtx := ctx.WithLog(log.WithField("step", step.Name))
		stepCtx.Log.Info("Executing step...")
		err := step.Action(stepCtx)
		if err != nil {
			return err
		}
		stepCtx.Log.Info("Step complete.")
	}

	log.Debug("Plan executed.")

	return nil
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
		Log: pkg.Log,
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
