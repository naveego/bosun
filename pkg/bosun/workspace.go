package bosun

import (
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
)

const logConfigs = true

type Workspace struct {
	Path               string                 `yaml:"-"`
	CurrentEnvironment string                 `yaml:"currentEnvironment"`
	Imports            []string               `yaml:"imports,omitempty"`
	GitRoots           []string               `yaml:"gitRoots"`
	Release            string                 `yaml:"release"`
	HostIPInMinikube   string                 `yaml:"hostIPInMinikube"`
	AppStates          AppStatesByEnvironment `yaml:"appStates"`
	ClonePaths         map[string]string      `yaml:"clonePaths"`
	MergedBosunFile    *File                  `yaml:"-"`
	ImportedBosunFiles map[string]*File       `yaml:"-"`
}

type State struct {
	Microservices map[string]AppState
}

func LoadWorkspace(path string) (*Workspace, error) {
	defaultPath := os.ExpandEnv("$HOME/.bosun/bosun.yaml")
	if path == "" {
		path = defaultPath
	} else {
		path = os.ExpandEnv(path)
	}

	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) && path == defaultPath {
			err = os.MkdirAll(filepath.Dir(defaultPath), 0600)
			if err != nil {
				return nil, errors.Errorf("could not create directory for default mergedFragments file path: %s", err)
			}
			f, err := os.Open(defaultPath)
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

func (r *Workspace) importFromPaths(relativeTo string, paths []string) error {
	for _, importPath := range paths {
		for _, importPath = range expandPath(relativeTo, importPath) {
			err := r.importFileFromPath(importPath)
			if err != nil {
				return errors.Errorf("error importing fragment relative to %q: %s", relativeTo, err)
			}
		}
	}

	return nil
}

func (r *Workspace) importFileFromPath(path string) error {
	log := pkg.Log.WithField("import_path", path)
	if logConfigs {
		log.Debug("Importing mergedFragments...")
	}

	if r.ImportedBosunFiles[path] != nil {
		if logConfigs {
			log.Debugf("Already imported.")
		}
		return nil
	}

	c := &File{
		FromPath: path,
		AppRefs:  map[string]*Dependency{},
	}

	err := pkg.LoadYaml(path, &c)

	if err != nil {
		return errors.Errorf("yaml error loading %q: %s", path, err)
	}

	for _, e := range c.Environments {
		e.SetFromPath(path)
	}

	for _, m := range c.Apps {
		m.SetFragment(c)
	}

	for _, m := range c.AppRefs {
		m.FromPath = path
	}

	for _, m := range c.Releases {
		m.SetParent(c)
	}

	err = r.MergedBosunFile.Merge(c)

	if err != nil {
		return errors.Errorf("merge error loading %q: %s", path, err)
	}

	if logConfigs {
		log.Debug("Import complete.")
	}
	r.ImportedBosunFiles[path] = c

	err = r.importFromPaths(c.FromPath, c.Imports)

	return err
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