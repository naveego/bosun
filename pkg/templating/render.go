package templating

import (
	"fmt"
	"github.com/Masterminds/sprig"
	"github.com/pkg/errors"
	"strings"
	"text/template"
)

// RenderTemplate compiles and renders a simple template.
func RenderTemplate(rawTemplate string, input interface{}) (string, error) {
	t, err := template.New("simple").
		Funcs(sprig.TxtFuncMap()).
		Parse(rawTemplate)
	if err != nil {

		var numbered []string
		lines := strings.Split(rawTemplate, "\n")
		for i, line := range lines {
			numbered = append(numbered, fmt.Sprintf("%-4v %s", i+1, line ))
		}

		numberedTemplate := strings.Join(numbered, "\n")

		return "", errors.Errorf( "%s: could not parse template\n\n%s\n\n%s", err, numberedTemplate, err)
	}

	w := new(strings.Builder)
	err = t.Option("missingkey=error").Execute(w, input)
	if err != nil {
		// this formats the error at the beginning and the end so it's easy to find if the template is very long
		return "", errors.Errorf("could not execute template: %s\ntemplate:\n%s\ninput:\n%#v\n!!! could not execute template: %s", err, rawTemplate, input, err)
	}
	return w.String(), nil
}
