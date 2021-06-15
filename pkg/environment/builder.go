package environment

import (
	"github.com/naveego/bosun/pkg/brns"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environmentvariables"
	"github.com/naveego/bosun/pkg/kube"
	"os"
)

type Builder struct {
	deps        Dependencies
	env         Config
	clusterName string
	stackName   string
}

type Dependencies interface {
	environmentvariables.Dependencies
	WithEnv(environment *Environment) interface{}
}

func (c Config) Builder(deps Dependencies) Builder {
	return Builder{
		deps: deps,
	}.WithEnvironment(c)
}

func (b Builder) WithEnvironment(env Config) Builder {
	b.env = env
	return b
}

func (b Builder) WithCluster(name string) Builder {
	b.clusterName = name
	return b
}

func (b Builder) WithStack(name string) Builder {
	b.stackName = name
	return b
}

const (
	builtEnvKey = "BOSUN_BUILT_ENV"
)

func (b Builder) Build() (*Environment, error) {
	var err error

	brn := brns.NewStack(b.env.Name, b.clusterName, b.stackName)

	internalBrn, found := core.GetInternalEnvironmentAndCluster()

	alreadyConfiguredEnv := found && brn.Equals(internalBrn)

	e := &Environment{
		Config: b.env,
	}

	deps := b.deps.WithPwd(b.env.FromPath).(Dependencies).
		WithEnv(e).(Dependencies)

	if !alreadyConfiguredEnv {

		for _, v := range b.env.Variables {
			if err = v.Ensure(deps); err != nil {
				return nil, err
			}
		}
	}

	if b.clusterName != "" {

		var clusterConfig *kube.ClusterConfig
		clusterConfig, err = e.Clusters.GetClusterConfig(b.clusterName)

		if err != nil {
			return nil, err
		}

		_ = os.Setenv(core.EnvCluster, clusterConfig.Name)

		e.cluster, err = kube.NewCluster(*clusterConfig, deps, false)
		if err != nil {
			return nil, err
		}

		err = e.cluster.Activate()
		if err != nil {
			return nil, err
		}

		if !alreadyConfiguredEnv {
			for _, v := range e.cluster.Variables {
				if err = v.Ensure(deps); err != nil {
					return nil, err
				}
			}
		}

		e.stack, err = e.Cluster().GetStack(b.stackName)
		if err != nil {
			return nil, err
		}

		if !e.stack.IsInitialized(){
			core.Log.Warnf("Current stack %q is not initialized, you should create it using `bosun stack create %s`", b.stackName, b.stackName)
		}

		if !alreadyConfiguredEnv {
			for _, v := range e.stack.Variables {
				if err = v.Ensure(deps); err != nil {
					return nil, err
				}
			}
		}
	}

	core.SetInternalBrn(brn)

	return e, nil
}

func (b Builder) WithBrn(brn brns.StackBrn) Builder {

	b.clusterName = brn.ClusterName
	b.stackName = brn.StackName
	return b
}
