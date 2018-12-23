package bosun

import (
	"fmt"
	"strings"
)

type AppConfig struct {
	Name       string                 `yaml:"name"`
	FromPath   string                 `yaml:"fromPath,omitempty"`
	Namespace  string                 `yaml:"namespace,omitempty"`
	Repo       string                 `yaml:"repo,omitempty"`
	RepoPath   string                 `yaml:"repoPath,omitempty"`
	Version    string                 `yaml:"version,omitempty"`
	Chart      string                 `yaml:"chart,omitempty"`
	ChartPath  string                 `yaml:"chartPath,omitempty"`
	VaultPaths []string               `yaml:"vaultPaths,omitempty"`
	RunCommand []string               `yaml:"runCommand,omitempty"`
	DependsOn  []Dependency           `yaml:"dependsOn,omitempty"`
	Labels     []string               `yaml:"labels,omitempty"`
	Values     AppValuesByEnvironment `yaml:"values,omitempty"`
	Scripts    []*Script              `yaml:"scripts,omitempty"`
	Actions    []*AppAction           `yaml:"actions,omitempty"`
	Fragment *ConfigFragment            `yaml:"-"`
}

type Dependency struct {
	Name     string `yaml:"name,omitempty"`
	FromPath string `yaml:"fromPath"`
	Repo     string `yaml:"repo,omitempty"`
	App      *App   `yaml:"-"`
	Version  string `yaml:"version"`
}

type Dependencies []Dependency

func (d Dependencies) Len() int { return len(d) }
func (d Dependencies) Less(i, j int) bool { return strings.Compare(d[i].Name, d[j].Name) < 0 }
func (d Dependencies) Swap(i, j int) { d[i], d[j] = d[j], d[i] }

type AppValuesConfig struct {
	Set   map[string]*DynamicValue `yaml:"set,omitempty"`
	Files []string                 `yaml:"files,omitempty"`
}

func NewAppValues() AppValuesConfig {
	return AppValuesConfig{Set: make(map[string]*DynamicValue)}
}

func (a *AppConfig) SetFragment(fragment *ConfigFragment) {
	a.FromPath = fragment.FromPath
	a.Fragment = fragment
	for i := range a.Scripts {
		a.Scripts[i].FromPath = a.FromPath
	}
	for i := range a.DependsOn {
		a.DependsOn[i].FromPath = a.FromPath
	}
}

func (a *AppConfig) ConfigureForEnvironment(ctx BosunContext) {

	if a.ChartPath != "" {
		a.ChartPath = resolvePath(a.FromPath, a.ChartPath)
	}
	for i := range a.VaultPaths {
		a.VaultPaths[i] = resolvePath(a.FromPath, a.VaultPaths[i])
	}
	// only resolve the files for the current context, anything else is confusing
	// when the mergedFragments is dumped.
	for env, av := range a.Values {
		if env == ctx.Env.Name {
			for i := range av.Files {
				av.Files[i] = resolvePath(a.FromPath, av.Files[i])
			}
		}
	}
}

func (a AppValuesConfig) Combine(other AppValuesConfig) AppValuesConfig {
	out := AppValuesConfig{
		Set: make(map[string]*DynamicValue),
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

type AppValuesByEnvironment map[string]AppValuesConfig

func (a *AppValuesByEnvironment) UnmarshalYAML(unmarshal func(interface{}) error) error {

	var m map[string]AppValuesConfig

	err := unmarshal(&m)
	if err != nil {
		return err
	}

	multis := map[string]AppValuesConfig{}
	out := AppValuesByEnvironment{}

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
)

type Routing string
