package templatefuncs

import "text/template"

var m = template.FuncMap{}


func Register(name string, fn interface{}) {
	m[name] = fn
}

func Include(fns template.FuncMap) template.FuncMap {

	for k, v := range m {
		fns[k] = v
	}

	return fns
}