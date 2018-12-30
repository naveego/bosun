package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"strings"
)

type AppRepoConfig struct {
	Name             string                 `yaml:"name"`
	FromPath         string                 `yaml:"-"`
	BranchForRelease bool                   `yaml:"branchForRelease,omitempty"`
	ReportDeployment bool                   `yaml:"reportDeployment,omitempty"`
	Namespace        string                 `yaml:"namespace,omitempty"`
	Repo             string                 `yaml:"repo,omitempty"`
	RepoPath         string                 `yaml:"repoPath,omitempty"`
	HarborProject    string                 `yaml:"harborProject,omitempty"`
	Version          string                 `yaml:"version,omitempty"`
	Chart            string                 `yaml:"chart,omitempty"`
	ChartPath        string                 `yaml:"chartPath,omitempty"`
	RunCommand       []string               `yaml:"runCommand,omitempty"`
	DependsOn        []Dependency           `yaml:"dependsOn,omitempty"`
	Labels           []string               `yaml:"labels,omitempty"`
	Values           AppValuesByEnvironment `yaml:"values,omitempty"`
	Scripts          []*Script              `yaml:"scripts,omitempty"`
	Actions          []*AppAction           `yaml:"actions,omitempty"`
	Fragment         *ConfigFragment        `yaml:"-"`
}

type Dependency struct {
	Name     string   `yaml:"name"`
	FromPath string   `yaml:"-"`
	Repo     string   `yaml:"repo,omitempty"`
	App      *AppRepo `yaml:"-"`
	Version  string   `yaml:"version,omitempty"`
}

type Dependencies []Dependency

func (d Dependencies) Len() int           { return len(d) }
func (d Dependencies) Less(i, j int) bool { return strings.Compare(d[i].Name, d[j].Name) < 0 }
func (d Dependencies) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }

type AppValuesConfig struct {
	Set     map[string]*DynamicValue `yaml:"set,omitempty"`
	Dynamic map[string]*DynamicValue `yaml:"dynamic,omitempty"`
	Files   []string                 `yaml:"files,omitempty"`
	Static  Values                   `yaml:"static"`
}

func (a *AppRepoConfig) SetFragment(fragment *ConfigFragment) {
	a.FromPath = fragment.FromPath
	a.Fragment = fragment
	for i := range a.Scripts {
		a.Scripts[i].FromPath = a.FromPath
	}
	for i := range a.DependsOn {
		a.DependsOn[i].FromPath = a.FromPath
	}
}

// Combine returns a new AppValuesConfig with the values from
// other added after (and/or overwriting) the values from this instance)
func (a AppValuesConfig) Combine(other AppValuesConfig) AppValuesConfig {
	out := AppValuesConfig{
		Set: make(map[string]*DynamicValue),
	}
	out.Files = append(out.Files, a.Files...)
	out.Files = append(out.Files, other.Files...)

	for k, v := range a.Set {
		out.Set[k] = v
	}
	for k, v := range other.Set {
		out.Set[k] = v
	}

	return out
}

type AppValuesByEnvironment map[string]AppValuesConfig

func (a AppValuesByEnvironment) GetValuesConfig(ctx BosunContext) AppValuesConfig {
	out := AppValuesConfig{}
	name := ctx.Env.Name

	// more precise values should override less precise values
	priorities := make([][]AppValuesConfig, 10, 10)

	for k, v := range a {
		keys := strings.Split(k, ",")
		for _, k2 := range keys {
			if k2 == name {
				priorities[len(keys)] = append(priorities[len(keys)], v)
			}
		}
	}

	for i := len(priorities) - 1; i >= 0; i-- {
		for _, v := range priorities[i] {
			out = out.Combine(v)
		}
	}

	return out
}

// LoadFiles resolves all file system dependencies into static values
// on this instance, then clears those dependencies.
func (a AppValuesConfig) LoadFiles(ctx BosunContext) error {

	if a.Static == nil {
		a.Static = Values{}
	}

	for _, file := range a.Files {
		file = ctx.ResolvePath(file)
		valuesFromFile, err := ReadValuesFile(file)
		if err != nil {
			return errors.Errorf("reading values file %q for env key %q: %s", file, ctx.Env.Name, err)
		}
		// make sure any existing static values are merged OVER the values from the file
		static := valuesFromFile
		static.Merge(a.Static)
		a.Static = static
	}
	a.Files = nil
	a.Dynamic = a.Set
	a.Set = nil

	return nil
}

func (a *AppRepoConfig) GetValuesConfig(ctx BosunContext) AppValuesConfig {
	out := a.Values.GetValuesConfig(ctx.WithDir(a.FromPath))

	if out.Static == nil {
		out.Static = Values{}
	}
	if out.Dynamic == nil {
		out.Dynamic = map[string]*DynamicValue{}
	}

	return out
}

// ExportValues creates an AppValuesByEnvironment instance with all the values
// for releasing this app, reified into their environments, including values from
// files and from the default values.yaml file for the chart.
func (a *AppRepo) ExportValues(ctx BosunContext) (AppValuesByEnvironment, error) {
	ctx = ctx.WithAppRepo(a)
	var err error
	envs := map[string]*EnvironmentConfig{}
	for envNames := range a.Values {
		for _, envName := range strings.Split(envNames, ",") {
			if _, ok := envs[envName]; !ok {
				envs[envName], err = ctx.Bosun.GetEnvironment(envName)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	chartRef := a.getAbsoluteChartPathOrChart(ctx)
	valuesYaml, err := pkg.NewCommand(
		"helm", "inspect", "values",
		chartRef,
		"--version", a.Version,
	).RunOut()
	if err != nil {
		return nil, errors.Errorf("load default values from %q: %s", chartRef, err)
	}

	defaultValues, err := ReadValues([]byte(valuesYaml))
	if err != nil {
		return nil, errors.Errorf("parse default values from %q: %s", chartRef, err)
	}

	out := AppValuesByEnvironment{}

	for _, env := range envs {
		envCtx := ctx.WithEnv(env)
		valuesConfig := a.GetValuesConfig(envCtx)
		err = valuesConfig.LoadFiles(envCtx)
		if err != nil {
			return nil, err
		}
		// make sure values from bosun app overwrite defaults from helm chart
		static := defaultValues.Clone()
		static.Merge(valuesConfig.Static)
		valuesConfig.Static = static
		valuesConfig.Files = nil
		valuesConfig.Dynamic = valuesConfig.Set
		valuesConfig.Set = nil
		out[env.Name] = valuesConfig
	}

	return out, nil
}

func (a *AppRepo) ExportActions(ctx BosunContext) ([]*AppAction, error) {
	var err error
	var actions []*AppAction
	for _, action := range a.Actions {
		err = action.MakeSelfContained(ctx)
		if err != nil {
			return nil, errors.Errorf("prepare action %q for release: %s", action.Name, err)
		}
		actions = append(actions, action)
	}

	return actions, nil
}

type AppStatesByEnvironment map[string]AppStateMap

type AppStateMap map[string]AppState

type AppState struct {
	Branch  string `yaml:"branch,omitempty"`
	Status  string `yaml:"deployment,omitempty"`
	Routing string `yaml:"routing,omitempty"`
	Version string `yaml:"version,omitempty"`
	Diff    string `yaml:"-"`
	Error   error  `yaml:"-"`
	Force   bool   `yaml:"-"`
	Unavailable bool `yaml:"-"`
}

func (a AppState) String() string {
	hasDiff := a.Diff != ""
	return fmt.Sprintf("status:%s routing:%s version:%s hasDiff:%t, force:%t",
		a.Status,
		a.Routing,
		a.Version,
		hasDiff,
		a.Force)
}

const (
	RoutingLocalhost     = "localhost"
	RoutingCluster       = "cluster"
	RoutingNA            = "n/a"
	StatusDeployed       = "DEPLOYED"
	StatusNotFound       = "NOTFOUND"
	StatusDeleted        = "DELETED"
	StatusFailed         = "FAILED"
	StatusPendingUpgrade = "PENDING_UPGRADE"
	StatusUnchanged = "UNCHANGED"
)

type Routing string
