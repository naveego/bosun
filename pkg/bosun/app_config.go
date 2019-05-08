package bosun

import (
	"fmt"
	"github.com/imdario/mergo"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/zenhub"
	"github.com/pkg/errors"
	"strings"
)

type AppConfig struct {
	Name                    string                   `yaml:"name" json:"name" json:"name" json:"name"`
	FromPath                string                   `yaml:"-" json:"-"`
	ProjectManagementPlugin *ProjectManagementPlugin `yaml:"projectManagementPlugin,omitempty" json:"projectManagementPlugin,omitempty"`
	BranchForRelease        bool                     `yaml:"branchForRelease,omitempty" json:"branchForRelease,omitempty"`
	// ContractsOnly means that the app doesn't have any compiled/deployed code, it just defines contracts or documentation.
	ContractsOnly    bool   `yaml:"contractsOnly,omitempty" json:"contractsOnly,omitempty"`
	ReportDeployment bool   `yaml:"reportDeployment,omitempty" json:"reportDeployment,omitempty"`
	Namespace        string `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	RepoName         string `yaml:"repo,omitempty" json:"repo,omitempty"`
	HarborProject    string `yaml:"harborProject,omitempty" json:"harborProject,omitempty"`
	Version          string `yaml:"version,omitempty" json:"version,omitempty"`
	// The location of a standard go version file for this app.
	GoVersionFile string                 `yaml:"goVersionFile,omitempty" json:"goVersionFile,omitempty"`
	Chart         string                 `yaml:"chart,omitempty" json:"chart,omitempty"`
	ChartPath     string                 `yaml:"chartPath,omitempty" json:"chartPath,omitempty"`
	RunCommand    []string               `yaml:"runCommand,omitempty,flow" json:"runCommand,omitempty,flow"`
	DependsOn     []Dependency           `yaml:"dependsOn,omitempty" json:"dependsOn,omitempty"`
	Labels        filter.Labels          `yaml:"labels,omitempty" json:"labels,omitempty"`
	Minikube      *AppMinikubeConfig     `yaml:"minikube,omitempty" json:"minikube,omitempty"`
	Images        []AppImageConfig       `yaml:"images" json:"images"`
	Values        AppValuesByEnvironment `yaml:"values,omitempty" json:"values,omitempty"`
	Scripts       []*Script              `yaml:"scripts,omitempty" json:"scripts,omitempty"`
	Actions       []*AppAction           `yaml:"actions,omitempty" json:"actions,omitempty"`
	Fragment      *File                  `yaml:"-" json:"-"`
	// If true, this app repo is only a ref, not a real cloned repo.
	IsRef bool `yaml:"-" json:"-"`
}

type ProjectManagementPlugin struct {
	Name   string             `yaml:"name" json:"name"`
	ZenHub *zenhub.RepoConfig `yaml:"zenHub,omitempty" json:"zenHub"`
}

type AppMinikubeConfig struct {
	// The ports which should be made exposed through nodePorts
	// when running on minikube.
	Ports []int `yaml:"ports,omitempty" json:"ports,omitempty"`
	// The services which should be replaced when toggling an
	// app to run on the host.
	RoutableServices []AppRoutableService `yaml:"routableServices" json:"routableServices"`
}

type AppRoutableService struct {
	Name     string `yaml:"name" json:"name,omitempty"`
	PortName string `yaml:"portName" json:"portName,omitempty"`
	// Deprecated, use localhostPort instead
	ExternalPort  int `yaml:"externalPort" json:"externalPort,omitempty"`
	LocalhostPort int `yaml:"localhostPort" json:"localhostPort,omitempty"`
}

type Dependency struct {
	Name     string `yaml:"name" json:"name,omitempty"`
	FromPath string `yaml:"-" json:"fromPath,omitempty"`
	Repo     string `yaml:"repo,omitempty" json:"repo,omitempty"`
	App      *App   `yaml:"-" json:"-"`
	Version  string `yaml:"version,omitempty" json:"version,omitempty"`
}

type Dependencies []Dependency

func (d Dependencies) Len() int           { return len(d) }
func (d Dependencies) Less(i, j int) bool { return strings.Compare(d[i].Name, d[j].Name) < 0 }
func (d Dependencies) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }

type AppValuesConfig struct {
	Dynamic map[string]*CommandValue `yaml:"dynamic,omitempty" json:"dynamic,omitempty"`
	Files   []string                 `yaml:"files,omitempty" json:"files,omitempty"`
	Static  Values                   `yaml:"static,omitempty" json:"static,omitempty"`
}

func (a *AppValuesConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var m map[string]interface{}
	err := unmarshal(&m)
	if err != nil {
		return errors.WithStack(err)
	}
	if _, ok := m["set"]; ok {
		// is v1
		var v1 appValuesConfigV1
		err = unmarshal(&v1)
		if err != nil {
			return errors.WithStack(err)
		}
		if a == nil {
			*a = AppValuesConfig{}
		}
		if v1.Static == nil {
			v1.Static = Values{}
		}
		if v1.Set == nil {
			v1.Set = map[string]*CommandValue{}
		}
		a.Files = v1.Files
		a.Static = v1.Static
		a.Dynamic = v1.Set
		// handle case where set AND dynamic both have values
		if v1.Dynamic != nil {
			err = mergo.Map(a.Dynamic, v1.Dynamic)
		}
		return err
	}

	type proxy AppValuesConfig
	var p proxy
	err = unmarshal(&p)
	if err != nil {
		return errors.WithStack(err)
	}
	*a = AppValuesConfig(p)
	return nil
}

type appValuesConfigV1 struct {
	Set     map[string]*CommandValue `yaml:"set,omitempty" json:"set,omitempty"`
	Dynamic map[string]*CommandValue `yaml:"dynamic,omitempty" json:"dynamic,omitempty"`
	Files   []string                 `yaml:"files,omitempty" json:"files,omitempty"`
	Static  Values                   `yaml:"static,omitempty" json:"static,omitempty"`
}

func (a *AppConfig) SetFragment(fragment *File) {
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

	for k, v := range a.Dynamic {
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

func (a *AppConfig) GetValuesConfig(ctx BosunContext) AppValuesConfig {
	out := a.Values.GetValuesConfig(ctx.WithDir(a.FromPath))

	if out.Static == nil {
		out.Static = Values{}
	}
	if out.Dynamic == nil {
		out.Dynamic = map[string]*CommandValue{}
	}

	return out
}

type AppStatesByEnvironment map[string]AppStateMap

type AppStateMap map[string]AppState

type AppState struct {
	Branch      string `yaml:"branch,omitempty" json:"branch,omitempty"`
	Status      string `yaml:"deployment,omitempty" json:"deployment,omitempty"`
	Routing     string `yaml:"routing,omitempty" json:"routing,omitempty"`
	Version     string `yaml:"version,omitempty" json:"version,omitempty"`
	Diff        string `yaml:"-" json:"-"`
	Error       error  `yaml:"-" json:"-"`
	Force       bool   `yaml:"-" json:"-"`
	Unavailable bool   `yaml:"-" json:"-"`
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
