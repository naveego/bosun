package tmpcache

import (
	"github.com/naveego/bosun/pkg/yaml"
	"io/ioutil"
	"os"
	"path"
)

var known = map[string][]byte{}

func makeTmpPath(key string) string {
	return  path.Join(os.TempDir(), "bosun_"+key)
}

func Get(key string, out interface{}) bool {

	var content []byte
	var ok bool
	var err error
	if content, ok = known[key]; !ok {

		tmpPath := makeTmpPath(key)

		content, err = ioutil.ReadFile(tmpPath)
		if err != nil {
			return false
		}
		if len(content) == 0 {
			return false
		}
	}

	known[key] = content
	err = yaml.Unmarshal(content, out)
	if err != nil {
		return false
	}

	return true
}

func Set(key string, in interface{})  {
	content, err := yaml.Marshal(in)
	if err != nil {
		return
	}

	known[key] = content

	tmpPath := makeTmpPath(key)

	_ = ioutil.WriteFile(tmpPath, content, 0660)
}
