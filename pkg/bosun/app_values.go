package bosun

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"strings"
)

// This code is copied from the Helm project, https://github.com/helm/helm/blob/58be8e461c9ab8e86ea2ad519601e1a288f21e8c/pkg/chartutil/values.go

// ErrNoTable indicates that a chart does not have a matching table.
type ErrNoTable error

// ErrNoValue indicates that Values does not contain a key with a value
type ErrNoValue error

// Values represents a collection of chart values.
type Values map[string]interface{}

// YAML encodes the Values into a YAML string.
func (v Values) YAML() (string, error) {
	b, err := yaml.Marshal(v)
	return string(b), err
}

// Table gets a table (YAML subsection) from a Values object.
//
// The table is returned as a Values.
//
// Compound table names may be specified with dots:
//
//	foo.bar
//
// The above will be evaluated as "The table bar inside the table
// foo".
//
// An ErrNoTable is returned if the table does not exist.
func (v Values) Table(name string) (Values, error) {
	names := strings.Split(name, ".")
	table := v
	var err error

	for _, n := range names {
		table, err = tableLookup(table, n)
		if err != nil {
			return table, err
		}
	}
	return table, err
}

// AsMap is a utility function for converting Values to a map[string]interface{}.
//
// It protects against nil map panics.
func (v Values) AsMap() map[string]interface{} {
	if v == nil || len(v) == 0 {
		return map[string]interface{}{}
	}
	return v
}

// Encode writes serialized Values information to the given io.Writer.
func (v Values) Encode(w io.Writer) error {
	//return yaml.NewEncoder(w).Encode(v)
	out, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(out)
	return err
}

// AddPath adds value to Values at the provided path, which can be a compound name.
// If there table missing from the path, they will be created.
func (v Values) AddPath(path string, value interface{}) {
	segs := strings.Split(path, ".")
	v.addPath(segs, value)
}

func (v Values) addPath(path []string, value interface{}) {

	if len(path) == 0{
		panic("invalid path")
	}
	name := path[0]
	if len(path) == 1 {
		v[name] = value
		return
	}
	child, ok := v[name].(Values)
	if !ok {
		child = Values{}
		v[name] = child
	}

	child.addPath(path[1:], value)
}

// MergeInto takes the properties in src and merges them into Values. Maps
// are merged while values and arrays are replaced.
func (v Values) MergeInto(src Values) {
	for key, srcVal := range src {
		destVal, found := v[key]

		if found && istable(srcVal) && istable(destVal) {
			destMap := destVal.(map[string]interface{})
			srcMap := srcVal.(map[string]interface{})
			Values(destMap).MergeInto(Values(srcMap))
		} else {
			v[key] = srcVal
		}
	}
}

// istable is a special-purpose function to see if the present thing matches the definition of a YAML table.
func istable(v interface{}) bool {
	_, ok := v.(map[string]interface{})
	return ok
}



func tableLookup(v Values, simple string) (Values, error) {
	v2, ok := v[simple]
	if !ok {
		return v, ErrNoTable(fmt.Errorf("no table named %q (%v)", simple, v))
	}
	if vv, ok := v2.(map[string]interface{}); ok {
		return vv, nil
	}

	// This catches a case where a value is of type Values, but doesn't (for some
	// reason) match the map[string]interface{}. This has been observed in the
	// wild, and might be a result of a nil map of type Values.
	if vv, ok := v2.(Values); ok {
		return vv, nil
	}

	var e ErrNoTable = fmt.Errorf("no table named %q", simple)
	return map[string]interface{}{}, e
}

// ReadValues will parse YAML byte data into a Values.
func ReadValues(data []byte) (vals Values, err error) {
	err = yaml.Unmarshal(data, &vals)
	if len(vals) == 0 {
		vals = Values{}
	}
	return
}

// ReadValuesFile will parse a YAML file into a map of values.
func ReadValuesFile(filename string) (Values, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return map[string]interface{}{}, err
	}
	return ReadValues(data)
}