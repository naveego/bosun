package bosun

import (
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
)

const logConfigs = false

type Workspace struct {
	Path               string                 `yaml:"-" json:"-"`
	CurrentEnvironment string                 `yaml:"currentEnvironment" json:"currentEnvironment"`
	CurrentPlatform    string                 `yaml:"currentPlatform" json:"currentPlatform"`
	CurrentRelease     string                 `yaml:"currentRelease" json:"currentRelease"`
	Imports            []string               `yaml:"imports,omitempty" json:"imports"`
	GitRoots           []string               `yaml:"gitRoots" json:"gitRoots"`
	ScratchDir         string                 `yaml:"scratchDir" json:"scratchDir"`
	HostIPInMinikube   string                 `yaml:"hostIPInMinikube" json:"hostIpInMinikube"`
	AppStates          AppStatesByEnvironment `yaml:"appStates" json:"appStates"`
	ClonePaths         map[string]string      `yaml:"clonePaths,omitempty" json:"clonePaths,omitempty"`
	MergedBosunFile    *File                  `yaml:"-" json:"merged"`
	ImportedBosunFiles map[string]*File       `yaml:"-" json:"imported"`
	GithubToken        *CommandValue          `yaml:"githubToken" json:"githubToken"`
	ZenhubToken        *CommandValue          `yaml:"zenhubToken" json:"zenhubToken"`
	Minikube           MinikubeConfig         `yaml:"minikube" json:"minikube"`
	LocalRepos         map[string]*LocalRepo  `yaml:"localRepos" json:"localRepos"`
}

func (w *Workspace) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type proxyType Workspace
	var proxy proxyType
	if w != nil {
		proxy = proxyType(*w)
	}
	err := unmarshal(&proxy)
	if err != nil {
		return err
	}

	// migrate to using MinikubeConfig property:
	if proxy.HostIPInMinikube != "" {
		proxy.Minikube.HostIP = proxy.HostIPInMinikube
		proxy.HostIPInMinikube = ""
	}

	*w = Workspace(proxy)

	if w.LocalRepos == nil {
		w.LocalRepos = map[string]*LocalRepo{}
	}

	if w.Minikube.DiskSize == "" {
		w.Minikube.DiskSize = "40g"
	}
	if w.Minikube.Driver == "" {
		w.Minikube.Driver = "virtualbox"
	}
	if w.Minikube.HostIP == "" {
		w.Minikube.HostIP = "192.168.99.1"
	}
	if w.ScratchDir == "" {
		w.ScratchDir = "/tmp/bosun"
	}

	return nil
}

type MinikubeConfig struct {
	HostIP   string `yaml:"hostIP" json:"hostIP"`
	Driver   string `yaml:"driver" json:"driver"`
	DiskSize string `yaml:"diskSize" json:"diskSize"`
}

type State struct {
	Microservices map[string]AppState
}

func LoadWorkspaceNoImports(path string) (*Workspace, error) {
	defaultPath := os.ExpandEnv("$HOME/.bosun/bosun.yaml")
	if path == "" {
		path = defaultPath
	} else {
		path = os.ExpandEnv(path)
	}

	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) && path == defaultPath {
			err = os.MkdirAll(filepath.Dir(defaultPath), 0700)
			if err != nil {
				return nil, errors.Errorf("could not create directory for default mergedFragments file path: %s", err)
			}
			f, err := os.OpenFile(defaultPath, os.O_CREATE|os.O_RDWR, 0600)
			if err != nil {
				return nil, errors.Errorf("could not create default mergedFragments file: %s", err)
			}
			f.Close()
		} else {
			return nil, err
		}
	}

	c := &Workspace{
		Path:               path,
		AppStates:          AppStatesByEnvironment{},
		ImportedBosunFiles: map[string]*File{},
		MergedBosunFile:    new(File),
	}

	err = pkg.LoadYaml(path, &c)
	if err != nil {
		return nil, errors.Wrap(err, "loading root config")
	}

	return c, nil

}

func LoadWorkspace(path string) (*Workspace, error) {

	path, _ = filepath.Abs(os.ExpandEnv(path))

	c, err := LoadWorkspaceNoImports(path)
	if err != nil {
		return nil, err
	}

	err = c.importFromPaths(path, c.Imports)

	if err != nil {
		return nil, errors.Wrap(err, "loading imports")
	}

	var syntheticPaths []string
	for _, app := range c.MergedBosunFile.AppRefs {
		if app.Repo != "" {
			for _, root := range c.GitRoots {
				dir := filepath.Join(root, app.Repo)
				bosunFile := filepath.Join(dir, "bosun.yaml")
				if _, err := os.Stat(bosunFile); err == nil {
					syntheticPaths = append(syntheticPaths, bosunFile)
				}
			}
		}
	}

	err = c.importFromPaths(path, syntheticPaths)
	if err != nil {
		return nil, errors.Errorf("error importing from synthetic paths based on %q: %s", path, err)
	}

	return c, err
}

func (w *Workspace) importFromPaths(relativeTo string, paths []string) error {
	for _, importPath := range paths {
		for _, importPath = range expandPath(relativeTo, importPath) {
			err := w.importFileFromPath(importPath)
			if err != nil {
				return errors.Errorf("error importing fragment relative to %q: %s", relativeTo, err)
			}
		}
	}

	return nil
}

func (w *Workspace) importFileFromPath(path string) error {
	log := pkg.Log.WithField("import_path", path)
	if logConfigs {
		log.Debug("Importing mergedFragments...")
	}

	if w.ImportedBosunFiles[path] != nil {
		if logConfigs {
			log.Debugf("Already imported.")
		}
		return nil
	}

	c := &File{
		AppRefs: map[string]*Dependency{},
	}

	err := pkg.LoadYaml(path, &c)

	if err != nil {
		return errors.Errorf("yaml error loading %q: %s", path, err)
	}

	c.SetFromPath(path)

	err = w.MergedBosunFile.Merge(c)

	if err != nil {
		return errors.Errorf("merge error loading %q: %s", path, err)
	}

	if logConfigs {
		log.Debug("Import complete.")
	}
	w.ImportedBosunFiles[path] = c

	err = w.importFromPaths(c.FromPath, c.Imports)

	return err
}

func (w *Workspace) Save() error {
	data, err := yaml.Marshal(w)
	if err != nil {
		return errors.Wrap(err, "marshalling for save")
	}

	err = ioutil.WriteFile(w.Path, data, 0600)
	if err != nil {
		return errors.Wrap(err, "writing for save")
	}
	return nil
}

// expandPath resolves a path relative to another file's path, including expanding env variables and globs.
func expandPath(relativeToFile, path string) []string {

	path = resolvePath(relativeToFile, path)

	paths, _ := filepath.Glob(path)

	return paths
}

func resolvePath(relativeToFile, path string) string {
	path = os.ExpandEnv(path)
	if !filepath.IsAbs(path) {
		relativeToDir := getDirIfFile(relativeToFile)
		path = filepath.Join(relativeToDir, path)
	}
	return path
}

func getDirIfFile(path string) string {
	if stat, err := os.Stat(path); err == nil {
		if stat.IsDir() {
			return path
		}
		return filepath.Dir(path)
	}
	return path
}
