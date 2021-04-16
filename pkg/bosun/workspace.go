package bosun

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/vcs"
	"github.com/naveego/bosun/pkg/workspace"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

const logConfigs = true

type Workspace struct {
	Path                   string `yaml:"-" json:"-"`
	workspace.Context      `yaml:",inline"`
	Imports                []string                         `yaml:"imports,omitempty" json:"imports"`
	GitRoots               []string                         `yaml:"gitRoots" json:"gitRoots"`
	GithubToken            *command.CommandValue            `yaml:"githubToken" json:"githubToken"`
	ScratchDir             string                           `yaml:"scratchDir" json:"scratchDir"`
	WorkspaceCommands      map[string]*command.CommandValue `yaml:"workspaceCommands"`
	HostIPInMinikube       string                           `yaml:"hostIPInMinikube" json:"hostIpInMinikube"`
	AppStates              workspace.AppStatesByEnvironment `yaml:"appStates" json:"appStates"`
	ClonePaths             map[string]string                `yaml:"clonePaths,omitempty" json:"clonePaths,omitempty"`
	MergedBosunFile        *File                            `yaml:"-" json:"merged"`
	ImportedBosunFiles     map[string]*File                 `yaml:"-" json:"imported"`
	Minikube               *kube.MinikubeConfig             `yaml:"minikube,omitempty" json:"minikube,omitempty"`
	LocalRepos             map[string]*vcs.LocalRepo        `yaml:"localRepos" json:"localRepos"`
	GithubCloneProtocol    string                           `yaml:"githubCloneProtocol"`
	StoryHandlers          []values.Values                  `yaml:"storyHandlers"`
	ClusterKubeconfigPaths map[string]string                `yaml:"clusterKubeconfigPaths"`
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
		w.LocalRepos = map[string]*vcs.LocalRepo{}
	}

	if w.Minikube != nil {

		if w.Minikube.DiskSize == "" {
			w.Minikube.DiskSize = "40g"
		}
		if w.Minikube.Driver == "" {
			w.Minikube.Driver = "virtualbox"
		}
		if w.Minikube.HostIP == "" {
			w.Minikube.HostIP = "192.168.99.1"
		}
	}
	if w.ScratchDir == "" {
		w.ScratchDir = "/tmp/bosun"
	}
	if w.GithubCloneProtocol == "" {
		w.GithubCloneProtocol = "ssh"
	}

	if w.WorkspaceCommands == nil {
		w.WorkspaceCommands = map[string]*command.CommandValue{}
	}

	return nil
}

type State struct {
	Microservices map[string]workspace.AppState
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
			f, openErr := os.OpenFile(defaultPath, os.O_CREATE|os.O_RDWR, 0600)
			if openErr != nil {
				return nil, errors.Errorf("could not create default mergedFragments file: %s", openErr)
			}
			f.Close()
		} else {
			return nil, err
		}
	}

	c := &Workspace{
		Path:               path,
		AppStates:          workspace.AppStatesByEnvironment{},
		ImportedBosunFiles: map[string]*File{},
		MergedBosunFile:    new(File),
	}

	err = yaml.LoadYaml(path, &c)
	if err != nil {
		return nil, errors.Wrap(err, "loading root config")
	}

	return c, nil

}

func LoadWorkspaceWithStaticImports(path string, imports []string) (*Workspace, error) {
	path, _ = filepath.Abs(os.ExpandEnv(path))

	c, err := LoadWorkspaceNoImports(path)
	if err != nil {
		return nil, err
	}

	err = c.importFromPaths(path, imports)

	if err != nil {
		return nil, errors.Wrap(err, "loading imports")
	}

	var syntheticPaths []string
	for _, app := range c.MergedBosunFile.AppRefs {
		if app.Repo != "" {
			for _, root := range c.GitRoots {
				dir := filepath.Join(root, app.Repo)
				bosunFile := filepath.Join(dir, "b.yaml")
				if _, statErr := os.Stat(bosunFile); statErr == nil {
					syntheticPaths = append(syntheticPaths, bosunFile)
				}
			}
		}
	}

	// err = c.importFromPaths(path, syntheticPaths)
	// if err != nil {
	// 	return nil, errors.Errorf("error importing from synthetic paths based on %q: %s", path, err)
	// }

	return c, err
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
				bosunFile := filepath.Join(dir, "b.yaml")
				if _, statErr := os.Stat(bosunFile); statErr == nil {
					syntheticPaths = append(syntheticPaths, bosunFile)
				}
			}
		}
	}

	// err = c.importFromPaths(path, syntheticPaths)
	// if err != nil {
	// 	return nil, errors.Errorf("error importing from synthetic paths based on %q: %s", path, err)
	// }

	return c, err
}

func (w *Workspace) GetWorkspaceCommand(key string, hint string) *command.CommandValue {

	if c, ok := w.WorkspaceCommands[key]; ok {
		return c
	}

	if !cli.IsInteractive() {
		_, _ = fmt.Fprintln(os.Stderr, color.RedString("Your workspace contains no command to generate value %q, but b is not running in an interactive mode.", key))
		_, _ = fmt.Fprintln(os.Stderr, color.RedString("Run the command below in interactive mode to fix this problem."))
		_, _ = fmt.Fprintf(os.Stderr, "%s\n", strings.Join(os.Args, " "))
		os.Exit(0)
	}

	if hint != "" {
		color.Yellow("Workspace command hint:\n%s\n", hint)
	}
	create := cli.RequestConfirmFromUser("Your workspace contains no command to generate value %q, do you want to create one", key)
	if !create {
		color.Red("You need to update your workspace with a command to generate value %q.", key)
		os.Exit(0)
	}

	script := cli.RequestStringFromUser("Enter script")

	commandValue := &command.CommandValue{
		Comment: hint,
		Command: command.Command{
			Script: script,
		},
	}
	w.WorkspaceCommands[key] = commandValue

	return commandValue
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
	log := core.Log.WithField("import_path", path)
	// if logConfigs {
	// 	log.Debug("Importing mergedFragments...")
	// }

	if w.ImportedBosunFiles[path] != nil {
		// if logConfigs {
		// 	log.Debugf("Already imported.")
		// }
		return nil
	}

	c := &File{
		AppRefs: map[string]*Dependency{},
	}

	err := yaml.LoadYaml(path, &c)

	if err != nil {
		return errors.Errorf("yaml error loading %q: %s", path, err)
	}

	c.SetFromPath(path)

	err = w.MergedBosunFile.Merge(c)

	if err != nil {
		return errors.Errorf("merge error loading %q: %s", path, err)
	}

	if logConfigs {
		log.Trace("Import complete.")
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
