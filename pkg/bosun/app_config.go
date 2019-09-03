package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/zenhub"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"path/filepath"
	"strings"
)

type AppConfig struct {
	Name                    string                   `yaml:"name" json:"name" json:"name" json:"name"`
	FromPath                string                   `yaml:"-" json:"-"`
	ProjectManagementPlugin *ProjectManagementPlugin `yaml:"projectManagementPlugin,omitempty" json:"projectManagementPlugin,omitempty"`
	BranchForRelease        bool                     `yaml:"branchForRelease,omitempty" json:"branchForRelease,omitempty"`
	Branching               git.BranchSpec           `yaml:"branching" json:"branching"`
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

	if a.Branching.Master == "" {
		a.Branching.Master = "master"
	}
	if a.Branching.Develop == "" {
		// default behavior is trunk based development
		a.Branching.Develop = "master"
	}
	if a.Branching.Release == "" && p.BranchForRelease {
		// migrate BranchForRelease to p.Branching.Release pattern.
		a.Branching.Release = "release/{{.Version}}"
	}
	if a.Branching.Feature == "" {
		a.Branching.Feature = "issue/{{.Number}}/{{.Slug}}"
	}

	return err
}

func (a *AppConfig) ErrIfFromManifest(msg string, args ...interface{}) error {
	if a.IsFromManifest {
		return errors.Errorf("app %q: %s", a.Name, fmt.Sprintf(msg, args...))
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
	ExternalPort int `yaml:"externalPort,omitempty" json:"externalPort,omitempty"`
	// The port the service should advertise within the cluster.
	InternalPort  int `yaml:"internalPort" json:"internalPort"`
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

	// clone the valueSet to ensure we don't mutate `a`
	y, _ := yaml.Marshal(a)
	var out ValueSet
	_ = yaml.Unmarshal(y, &out)

	// clone the other valueSet to ensure we don't capture items from it
	y, _ = yaml.Marshal(other)
	_ = yaml.Unmarshal(y, &other)

	if out.Dynamic == nil {
		out.Dynamic = map[string]*CommandValue{}
	}
	if out.Static == nil {
		out.Static = Values{}
	}

	out.Files = append(out.Files, other.Files...)

	out.Static.Merge(other.Static)

	for k, v := range other.Dynamic {
		out.Dynamic[k] = v
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
