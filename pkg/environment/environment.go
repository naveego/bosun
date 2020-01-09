package environment

import (
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/values"
)

type Environment struct {
	Config

	ClusterName string
	Cluster     *kube.ClusterConfig

	ValueSet values.ValueSet
}

type Options struct {
	Cluster   string
	ValueSets values.ValueSetCollection
}

func New(config Config, options Options) (Environment, error) {

	env := Environment{
		Config:      config,
		ClusterName: options.Cluster,
		ValueSet:    values.ValueSet{},
	}

	env = env.WithValuesFrom(options.ValueSets)

	return env, nil
}

func (e Environment) WithValuesFrom(source values.ValueSetCollection) Environment {

	valueSetFromNames := source.ExtractValueSetByNames(e.Config.ValueSetNames...)
	valueSetFromRoles := source.ExtractValueSetByRoles(e.Config.Role)

	e.ValueSet = valueSetFromNames.WithValues(valueSetFromRoles)
	return e
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

func (e *Environment) SwitchToCluster(cluster *kube.ClusterConfig) error {

	if e.Cluster != nil && e.Cluster.Name == cluster.Name {
		return nil
	}

	var err error

	e.Cluster = cluster

	err = e.Clusters.HandleConfigureKubeContextRequest(kube.ConfigureKubeContextRequest{
		Name: e.Cluster.Name,
		Log:  pkg.Log,
	})

	if err != nil {
		return err
	}

	pkg.Log.Infof("Switching to cluster %q", e.Cluster.Name)
	err = pkg.NewShellExe("kubectl", "config", "use-context", e.Cluster.Name).RunE()

	return err
}

func (e *Environment) GetClusterForRole(role core.ClusterRole) (*kube.ClusterConfig, error) {

	cluster, err := e.Clusters.GetKubeConfigDefinitionByRole(role)
	return cluster, err
}

type EnvironmentFilterable interface {
	GetEnvironmentRoles() (core.EnvironmentRoles, bool)
	GetEnvironmentName() (string, bool)
}
