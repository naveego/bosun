package templating

import (
	"github.com/pkg/errors"
	"strings"
)

type TemplateValues struct {
	Cluster string
	Domain  string
	Values  map[string]interface{}
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
