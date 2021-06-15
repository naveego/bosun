package kube

import (
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environmentvariables"
	"github.com/naveego/bosun/pkg/templating"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"regexp"
)

type StackTemplate struct {
	core.ConfigShared `yaml:",inline"`
	NamePattern       string                               `yaml:"namePattern"`
	Variables         []*environmentvariables.Variable     `yaml:"variables,omitempty"`
	Namespaces        NamespaceConfigs                     `yaml:"namespaces"`
	Apps              map[string]values.ValueSetCollection `yaml:"apps"`
	Certs             []ClusterCert                        `yaml:"certs"`
	ValueOverrides    *values.ValueSetCollection           `yaml:"valueOverrides,omitempty"`
}

type StackState struct {
	Name          string `yaml:"name"`
	TemplateName  string `yaml:"templateName"`
	StoryID       string `yaml:"storyId"`
	Uninitialized bool
	DeployedApps  map[string]StackApp `yaml:"apps"`
}

func (e *StackTemplate) SetFromPath(path string) {
	e.FromPath = path
	for i := range e.Variables {
		e.Variables[i].FromPath = path
	}
}

func (e *StackTemplate) Render(name string) (*StackTemplate, error) {
	y, _ := yaml.MarshalString(e)

	re, err := regexp.Compile(e.NamePattern)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid name pattern in template %q", e.Name)
	}

	m := re.FindStringSubmatch(name)
	shortName := name
	if len(m) > 1 {
		shortName = m[1]
	}

	parameters := map[string]string{
		"Name":      name,
		"ShortName": shortName,
	}

	var renderedStackTemplate *StackTemplate

	rendered, err := templating.NewTemplateBuilder(name + "-stack-template").WithTemplate(y).BuildAndExecute(parameters)
	if err != nil {
		return nil, err
	}

	err = yaml.UnmarshalString(rendered, &renderedStackTemplate)

	if err != nil {
		return nil, err
	}

	renderedStackTemplate.Name = name
	renderedStackTemplate.SetFromPath(e.FromPath)

	return renderedStackTemplate, nil
}