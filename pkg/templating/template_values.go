package templating

import (
	"fmt"
	"github.com/Masterminds/sprig"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"regexp"
	"strings"
	"text/template"
)

type TemplateValues struct {
	Cluster   string
	Domain    string
	Values    map[string]interface{}
	Functions TemplateFunctions
}

type TemplateValuer interface {
	TemplateValues() TemplateValues
}

func NewTemplateValuesFromStrings(args ...string) (TemplateValues, error) {
	t := TemplateValues{}
	for _, kv := range args {
		segs := strings.Split(kv, "=")
		if len(segs) != 2 {
			return t, errors.Errorf("invalid values flag value: %q (should be Key=value)", kv)
		}
		t.Values[segs[0]] = segs[1]
	}

	return t, nil
}

type TemplateFunctions struct {
	ResolveSecretPath func(secretPath string) (string, error)
}

// RenderTemplate compiles and renders a simple template.
func (v TemplateValues) RenderInto(rawTemplate string) (string, error) {
	// deal with nested templates
	escaped, unescape := escapeNonValuesTemplateCode(rawTemplate)


	t, err := template.New("simple").
		Funcs(sprig.TxtFuncMap()).
		Funcs(map[string]interface{}{
			"resolveSecretPath": v.Functions.ResolveSecretPath,
		}).Parse(escaped)

	if err != nil {

		var numbered []string
		lines := strings.Split(rawTemplate, "\n")
		for i, line := range lines {
			numbered = append(numbered, fmt.Sprintf("%-4v %s", i+1, line))
		}

		numberedTemplate := strings.Join(numbered, "\n")

		return "", errors.Errorf("%s: could not parse template\n\n%s\n\n%s", err, numberedTemplate, err)
	}

	w := new(strings.Builder)
	err = t.Option("missingkey=error").Execute(w, v)
	if err != nil {
		// this formats the error at the beginning and the end so it's easy to find if the template is very long
		return "", errors.Errorf("could not execute template: %s\ntemplate:\n%s\ninput:\n%#v\n!!! could not execute template: %s", err, rawTemplate, v, err)
	}

	rendered := w.String()

	rendered = unescape(rendered)

	return rendered, nil
}


var templateEscapeRE = regexp.MustCompile(`{{[^}]+}}`)

func escapeNonValuesTemplateCode(in string) (escaped string, unescape func(string) string) {

	m := map[string]string{}

	escaped = templateEscapeRE.ReplaceAllStringFunc(in, func(s string) string {
		if strings.Contains(s, ".Values") ||
			strings.Contains(s, "resolveSecretPath") {
			return s
		}
		key := uuid.New()
		m[key] = s
		return key
	})

	unescape = func(s string) string {
		for key, value := range m {
			s = strings.Replace(s, key, value, 1)
		}
		return s
	}
	return
}
