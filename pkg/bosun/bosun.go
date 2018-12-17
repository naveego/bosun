package bosun

import (
	"github.com/google/go-github/github"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type Bosun struct {
	params           Parameters
	rootConfig       *RootConfig
	config           *Config
	apps             map[string]*App
	clusterAvailable *bool
}

type Parameters struct {
	Verbose bool
	DryRun  bool
}

func New(params Parameters, rootConfig *RootConfig) *Bosun {


	b := &Bosun{
		params:     params,
		rootConfig: rootConfig,
		config:     rootConfig.MergedConfig,
		apps:       make(map[string]*App),
	}

	for _, a := range b.config.Apps {
		b.addApp(a)
	}

	return b
}

func (b *Bosun) addApp(config *AppConfig) *App {
	ms := &App{
		bosun:  b,
		AppConfig: *config,
	}
	b.apps[config.Name] = ms
	ms.DesiredState = b.rootConfig.AppStates[config.Name]
	return ms
}

func (b *Bosun) GetApps() []*App {
	var ms AppSlice

	for _, x := range b.apps {
		ms = append(ms, x)
	}
	sort.Sort(ms)
	return ms
}

type AppSlice []*App

func (a AppSlice) Len() int {
	return len(a)
}

func (a AppSlice) Less(i, j int) bool {
	return strings.Compare(a[i].Name, a[j].Name) < 0
}

func (a AppSlice) Swap(i, j int) {
	a[i], a[j]= a[j],a[i]
}

func (b *Bosun) GetScripts() ([]*Script, error) {
	env, err := b.GetCurrentEnvironment()
	if err != nil {
		return nil, err
	}

	return env.Scripts, nil
}

func (b *Bosun) GetScript(name string) (*Script, error) {
	env, err := b.GetCurrentEnvironment()
	if err != nil {
		return nil, err
	}
	for _, s := range env.Scripts {
		if s.Name == name {
			return s, nil
		}
	}

	return nil, errors.Errorf("no script in environment %q with name %q", env.Name, name)
}

func (b *Bosun) GetMicroservice(name string) (*App, error) {
	m, ok := b.apps[name]
	if !ok {
		return nil, errors.Errorf("no service named %q", name)
	}
	return m, nil
}

func (b *Bosun) GetOrAddMicroserviceForPath(path string) (*App, error) {
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

	err = b.Save()
	if err != nil {
		return nil, err
	}

	m, _ := b.GetMicroservice(name)
	return m, nil
}

func (b *Bosun) SetCurrentEnvironment(name string) error {

	for _, env := range b.config.Environments {
		if env.Name == name {
			b.rootConfig.CurrentEnvironment = name
			return nil
		}
	}

	return errors.Errorf("environment %q does not exist", name)
}

func (b *Bosun) GetCurrentEnvironment() (*EnvironmentConfig, error) {

	for _, env := range b.config.Environments {
		if env.Name == b.rootConfig.CurrentEnvironment {
			return env, nil
		}
	}

	return nil, errors.Errorf("current environment %q does not exist", b.rootConfig.CurrentEnvironment)
}


func (b *Bosun) Save() error {

	config := b.rootConfig

	config.AppStates = make(map[string]AppState)
	for _, app := range b.apps {
		config.AppStates[app.Name] = app.DesiredState
	}

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

func (b *Bosun) GetMergedConfig() string {

	data, _ := yaml.Marshal(b.config)

	return string(data)

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
	if b.clusterAvailable == nil {
		pkg.Log.Debug("Checking if cluster is available...")
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
	err := app.LoadActualState(true)
	if err != nil {
		return errors.Errorf("error checking actual state for %q: %s", app.Name, err)
	}

	log := pkg.Log.WithField("app", app.Name)
	log.Debug("Planning reconciliation...")

	plan, err := app.PlanReconciliation()

	if err != nil {
		return err
	}

	for _, step := range plan {
		log.WithField("step", step.Description).Info("Planned step.")
	}

	log.Debug("Executing...")
	err = plan.Execute()
	if err != nil {
		return err
	}

	return nil
}