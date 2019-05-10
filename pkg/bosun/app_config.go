package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/zenhub"
	"github.com/pkg/errors"
	"path/filepath"
	"strings"
)

type AppConfig struct {
	Name                    string                   `yaml:"name" json:"name" json:"name" json:"name"`
	FromPath                string                   `yaml:"-" json:"-"`
	ProjectManagementPlugin *ProjectManagementPlugin `yaml:"projectManagementPlugin,omitempty" json:"projectManagementPlugin,omitempty"`
	BranchForRelease        bool                     `yaml:"branchForRelease,omitempty" json:"branchForRelease,omitempty"`
	// ContractsOnly means that the app doesn't have any compiled/deployed code, it just defines contracts or documentation.
	ContractsOnly    bool           `yaml:"contractsOnly,omitempty" json:"contractsOnly,omitempty"`
	ReportDeployment bool           `yaml:"reportDeployment,omitempty" json:"reportDeployment,omitempty"`
	Namespace        string         `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	RepoName         string         `yaml:"repo,omitempty" json:"repo,omitempty"`
	HarborProject    string         `yaml:"harborProject,omitempty" json:"harborProject,omitempty"`
	Version          semver.Version `yaml:"version,omitempty" json:"version,omitempty"`
	// The location of a standard go version file for this app.
	GoVersionFile string             `yaml:"goVersionFile,omitempty" json:"goVersionFile,omitempty"`
	Chart         string             `yaml:"chart,omitempty" json:"chart,omitempty"`
	ChartPath     string             `yaml:"chartPath,omitempty" json:"chartPath,omitempty"`
	RunCommand    []string           `yaml:"runCommand,omitempty,flow" json:"runCommand,omitempty,flow"`
	DependsOn     []Dependency       `yaml:"dependsOn,omitempty" json:"dependsOn,omitempty"`
	Labels        filter.Labels      `yaml:"labels,omitempty" json:"labels,omitempty"`
	Minikube      *AppMinikubeConfig `yaml:"minikube,omitempty" json:"minikube,omitempty"`
	Images        []AppImageConfig   `yaml:"images" json:"images"`
	Values        ValueSetMap        `yaml:"values,omitempty" json:"values,omitempty"`
	Scripts       []*Script          `yaml:"scripts,omitempty" json:"scripts,omitempty"`
	Actions       []*AppAction       `yaml:"actions,omitempty" json:"actions,omitempty"`
	Parent        *File              `yaml:"-" json:"-"`
	// If true, this app repo is only a ref, not a real cloned repo.
	IsRef          bool         `yaml:"-" json:"-"`
	IsFromManifest bool         `yaml:"-"`          // Will be true if this config was embedded in an AppManifest.
	manifest       *AppManifest `yaml:"-" json:"-"` // Will contain a pointer to the container if this AppConfig is contained in an AppManifest
}

func (a *AppConfig) MarshalYAML() (interface{}, error) {
	if a == nil {
		return nil, nil
	}
	type proxy AppConfig
	p := proxy(*a)

	return &p, nil
}

func (a *AppConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type proxy AppConfig
	var p proxy
	if a != nil {
		p = proxy(*a)
	}

	err := unmarshal(&p)

	if err == nil {
		*a = AppConfig(p)
	}

	if a.Chart == "" && a.ChartPath != "" {
		a.Chart = filepath.Base(a.ChartPath)
	}

	return err
}

func (a *AppConfig) ErrIfFromManifest(msg string, args ...interface{}) error {
	if a.IsFromManifest {
		return errors.Errorf("app %q: %s", fmt.Sprintf(msg, args...))
	}
	return nil
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
	Name     string         `yaml:"name" json:"name,omitempty"`
	FromPath string         `yaml:"-" json:"fromPath,omitempty"`
	Repo     string         `yaml:"repo,omitempty" json:"repo,omitempty"`
	App      *App           `yaml:"-" json:"-"`
	Version  semver.Version `yaml:"version,omitempty" json:"version,omitempty"`
}

type Dependencies []Dependency

func (d Dependencies) Len() int           { return len(d) }
func (d Dependencies) Less(i, j int) bool { return strings.Compare(d[i].Name, d[j].Name) < 0 }
func (d Dependencies) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }

type appValuesConfigV1 struct {
	Set     map[string]*CommandValue `yaml:"set,omitempty" json:"set,omitempty"`
	Dynamic map[string]*CommandValue `yaml:"dynamic,omitempty" json:"dynamic,omitempty"`
	Files   []string                 `yaml:"files,omitempty" json:"files,omitempty"`
	Static  Values                   `yaml:"static,omitempty" json:"static,omitempty"`
}

func (a *AppConfig) SetParent(fragment *File) {
	a.FromPath = fragment.FromPath
	a.Parent = fragment
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

// GetNamespace returns the app's namespace, or "default" if it isn't set
func (a *AppConfig) GetNamespace() string {
	if a.Namespace != "" {
		return a.Namespace
	}
	return "default"
}

// Combine returns a new ValueSet with the values from
// other added after (and/or overwriting) the values from this instance)
func (a ValueSet) Combine(other ValueSet) ValueSet {
	out := ValueSet{
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

// ValueSetMap is a map of (possibly multiple) names
// to ValueSets. The the keys can be single names (like "red")
// or multiple, comma-delimited names (like "red,green").
// Use ExtractValueSetByName to get a merged ValueSet
// comprising the ValueSets under each key which contains that name.
type ValueSetMap map[string]ValueSet

// ExtractValueSetByName returns a merged ValueSet
// comprising the ValueSets under each key which contains the provided names.
// ValueSets with the same name are merged in order from least specific key
// to most specific, so values under the key "red" will overwrite values under "red,green",
// which will overwrite values under "red,green,blue", and so on. Then the
// ValueSets with each name are merged in the order the names were provided.
func (a ValueSetMap) ExtractValueSetByName(name string) ValueSet {

	out := ValueSet{}

	// More precise values should override less precise values
	// We assume no ValueSetMap will ever have more than 10
	// named keys in it.
	priorities := make([][]ValueSet, 10, 10)

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

// ExtractValueSetByName returns a merged ValueSet
// comprising the ValueSets under each key which contains the provided names.
// ValueSets with the same name are merged in order from least specific key
// to most specific, so values under the key "red" will overwrite values under "red,green",
// which will overwrite values under "red,green,blue", and so on. Then the
// ValueSets with each name are merged in the order the names were provided.
func (a ValueSetMap) ExtractValueSetByNames(names ...string) ValueSet {

	out := ValueSet{}

	for _, name := range names {
		vs := a.ExtractValueSetByName(name)
		out = out.Combine(vs)
	}

	return out
}

// CanonicalizedCopy returns a copy of this ValueSetMap with
// only single-name keys, by de-normalizing any multi-name keys.
// Each ValueSet will have its name set to the value of the name it's under.
func (a ValueSetMap) CanonicalizedCopy() ValueSetMap {

	out := ValueSetMap{}

	for k := range a {
		names := strings.Split(k, ",")
		for _, name := range names {
			out[name] = ValueSet{}
		}
	}

	for name := range out {
		vs := a.ExtractValueSetByName(name)
		vs.Name = name
		out[name] = vs
	}

	return out
}

// WithFilesLoaded resolves all file system dependencies into static values
// on this instance, then clears those dependencies.
func (a ValueSet) WithFilesLoaded(ctx BosunContext) (ValueSet, error) {

	out := ValueSet{
		Static: a.Static.Clone(),
	}

	mergedValues := Values{}

	// merge together values loaded from files
	for _, file := range a.Files {
		file = ctx.ResolvePath(file, "VALUE_SET", a.Name)
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
