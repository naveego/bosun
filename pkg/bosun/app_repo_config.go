package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/zenhub"
	"github.com/pkg/errors"
	"strings"
)

type AppRepoConfig struct {
	Name                    string                   `yaml:"name"`
	FromPath                string                   `yaml:"-"`
	ProjectManagementPlugin *ProjectManagementPlugin `yaml:"projectManagementPlugin,omitempty"`
	BranchForRelease        bool                     `yaml:"branchForRelease,omitempty"`
	// ContractsOnly means that the app doesn't have any compiled/deployed code, it just defines contracts or documentation.
	ContractsOnly    bool   `yaml:"contractsOnly,omitempty"`
	ReportDeployment bool   `yaml:"reportDeployment,omitempty"`
	Namespace        string `yaml:"namespace,omitempty"`
	Repo             string `yaml:"repo,omitempty"`
	HarborProject    string `yaml:"harborProject,omitempty"`
	Version          string `yaml:"version,omitempty"`
	// The location of a standard go version file for this app.
	GoVersionFile string                 `yaml:"goVersionFile,omitempty"`
	Chart         string                 `yaml:"chart,omitempty"`
	ChartPath     string                 `yaml:"chartPath,omitempty"`
	RunCommand    []string               `yaml:"runCommand,omitempty,flow"`
	DependsOn     []Dependency           `yaml:"dependsOn,omitempty"`
	AppLabels     Labels                 `yaml:"labels,omitempty"`
	Minikube      *AppMinikubeConfig     `yaml:"minikube,omitempty"`
	Images        []AppImageConfig       `yaml:"images"`
	Values        AppValuesByEnvironment `yaml:"values,omitempty"`
	Scripts       []*Script              `yaml:"scripts,omitempty"`
	Actions       []*AppAction           `yaml:"actions,omitempty"`
	Fragment      *File                  `yaml:"-"`
	// If true, this app repo is only a ref, not a real cloned repo.
	IsRef bool `yaml:"-"`
}

type ProjectManagementPlugin struct {
	Name   string             `yaml:"name"`
	ZenHub *zenhub.RepoConfig `yaml:"zenHub,omitempty"`
}

type AppImageConfig struct {
	ImageName   string `yaml:"imageName"`
	ProjectName string `yaml:"projectName,omitempty"`
	Dockerfile  string `yaml:"dockerfile,omitempty"`
	ContextPath string `yaml:"contextPath,omitempty"`
}

func (a AppImageConfig) GetPrefixedName() string {
	return fmt.Sprintf("docker.n5o.black/%s/%s", a.ProjectName, a.ImageName)
}

type AppMinikubeConfig struct {
	// The ports which should be made exposed through nodePorts
	// when running on minikube.
	Ports []int `yaml:"ports,omitempty"`
	// The services which should be replaced when toggling an
	// app to run on the host.
	RoutableServices []AppRoutableService `yaml:"routableServices"`
}

type AppRoutableService struct {
	Name     string `yaml:"name"`
	PortName string `yaml:"portName"`
	// Deprecated, use localhostPort instead
	ExternalPort  int `yaml:"externalPort"`
	LocalhostPort int `yaml:"localhostPort"`
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
	Set     map[string]*CommandValue `yaml:"set,omitempty"`
	Dynamic map[string]*CommandValue `yaml:"dynamic,omitempty"`
	Files   []string                 `yaml:"files,omitempty"`
	Static  Values                   `yaml:"static"`
}

func (a *AppRepoConfig) SetFragment(fragment *File) {
	a.FromPath = fragment.FromPath
	a.Fragment = fragment
	for i := range a.Scripts {
		a.Scripts[i].FromPath = a.FromPath
	}
	for i := range a.DependsOn {
		a.DependsOn[i].FromPath = a.FromPath
	}
	for i := range a.Actions {
		a.Actions[i].FromPath = a.FromPath
	}
}

// Combine returns a new AppValuesConfig with the values from
// other added after (and/or overwriting) the values from this instance)
func (a AppValuesConfig) Combine(other AppValuesConfig) AppValuesConfig {
	out := AppValuesConfig{
		Dynamic: make(map[string]*CommandValue),
		Static:  Values{},
	}
	out.Files = append(out.Files, a.Files...)
	out.Files = append(out.Files, other.Files...)

	out.Static.Merge(a.Static)
	out.Static.Merge(other.Static)

	// Set is deprecated, it should now be Dynamic,
	// so we copy everything into Dynamic.
	for k, v := range a.Set {
		out.Dynamic[k] = v
	}
	for k, v := range a.Dynamic {
		out.Dynamic[k] = v
	}
	for k, v := range other.Set {
		out.Dynamic[k] = v
	}
	for k, v := range other.Dynamic {
		out.Dynamic[k] = v
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

// WithFilesLoaded resolves all file system dependencies into static values
// on this instance, then clears those dependencies.
func (a AppValuesConfig) WithFilesLoaded(ctx BosunContext) (AppValuesConfig, error) {

	out := AppValuesConfig{
		Static: a.Static.Clone(),
	}

	mergedValues := Values{}

	// merge together values loaded from files
	for _, file := range a.Files {
		file = ctx.ResolvePath(file)
		valuesFromFile, err := ReadValuesFile(file)
		if err != nil {
			return out, errors.Errorf("reading values file %q for env key %q: %s", file, ctx.Env.Name, err)
		}
		mergedValues.Merge(valuesFromFile)
	}

	// make sure any existing static values are merged OVER the values from the file
	mergedValues.Merge(out.Static)
	out.Static = mergedValues

	out.Dynamic = a.Dynamic

	return out, nil
}

func (a *AppRepoConfig) GetValuesConfig(ctx BosunContext) AppValuesConfig {
	out := a.Values.GetValuesConfig(ctx.WithDir(a.FromPath))

	if out.Static == nil {
		out.Static = Values{}
	}
	if out.Dynamic == nil {
		out.Dynamic = map[string]*CommandValue{}
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
				env, err := ctx.Bosun.GetEnvironment(envName)
				if err != nil {
					ctx.Log.Warnf("App values include key for environment %q, but no such environment has been defined.", envName)
					continue
				}
				envs[envName] = env
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
		valuesConfig, err = valuesConfig.WithFilesLoaded(envCtx)
		if err != nil {
			return nil, err
		}
		// make sure values from bosun app overwrite defaults from helm chart
		static := defaultValues.Clone()
		static.Merge(valuesConfig.Static)
		valuesConfig.Static = static
		valuesConfig.Files = nil
		out[env.Name] = valuesConfig
	}

	return out, nil
}

func (a *AppRepo) ExportActions(ctx BosunContext) ([]*AppAction, error) {
	var err error
	var actions []*AppAction
	for _, action := range a.Actions {
		if action.When == ActionManual {
			ctx.Log.Infof("Skipping action %q because it is marked as manual.", action.Name)
		} else {
			err = action.MakeSelfContained(ctx)
			if err != nil {
				return nil, errors.Errorf("prepare action %q for release: %s", action.Name, err)
			}
			actions = append(actions, action)
		}
	}

	return actions, nil
}

type AppStatesByEnvironment map[string]AppStateMap

type AppStateMap map[string]AppState

type AppState struct {
	Branch      string `yaml:"branch,omitempty"`
	Status      string `yaml:"deployment,omitempty"`
	Routing     string `yaml:"routing,omitempty"`
	Version     string `yaml:"version,omitempty"`
	Diff        string `yaml:"-"`
	Error       error  `yaml:"-"`
	Force       bool   `yaml:"-"`
	Unavailable bool   `yaml:"-"`
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
	StatusUnchanged      = "UNCHANGED"
)

type Routing string
