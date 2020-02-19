package bosun

import (
	"fmt"
	"github.com/mattn/go-zglob"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
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
	AppConfig    *AppConfig        `yaml:"appConfig" json:"appConfig"`
	Files        map[string][]byte `yaml:"-" json:"-"`
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

func LoadAppManifestFromPathAndName(fileOrDir string, name string) (*AppManifest, error) {

	paths := []string{
		fileOrDir,
		filepath.Join(fileOrDir, "bosun.yaml"),
		filepath.Join(fileOrDir, name+".yaml"),
		filepath.Join(fileOrDir, name, name+".yaml"),
		filepath.Join(fileOrDir, name, "bosun.yaml"),
	}

	var out *AppManifest
	for _, path := range paths {
		stat, err := os.Stat(path)
		if err == nil && !stat.IsDir() {
			err = yaml.LoadYaml(path, &out)
			if err != nil {
				return nil, err
			}
			if out.AppMetadata == nil {
				return nil, errors.Errorf("trying to load app manifest for %q from %q, but content was empty", name, path)
			}
			if out.Name != name {
				return nil, errors.Errorf("trying to load app manifest for %q from %q, but file contains manifest for %q", name, path, out.Name)
			}
			out.AppConfig.SetFromPath(path)

			err = out.MakePortable()

			return out, err
		}
	}

	return nil, errors.Errorf("could not find bosun file for %q (tried paths %v)", name, paths)
}

// Save saves the app manifest in the provided directory. It return the path to the bosun file which was saved and contains the manifest.
func (a *AppManifest) Save(dir string) (string, error) {

	hasFiles := len(a.Files) > 0
	if hasFiles {
		dir = filepath.Join(dir, a.Name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", errors.WithStack(err)
		}
	}

	savePath := ""

	if hasFiles {
		for path, bytes := range a.Files {
			path = filepath.Join(dir, path)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return "", errors.WithStack(err)
			}
			if err := ioutil.WriteFile(path, bytes, 0600); err != nil {
				return "", errors.WithStack(err)
			}
		}
		savePath = filepath.Join(dir, "bosun.yaml")
		if err := yaml.SaveYaml(savePath, a); err != nil {
			return "", errors.WithStack(err)
		}
	} else {
		savePath = filepath.Join(dir, a.Name+".yaml")
		if err := yaml.SaveYaml(savePath, a); err != nil {
			return savePath, errors.WithStack(err)
		}
	}

	return savePath, nil
}

func (a *AppManifest) MakePortable() error {

	root := filepath.Dir(a.AppConfig.FromPath)

	if a.Files != nil {
		// already made portable
		return nil
	}

	a.Files = map[string][]byte{}

	var paths []string
	for _, glob := range a.AppConfig.Files {
		glob = filepath.Join(root, glob)
		if !strings.HasPrefix(glob, root) {
			return errors.Errorf("app files cannot be outside the folder containing the bosun.yaml file which contains the app")
		}
		expanded, err := zglob.Glob(glob)
		if err != nil {
			return errors.Wrapf(err, "expand file paths for app %q", a.Name)
		}
		paths = append(paths, expanded...)
	}

	for _, path := range paths {
		stat, err := os.Stat(path)
		if err != nil || stat.IsDir() {
			continue
		}

		b, err := ioutil.ReadFile(path)
		if err != nil {
			return errors.Wrapf(err, "load files for app %q", a.Name)
		}
		path, _ = filepath.Rel(root, path)
		a.Files[path] = b
	}

	return nil
}

func (a *AppManifest) GetTagBasedOnVersionAndBranch() string {
	branch := git.BranchName(a.Branch)
	if a.AppConfig.Branching.IsFeature(branch) {
		return "unstable-" + featureBranchTagRE.ReplaceAllString(strings.ToLower(branch.String()), "-")
	} else if a.AppConfig.Branching.IsDevelop(branch) {
		return "develop"
	} else if a.AppConfig.Branching.IsMaster(branch) {
		return a.AppConfig.Version.String()
	} else if a.AppConfig.Branching.IsRelease(branch) {
		_, releaseVersion, _ := a.AppConfig.Branching.GetReleaseNameAndVersion(branch)
		return fmt.Sprintf("%s-%s", a.Version, releaseVersion)
	} else {
		return "latest"
	}
}
