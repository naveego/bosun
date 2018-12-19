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
	rootConfig       *RootConfig
	config           *Config
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

func New(params Parameters, rootConfig *RootConfig) (*Bosun, error) {
	b := &Bosun{
		params:     params,
		rootConfig: rootConfig,
		config:     rootConfig.MergedConfig,
		apps:       make(map[string]*App),
	}

	for _, a := range b.config.Apps {
		b.addApp(a)
	}

	// if only one environment exists, it's the current one
	if rootConfig.CurrentEnvironment == "" && len(b.config.Environments) == 1 {
		rootConfig.CurrentEnvironment = b.config.Environments[0].Name
	}

	if rootConfig.CurrentEnvironment != "" {

		env, err := b.GetEnvironment(rootConfig.CurrentEnvironment)
		if err != nil {
			return nil, errors.Errorf("get environment %q: %s", rootConfig.CurrentEnvironment, err)
		}

		// set the current environment.
		// this will also set environment vars based on it.
		err = b.setCurrentEnvironment(env)
	}

	ctx := b.NewContext("")

	// now that environment variables are all set
	// we'll resolve all the paths in all our apps
	for _, app := range b.apps {
		app.ConfigureForEnvironment(ctx.ForDir(app.FromPath))
	}

	return b, nil
}

func (b *Bosun) addApp(config *AppConfig) *App {
	ms := &App{
		bosun:     b,
		AppConfig: *config,
	}
	b.apps[config.Name] = ms
	appStates := b.rootConfig.AppStates[b.rootConfig.CurrentEnvironment]
	ms.DesiredState = appStates[config.Name]
	return ms
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

	err := b.rootConfig.importFromPath(path)

	if err != nil {
		return nil, err
	}

	pkg.Log.WithField("path", path).Debug("New microservice found at path.")

	imported := b.rootConfig.ImportedConfigs[path]

	var name string
	for _, m := range imported.Apps {
		b.addApp(m)
		name = m.Name
	}

	m, _ := b.GetApp(name)
	return m, nil
}

func (b *Bosun) setCurrentEnvironment(env *EnvironmentConfig) error {

	b.rootConfig.CurrentEnvironment = env.Name
	b.env = env

	err := b.env.Ensure(b.NewContext(""))
	if err != nil {
		return errors.Errorf("ensure environment %q: %s", b.env.Name, err)
	}

	return nil
}

func (b *Bosun) SetCurrentEnvironment(name string) error {

	env, err := b.GetEnvironment(name)
	if err != nil {
		return err
	}

	return b.setCurrentEnvironment(env)
}

func (b *Bosun) GetCurrentEnvironment() *EnvironmentConfig {
	if b.env == nil {
		panic(errors.Errorf("environment not initialized; current environment is %s", b.rootConfig.CurrentEnvironment))
	}

	return b.env
}

func (b *Bosun) Save() error {

	config := b.rootConfig

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

func (b *Bosun) GetMergedConfig() Config {

	return *b.config

}

func (b *Bosun) AddImport(file string) bool {
	for _, i := range b.rootConfig.Imports {
		if i == file {
			return false
		}
	}
	b.rootConfig.Imports = append(b.rootConfig.Imports, file)
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

	err := app.LoadActualState(true)
	if err != nil {
		return errors.Errorf("error checking actual state for %q: %s", app.Name, err)
	}

	env := b.GetCurrentEnvironment()
	reportDeploy :=
		b.params.CIMode &&
			env.IsLocal &&
			app.DesiredState.Status == StatusDeployed &&
			!b.params.DryRun

	ctx := b.NewContext(app.FromPath)

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
		log.WithField("step", step.Description).Info("Planned step.")
	}

	log.Info("Planning complete.")

	log.Debug("Executing plan...")

	for _, step := range plan {
		pkg.Log.WithField("name", step.Description).Info("Executing step...")
		err := step.Action(ctx)
		if err != nil {
			return err
		}
		pkg.Log.WithField("name", step.Description).Info("Step complete.")
	}

	log.Debug("Plan executed.")

	return nil
}

func (b *Bosun) GetEnvironment(name string) (*EnvironmentConfig, error) {
	for _, env := range b.config.Environments {
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
		Ctx:   context.Background(),
	}.ForDir(dir)
}
