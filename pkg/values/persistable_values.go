package values

import (
	"github.com/naveego/bosun/pkg/core"
	yml "gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
	"regexp"
)

type PersistableValues struct {
	Attribution Values `yaml:"attribution"`
	Values      Values `yaml:"values"`
	FilePath    string `yaml:"-"`
}

func (r *PersistableValues) PersistValues() (string, error) {
	if r.FilePath == "" {

		// b, err := r.Values.YAML()
		// if err != nil {
		// 	return "", err
		// }
		// r.FilePath = server.GetDefaultServer().AddValueFile(uuid.New().String(), []byte(b))

		tmp, err := ioutil.TempFile(os.TempDir(), "bosun-release-*.yaml")
		if err != nil {
			return "", err
		}
		defer tmp.Close()
		err = r.Values.Encode(tmp)
		if err != nil {
			return "", err
		}
		r.FilePath = tmp.Name()
		return r.FilePath, nil
	}
	return r.FilePath, nil

}

func (r *PersistableValues) Cleanup() {
	err := os.Remove(r.FilePath)
	if err != nil && !os.IsNotExist(err) {
		core.Log.WithError(err).WithField("path", r.FilePath).
			Fatal("Failed to clean up persisted values file, which make contain secrets. You must manually delete this file.")
	}
}

var attributionCommentRegex = regexp.MustCompile(`^(\s)*(\S)`)

func (r *PersistableValues) Report() string {

	values, _ := yml.Marshal(r.Values)

	var valuesRoot yml.Node

	err := yml.Unmarshal(values, &valuesRoot)
	if err != nil {
		panic(err)
	}

	applyAttributionComments(&valuesRoot, r.Attribution, "")

	outBytes, _ := yml.Marshal(valuesRoot)

	return string(outBytes)
}

func applyAttributionComments(node *yml.Node, attributions Values, path string) {

	attribution, err := attributions.GetAtPath(path)
	if err != nil {
		if attrString, ok := attribution.(string); ok {
			node.HeadComment = attrString
		} else {
			node.HeadComment = err.Error()
		}
	}

	for _, content := range node.Content {
		switch content.Kind {
		case yml.MappingNode:
			applyAttributionComments(content, attributions, path+"."+content.Value)
		}
	}

}
