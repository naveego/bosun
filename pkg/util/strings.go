package util

import (
	"crypto/sha256"
	"fmt"
	"gopkg.in/yaml.v2"
	"reflect"
	"sort"
)

// StringSliceToMap converts
// []string{"a","A", "b", "B"} to
// map[string]string{"a":"A", "b":"B"}
func StringSliceToMap(ss ...string) map[string]string {
	out := map[string]string{}
	for i := 0; i+1 < len(ss); i += 2 {
		out[ss[i]] = ss[i+1]
	}
	return out
}

func ConcatStrings(stringsOrSlices ...interface{}) []string {
	var out []string
	for i, x := range stringsOrSlices {
		switch v := x.(type) {
		case string:
			out = append(out, v)
		case []string:
			out = append(out, v...)
		default:
			panic(fmt.Sprintf("want string or []string, got %v (%T) at %d", x, x, i))
		}
	}
	return out
}

func HashToStringViaYaml(i interface{}) (string, error) {
	y, err := yaml.Marshal(i)
	if err != nil {
		return "", err
	}

	h := sha256.New()
	_, err = h.Write(y)
	if err != nil {
		return "", err
	}
	o := h.Sum(nil)

	return fmt.Sprintf("%x", o), nil
}

func SortedKeys(i interface{}) []string {
	if i == nil {
		return nil
	}
	v := reflect.ValueOf(i)
	if v.Kind() != reflect.Map {
		panic(fmt.Sprintf("need a map, got a %T", i))
	}

	keyValues := v.MapKeys()
	var keyStrings []string

	for _, kv := range keyValues {
		if s, ok := kv.Interface().(string); !ok {
			panic(fmt.Sprintf("map keys must strings, got key of %s", kv.Type()))
		} else {
			keyStrings = append(keyStrings, s)
		}
	}

	sort.Strings(keyStrings)
	return keyStrings
}
