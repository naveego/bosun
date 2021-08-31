package apps

import (
	"fmt"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/yaml"
)

type AppRef struct {
	Name             string                 `yaml:"name"`
	Provider         string                 `yaml:"provider"`
	Version          semver.Version         `yaml:"version"`
	Platform         string                 `yaml:"platform"`
	ProviderMetadata map[string]interface{} `yaml:"providerMetadata"`
}

func (a AppRef) String() string {
	return fmt.Sprintf("%s @ %s from %s", a.Name, a.Version, a.Provider)
}

type AppRefList []AppRef

func (a AppRefList) Headers() []string {

	return []string{
		"Name",
		"Version",
		"Platform",
		"Provider",
		"ProviderMetadata",
	}
}

func (a AppRefList) Rows() [][]string {

	var out [][]string

	for _, appRef := range a {

		pm, _ := yaml.MarshalString(appRef.ProviderMetadata)

		row := []string{
			appRef.Name,
			appRef.Provider,
			appRef.Version.String(),
			pm,
		}
		out = append(out, row)
	}

	return out

}
