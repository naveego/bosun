package environment

import (
	"errors"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environmentvariables"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/values"
)

type Environment struct {
	Config

	ClusterName string
	Cluster     *kube.ClusterConfig
}

func (e *Environment) GetValueSetCollection() values.ValueSetCollection {
	if e.ValueOverrides == nil {
		return values.NewValueSetCollection()
	}
	return *e.ValueOverrides
}

// GetAppValueSetCollectionProvider returns a ValuesSetCollectionProvider that will provide any values set collection
// defined in this environment for a specific app. If none is defined, an instance that does nothing will be returned.
func (e *Environment) GetAppValueSetCollectionProvider(appName string) values.ValueSetCollectionProvider {

	if appValueOverride, ok := e.AppValueOverrides[appName]; ok {
		return appValueSetCollectionProvider{
			valueSetCollection:appValueOverride,
		}
	}

	return appValueSetCollectionProvider{
		valueSetCollection:values.NewValueSetCollection(),
	}
}

type appValueSetCollectionProvider struct {
	valueSetCollection values.ValueSetCollection
}

func (a appValueSetCollectionProvider) GetValueSetCollection() values.ValueSetCollection {
	return a.valueSetCollection
}

type Options struct {
	Cluster string
}

func New(config Config, options Options) (Environment, error) {

	env := Environment{
		Config:      config,
		ClusterName: options.Cluster,
	}

	return env, nil
}

func (e Environment) Matches(candidate EnvironmentFilterable) bool {
	roles, checkRoles := candidate.GetEnvironmentRoles()
	if checkRoles {
		if !roles.Accepts(e.Role) {
			return false
		}
	}

	name, checkName := candidate.GetEnvironmentName()
	if checkName {
		if name != e.Name {
			return false
		}
	}

	return true
}

func (e *Environment) SwitchToCluster(ctx environmentvariables.EnsureContext, cluster *kube.ClusterConfig) error {

	if cluster == nil {
		return errors.New("cluster parameter was nil")
	}

	if e.Cluster != nil && e.Cluster.Name == cluster.Name {
		return nil
	}

	e.setCurrentCluster(cluster)

	e.ClusterName = cluster.Name
	e.Cluster = cluster

	return e.EnsureCluster(ctx)
}

func (e *Environment) GetClustersForRole(role core.ClusterRole) ([]*kube.ClusterConfig, error) {

	clusters, err := e.Clusters.GetKubeConfigDefinitionsByRole(role)
	return clusters, err
}

func (e *Environment) GetClusterByName(name string) (*kube.ClusterConfig, error) {
	cluster, err := e.Clusters.GetKubeConfigDefinitionByName(name)
	return cluster, err
}

func (e *Environment) setCurrentCluster(cluster *kube.ClusterConfig) {
	e.Cluster = cluster
	e.ClusterName = cluster.Name
}

type EnvironmentFilterable interface {
	GetEnvironmentRoles() (core.EnvironmentRoles, bool)
	GetEnvironmentName() (string, bool)
}
