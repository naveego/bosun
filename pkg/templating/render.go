package templating

import (
	"github.com/pkg/errors"
	"strings"
	"text/template"
)

// RenderTemplate compiles and renders a simple template.
func RenderTemplate(rawTemplate string, input interface{}) (string, error) {
	t, err := template.New("simple").Parse(rawTemplate)
	if err != nil {
		return "", errors.Wrapf(err, "could not parse template %s", rawTemplate)
	}
	w := new(strings.Builder)
	err = t.Option("missingkey=error").Execute(w, input)
	if err != nil {
		return "", errors.Errorf("could not execute template: %s\ntemplate:\n%s\ninput:\n%#v", err, rawTemplate, input)
	}
	return w.String(), nil
}
