package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg/actions"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/script"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"path/filepath"
	"strings"
)

type AppConfig struct {
	core.ConfigShared       `yaml:",inline"`
	ProjectManagementPlugin *ProjectManagementPlugin `yaml:"projectManagementPlugin,omitempty" json:"projectManagementPlugin,omitempty"`
	BranchForRelease        bool                     `yaml:"branchForRelease,omitempty" json:"branchForRelease,omitempty"`
	Branching               git.BranchSpec           `yaml:"branching,omitempty" json:"branching"`
	// ContractsOnly means that the app doesn't have any compiled/deployed code, it just defines contracts or documentation.
	ContractsOnly bool `yaml:"contractsOnly,omitempty" json:"contractsOnly,omitempty"`
	// FilesOnly means the app consists only of the files referenced in the bosun file, with no compiled code.
	FilesOnly        bool           `yaml:"filesOnly,omitempty" json:"filesOnly,omitempty"`
	ReportDeployment bool           `yaml:"reportDeployment,omitempty" json:"reportDeployment,omitempty"`
	RepoName         string         `yaml:"repo,omitempty" json:"repo,omitempty"`
	HarborProject    string         `yaml:"harborProject,omitempty" json:"harborProject,omitempty"`
	Version          semver.Version `yaml:"version,omitempty" json:"version,omitempty"`
	// The location of a standard go version file for this app.
	GoVersionFile string                    `yaml:"goVersionFile,omitempty" json:"goVersionFile,omitempty"`
	Chart         string                    `yaml:"chart,omitempty" json:"chart,omitempty"`
	ChartPath     string                    `yaml:"chartPath,omitempty" json:"chartPath,omitempty"`
	RunCommand    []string                  `yaml:"runCommand,omitempty,flow" json:"runCommand,omitempty,flow"`
	DependsOn     []Dependency              `yaml:"dependsOn,omitempty" json:"dependsOn,omitempty"`
	Labels        filter.Labels             `yaml:"labels,omitempty" json:"labels,omitempty"`
	Minikube      *AppMinikubeConfig        `yaml:"minikube,omitempty" json:"minikube,omitempty"`
	Images        []AppImageConfig          `yaml:"images" json:"images"`
	ValueMappings values.ValueMappings      `yaml:"valueMappings,omitempty"`
	Values        values.ValueSetCollection `yaml:"values,omitempty" json:"values,omitempty"`
	Scripts       []*script.Script          `yaml:"scripts,omitempty" json:"scripts,omitempty"`
	Actions       []*actions.AppAction      `yaml:"actions,omitempty" json:"actions,omitempty"`
	// Glob paths (relative to the file containing the app config)
	// to files and folders  which should be included when the app is packaged for a release or a deployment.
	// In particular, the path to the chart should be included.
	Files          []string          `yaml:"files"`
	ReleaseHistory AppReleaseHistory `yaml:"releaseHistory" json:"releaseHistory,omitempty"`

	// If true, this app repo is only a ref, not a real cloned repo.
	IsRef          bool         `yaml:"-" json:"-"`
	IsFromManifest bool         `yaml:"-"` // Will be true if this config was embedded in an AppManifest.
	manifest       *AppManifest // Will contain a pointer to the container if this AppConfig is contained in an AppManifest
	ProviderInfo   string
}

func (p *AppConfig) GetValueSetCollection() values.ValueSetCollection {
	return p.Values
}

type AppReleaseHistory []AppReleaseHistoryEntry
type AppReleaseHistoryEntry struct {
	ReleaseVersion string
	Version        string
}

func (a *AppConfig) AddReleaseToHistory(releaseVersion string) {
	thisVersion := a.Version.String()
	var history AppReleaseHistory
	var found bool
	for _, entry := range a.ReleaseHistory {
		if entry.ReleaseVersion == releaseVersion {
			entry.Version = thisVersion
			found = true
		}
		history = append(history, entry)
	}
	if !found {
		history = append(AppReleaseHistory{{ReleaseVersion: releaseVersion, Version: thisVersion}}, history...)
	}
	a.ReleaseHistory = history
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
		a.Branching.IsDefaulted = true
	}
	if a.Branching.Develop == "" {
		// default behavior is trunk based development
		a.Branching.Develop = "master"
		a.Branching.IsDefaulted = true
	}
	if a.Branching.Release == "" {
		a.Branching.Release = "release/{{.Version}}"
		a.Branching.IsDefaulted = true
	}
	if a.Branching.Feature == "" {
		a.Branching.Feature = "issue/{{.ID}}/{{.Slug}}"
		a.Branching.IsDefaulted = true
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
	Name string `yaml:"name" json:"name"`
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
	InternalPort  int    `yaml:"internalPort" json:"internalPort"`
	LocalhostPort int    `yaml:"localhostPort" json:"localhostPort,omitempty"`
	Namespace     string `yaml:"namespace"`
}

type Dependency struct {
	Name     string         `yaml:"name" json:"name,omitempty"`
	FromPath string         `yaml:"-" json:"fromPath,omitempty"`
	Repo     string         `yaml:"repo,omitempty" json:"repo,omitempty"`
	Version  semver.Version `yaml:"version,omitempty" json:"version,omitempty"`
}

type Dependencies []Dependency

func (d Dependencies) Len() int           { return len(d) }
func (d Dependencies) Less(i, j int) bool { return strings.Compare(d[i].Name, d[j].Name) < 0 }
func (d Dependencies) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }

func (a *AppConfig) SetFromPath(fromPath string) {
	a.FromPath = fromPath
	for i := range a.Scripts {
		a.Scripts[i].SetFromPath(a.FromPath)
	}
	for i := range a.DependsOn {
		a.DependsOn[i].FromPath = a.FromPath
	}
	for i := range a.Actions {
		a.Actions[i].SetFromPath(a.FromPath)
	}
}

func (a *AppConfig) LoadChartValues() (values.ValueSet, error) {

	if a.ChartPath != "" {
		chartRef := a.ResolveRelative(a.ChartPath)
		valuesYaml, err := command.NewShellExe(
			"helm", "inspect", "values",
			chartRef,
			"--version", a.Version.String(),
		).RunOut()
		if err != nil {
			return values.ValueSet{}, errors.Errorf("load default values from %q: %s", chartRef, err)
		}
		var chartValues values.Values
		chartValues, err = values.ReadValues([]byte(valuesYaml))
		if err != nil {
			return values.ValueSet{}, errors.Errorf("parse default values from %q: %s", chartRef, err)
		}
		return values.ValueSet{
			Source: fmt.Sprintf("%s chart at %s", a.Name, chartRef),
			Static: chartValues,
		}, nil
	} else {
		return values.ValueSet{
			Source: fmt.Sprintf("%s chart not found", a.Name),
		}, nil
	}
}
