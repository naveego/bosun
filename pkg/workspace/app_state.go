package workspace

import "fmt"

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
	StatusPendingUpgrade = "PENDING-UPGRADE"
	StatusUnchanged      = "UNCHANGED"
)

var KnownHelmChartStatuses = map[string]bool{
	StatusDeployed:       true,
	StatusNotFound:       true,
	StatusDeleted:        true,
	StatusFailed:         true,
	StatusPendingUpgrade: true,
	StatusUnchanged:      true,
}

type Routing string
