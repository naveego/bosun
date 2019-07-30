package util

import (
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

func SaveYaml(path string, value interface{}) error {

	data, err := yaml.Marshal(value)
	if err != nil {
		return errors.Wrapf(err, "marshalling for save: %v", value)
	}

	err = ioutil.WriteFile(path, data, 0600)
	if err != nil {
		return errors.Wrap(err, "writing for save")
	}
	return nil
}

func LoadYaml(path string, out interface{}) error {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(b, out)
	return err
}
