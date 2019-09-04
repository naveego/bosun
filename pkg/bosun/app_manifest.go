package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/semver"
)

type AppMetadata struct {
	Name                 string          `yaml:"name" json:"name"`
	Repo                 string          `yaml:"repo" json:"repo"`
	Version              semver.Version  `yaml:"version" json:"version"`
	PinnedReleaseVersion *semver.Version `yaml:"pinnedReleaseVersion,omitempty"`
	Hashes               AppHashes       `yaml:"hashes"`
	Branch               string          `yaml:"branch" json:"branch"`
}

func (a *AppMetadata) RepoRef() issues.RepoRef {
	ref, _ := issues.ParseRepoRef(a.Repo)
	return ref
}

func (a *AppMetadata) PinToRelease(release *ReleaseMetadata) {
	a.PinnedReleaseVersion = &release.Version
}

//
// func (a *AppMetadata) GetImageTag() string {
// 	if a.PinnedReleaseVersion == nil {
// 		return a.Version.String()
// 	}
// 	return fmt.Sprintf("%s-%s", a.Version, a.PinnedReleaseVersion)
// }

func (a AppMetadata) Format(f fmt.State, c rune) {
	switch c {
	case 'c':
		_, _ = f.Write([]byte(a.Hashes.Commit))
	default:
		_, _ = f.Write([]byte(a.String()))
	}
}

func (a AppMetadata) String() string {
	return fmt.Sprintf("%s@%s", a.Name, a.Version)
}

// AppManifest contains the configuration for an app in a ReleaseManifest
// as part of a Platform. Instances should be manipulated using methods
// on Platform, not updated directly.
type AppManifest struct {
	*AppMetadata `yaml:"metadata"`
	AppConfig    *AppConfig `yaml:"appConfig" json:"appConfig"`
}

func (a *AppManifest) MarshalYAML() (interface{}, error) {
	if a == nil {
		return nil, nil
	}
	type proxy AppManifest
	p := proxy(*a)

	return &p, nil
}

func (a *AppManifest) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type proxy AppManifest
	var p proxy
	if a != nil {
		p = proxy(*a)
	}

	err := unmarshal(&p)

	if err == nil {
		*a = AppManifest(p)
	}

	if a.AppConfig != nil {
		a.AppConfig.IsFromManifest = true
		a.AppConfig.manifest = a
	}

	return err
}

func (a AppMetadata) DiffersFrom(other *AppMetadata) bool {

	return a.Version != other.Version || a.Hashes != other.Hashes
}
