package environment

import "github.com/naveego/bosun/pkg/values"

type Environment struct {
	Config

	Cluster string

	ValueSet values.ValueSet
}

type Options struct {
	Cluster   string
	ValueSets values.ValueSetCollection
}

func New(config Config, options Options) (Environment, error) {

	env := Environment{
		Config:   config,
		Cluster:  options.Cluster,
		ValueSet: values.ValueSet{},
	}

	env = env.WithValuesFrom(options.ValueSets)

	return env, nil
}

func (e Environment) WithValuesFrom(source values.ValueSetCollection) Environment {

	valueSetFromNames := source.ExtractValueSetByNames(e.Config.ValueSetNames...)
	valueSetFromRoles := source.ExtractValueSetByRoles(e.Config.Roles...)

	e.ValueSet = valueSetFromNames.WithValues(valueSetFromRoles)
	return e
}
