package values

import (
	"github.com/naveego/bosun/pkg"
	"io/ioutil"
	"os"
)

type PersistableValues struct {
	Attribution Values `yaml:"attribution"`
	Values   Values `yaml:"values"`
	FilePath string `yaml:"-"`
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
		pkg.Log.WithError(err).WithField("path", r.FilePath).
			Fatal("Failed to clean up persisted values file, which make contain secrets. You must manually delete this file.")
	}
}
