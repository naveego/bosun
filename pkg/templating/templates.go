package templating

import (
	"github.com/Masterminds/sprig"
	"github.com/fatih/color"
	vault "github.com/hashicorp/vault/api"
	"github.com/imdario/mergo"
	"github.com/naveego/bosun/pkg/templating/templatefuncs"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"github.com/rs/xid"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
)

type TemplateHelper struct {
	TemplateValues TemplateValues
	VaultClient    *vault.Client
	templateFuncs []template.FuncMap
}

func (h *TemplateHelper) LoadFromYaml(out interface{}, globs ...string) error {

	mergedYaml, err := h.LoadMergedYaml(globs...)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal([]byte(mergedYaml), out)

	return err
}

func (h *TemplateHelper) WithTemplateFuncs(fns ...template.FuncMap) *TemplateHelper {
	h.templateFuncs = append(h.templateFuncs, fns...)
	return h
}

var lineExtractor = regexp.MustCompile(`line (\d+):`)

func (h *TemplateHelper) LoadMergedYaml(globs ...string) (string, error) {

	var merged map[string]interface{}

	var paths []string
	for _, glob := range globs {
		p, err := filepath.Glob(glob)

		if err != nil {
			return "", err
		}
		paths = append(paths, p...)
	}

	if len(paths) == 0 {
		return "", errors.Errorf("no paths found from expanding %v", globs)
	}

	for _, path := range paths {

		b, err := ioutil.ReadFile(path)
		if err != nil {
			return "", err
		}

		builder := NewTemplateBuilder(path).
			WithFunctions(h.templateFuncs...).
			WithTemplate(string(b))

		yamlString, err := builder.BuildAndExecute(h.TemplateValues)

		if err != nil {
			return "", err
		}

		var current map[string]interface{}

		err = yaml.Unmarshal([]byte(yamlString), &current)
		if err != nil {
			var badLine int
			matches := lineExtractor.FindStringSubmatch(err.Error())
			if len(matches) > 0 {
				badLine, _ = strconv.Atoi(matches[1])
			}
			color.Red("Invalid yaml in %s at line %d:", path, badLine)

			//fmt.Println(yamlString)

			return "", err
		}

		if err = mergo.Merge(&merged, current); err != nil {
			return "", err
		}
	}

	mergedYaml, err := yaml.Marshal(merged)

	return string(mergedYaml), err
}

type TemplateBuilder struct {
	t       *template.Template
	docs    []TemplateFuncDocs
	content string
}

type TemplateFuncDocs struct {
	Name        string
	Description string
	Args        []string
}

func NewTemplateBuilder(name string) *TemplateBuilder {
	t := template.New(name).
		Funcs(sprig.TxtFuncMap()).
		Funcs(templatefuncs.Include(template.FuncMap{
			"xid": func() string {
				return xid.New().String()
			},
		}))

	return &TemplateBuilder{t: t}
}

func (t *TemplateBuilder) Build() (*template.Template, error) {
	var err error
	t.t, err = t.t.Parse(t.content)
	return t.t, errors.WithStack(err)
}

func (t *TemplateBuilder) BuildAndExecute(input interface{}) (string, error) {

	o, err := t.Build()
	if err != nil {
		return "", err
	}

	w := new(strings.Builder)
	err = o.Execute(w, input)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return w.String(), nil
}

func (t *TemplateBuilder) WithTemplate(c string) *TemplateBuilder {
	t.content = c
	return t
}

func (t *TemplateBuilder) AddFunc(name, description string, args []string, fn interface{}) {
	t.docs = append(t.docs, TemplateFuncDocs{
		Name:        name,
		Description: description,
		Args:        args,
	})

	t.t.Funcs(template.FuncMap{
		name: fn,
	})
}

func (t *TemplateBuilder) WithFunctions(fns ...template.FuncMap) *TemplateBuilder{
	for _, fn := range fns {
		t.t = t.t.Funcs(fn)
	}
	return t
}

func (t *TemplateBuilder) WithDisabledVaultTemplateFunctions() *TemplateBuilder {

	t.t = t.t.Funcs(template.FuncMap{
		"vaultWrappedAppRoleToken": func(role string) (string, error) {
			return "disabled", nil
		},

		"vaultTokenWithPolicy": func(policy string) (string, error) {
			return "disabled", nil
		},

		"vaultTokenWithRole": func(role string) (string, error) {
			return "disabled", nil
		},

		"vaultSecret": func(path string, optionalKey ...string) (string, error) {
			return "disabled", nil
		},
	})

	return t
}
