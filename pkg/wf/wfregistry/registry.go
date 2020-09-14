package wfregistry

import (
	"fmt"
	"github.com/naveego/bosun/pkg/wf/wfcontracts"
	"github.com/pkg/errors"
)

var DefaultRegistry = New()

type Registry struct {
	workflows map[string]func() wfcontracts.Workflow
}

func New() *Registry  {
	return  &Registry{workflows: map[string]func() wfcontracts.Workflow{}}
}

func (r *Registry) Register(typ string, factory func() wfcontracts.Workflow) {
	if _, ok := r.workflows[typ]; ok{
		panic(fmt.Sprintf("workflow with type %q already registered", typ))
	}
	r.workflows[typ] = factory
}

func (r *Registry) Create(typ string) (wfcontracts.Workflow, error){
	if factory, ok := r.workflows[typ]; !ok{
		return nil, errors.Errorf("workflow with type %q not registered", typ)
	} else {
		return factory(), nil
	}
}

func (r *Registry) GetConfigTemplates() []wfcontracts.Config {
	var configs []wfcontracts.Config
	for _, factory := range r.workflows {
		instance := factory()
		template, _ := instance.Templates()
		configs = append(configs, template)
	}
	return configs
}