package bosun

import "strings"

type AppConfig struct {
	FromPath   string               `yaml:"fromPath,omitempty"`
	Name       string               `yaml:"name"`
	Namespace  string               `yaml:"namespace,omitempty"`
	Repo       string               `yaml:"repo,omitempty"`
	Version    string               `yaml:"version,omitempty"`
	Chart      string               `yaml:"chart,omitempty"`
	ChartPath  string               `yaml:"chartPath,omitempty"`
	VaultPaths []string 			`yaml:"vaultPaths,omitempty"`
	RunCommand []string             `yaml:"runCommand,omitempty"`
	DependsOn  []Dependency         `yaml:"dependsOn,omitempty"`
	Labels     []string             `yaml:"labels,omitempty"`
	Values     map[string]AppValues `yaml:"values,omitempty"`
}

type Dependency struct {
	Name string `yaml:"name,omitempty"`
	Repo string `yaml:"repo,omitempty"`
}

type AppValues struct {
	Set   map[string]string `yaml:"set,omitempty"`
	Files []string          `yaml:"files,omitempty"`
}

func (a AppValues) Combine(other AppValues) AppValues {
	out := AppValues{
		Set: make(map[string]string),
	}
	out.Files = append(out.Files, other.Files...)
	out.Files = append(out.Files, a.Files...)

	for k, v := range a.Set {
		out.Set[k] = v
	}
	for k, v := range other.Set {
		if _, ok := out.Set[k]; !ok {
			out.Set[k] = v
		}
	}
	return out
}

type AppValuesMap map[string]AppValues

func (a *AppValuesMap) UnmarshalYAML(unmarshal func(interface{}) error) error {

	var m map[string]AppValues

	err := unmarshal(&m)
	if err != nil {
		return err
	}

	multis := map[string]AppValues{}
	out := AppValuesMap{}

	for k, v := range m {
		keys := strings.Split(k, ",")
		if len(keys) > 1 {
			multis[k] = multis[k].Combine(v)
		} else {
			out[k] = v
		}
	}

	for k, v := range multis {
		keys := strings.Split(k, ",")
		for _, k = range keys {
			out[k] = out[k].Combine(v)
		}
	}

	*a = out

	return nil
}

type AppState struct {
	Branch  string `yaml:"branch,omitempty"`
	Status  string `yaml:"deployment,omitempty"`
	Routing string `yaml:"routing,omitempty"`
	Version string `yaml:"version,omitempty"`
	Diff    string `yaml:"diff,omitempty"`
	Error   error  `yaml:"-"`
	Force bool `yaml:"-"`
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
)

type Routing string

func (a *AppConfig) SetFromPath(path string) {
	a.FromPath = path
	if a.ChartPath != "" {
		a.ChartPath = resolvePath(path, a.ChartPath)
	}
	for i := range a.VaultPaths {
		a.VaultPaths[i] = resolvePath(path, a.VaultPaths[i])
	}
}