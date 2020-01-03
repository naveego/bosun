package yaml

import (
	"bytes"
	"encoding/json"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"strings"
)

// Yamlize ensures that a string is valid YAML.
func Yamlize(y string) string {
	return strings.Replace(y, "\t", "  ", -1)
}

func MustYaml(value interface{}) string {

	data, err := yaml.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func MustJSON(value interface{}) string {

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(data)
}

func SaveYaml(path string, value interface{}) error {

	data, err := Marshal(value)
	if err != nil {
		return errors.Wrapf(err, "marshalling for save: %v", value)
	}

	err = ioutil.WriteFile(path, []byte(data), 0600)
	if err != nil {
		return errors.Wrap(err, "writing for save")
	}
	return nil
}

func MarshalString(value interface{}) (string, error) {
	out, err := Marshal(value)
	return string(out), err
}

func Marshal(value interface{}) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	encoder := yaml.NewEncoder(buf)
	encoder.SetIndent(2)

	err := encoder.Encode(value)

	out := bytes.Trim(bytes.TrimSpace(buf.Bytes()), "\n")

	return out, err
}

func Unmarshal(b []byte, out interface{}) error {
	return yaml.Unmarshal(b, out)
}

func LoadYaml(path string, out interface{}) error {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(b, out)
	return err
}
