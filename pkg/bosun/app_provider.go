package bosun

import (
	"fmt"
	"github.com/manifoldco/promptui"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh/terminal"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const WorkspaceProviderName = "workspace"
const FileProviderName = "file"

var DefaultAppProviderPriority = []string{WorkspaceProviderName, SlotNext, SlotUnstable, SlotStable, FileProviderName}

type AppProvider interface {
	fmt.Stringer
	GetApp(name string) (*App, error)
	GetAllApps() (map[string]*App, error)
}

type AppConfigAppProvider struct {
	appConfigs map[string]*AppConfig
	apps       map[string]*App
	ws         *Workspace
}

func NewAppConfigAppProvider(ws *Workspace) AppConfigAppProvider {

	p := AppConfigAppProvider{
		ws:         ws,
		appConfigs: map[string]*AppConfig{},
		apps:       map[string]*App{},
	}

	for _, appConfig := range ws.MergedBosunFile.Apps {
		p.appConfigs[appConfig.Name] = appConfig
	}

	return p
}

func (a AppConfigAppProvider) String() string {
	return WorkspaceProviderName
}

func (a AppConfigAppProvider) GetApp(name string) (*App, error) {
	var app *App
	var appConfig *AppConfig
	var ok bool
	app, ok = a.apps[name]
	if ok {
		return app, nil
	}
	appConfig, ok = a.appConfigs[name]
	if !ok {
		return nil, ErrAppNotFound(name)
	}
	app = &App{
		Provider:  a,
		AppConfig: appConfig,
		isCloned:  true,
	}
	if app.RepoName == "" {
		repoDir, err := git.GetRepoPath(app.FromPath)
		if err == nil {
			org, repo := git.GetOrgAndRepoFromPath(repoDir)
			app.RepoName = org + "/" + repo
		}
	}

	if app.RepoName != "" {
		app.Repo = &Repo{
			RepoConfig: RepoConfig{
				ConfigShared: ConfigShared{
					Name: app.Name,
				},
			},
		}

		if app.Repo.LocalRepo, ok = a.ws.LocalRepos[app.FromPath]; !ok {
			localRepoPath, err := git.GetRepoPath(app.FromPath)

			if err == nil {
				app.Repo.LocalRepo = &LocalRepo{
					Name: app.RepoName,
					Path: localRepoPath,
				}
			}
			a.ws.LocalRepos[app.FromPath] = app.Repo.LocalRepo
		}
	}

	a.apps[name] = app
	return app, nil
}

func (a AppConfigAppProvider) GetAllApps() (map[string]*App, error) {
	out := map[string]*App{}
	for name := range a.appConfigs {
		app, err := a.GetApp(name)
		if err != nil {
			return nil, err
		}
		out[name] = app
	}
	return out, nil
}

type ReleaseManifestAppProvider struct {
	release *ReleaseManifest
	apps    map[string]*App
}

func NewReleaseManifestAppProvider(release *ReleaseManifest) ReleaseManifestAppProvider {
	return ReleaseManifestAppProvider{
		release: release,
		apps:    map[string]*App{},
	}
}

func (a ReleaseManifestAppProvider) String() string {
	return a.release.Slot
}

func (a ReleaseManifestAppProvider) GetApp(name string) (*App, error) {
	app, ok := a.apps[name]
	if ok {
		return app, nil
	}

	appManifest, err := a.release.GetAppManifest(name)
	if err != nil {
		return nil, err
	}

	app = &App{
		Provider:    a,
		AppConfig:   appManifest.AppConfig,
		AppManifest: appManifest,
	}

	if app.RepoName != "" {
		app.Repo = &Repo{
			RepoConfig: RepoConfig{
				ConfigShared: ConfigShared{
					Name: app.RepoName,
				},
			},
		}
	}

	a.apps[name] = app

	return app, nil
}

func (a ReleaseManifestAppProvider) GetAllApps() (map[string]*App, error) {
	out := map[string]*App{}
	appManifests, err := a.release.GetAppManifests()
	if err != nil {
		return nil, err
	}
	for name := range appManifests {
		out[name], err = a.GetApp(name)
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

type ChainAppProvider struct {
	mu              *sync.Mutex
	providers       []AppProvider
	providersByName map[string]AppProvider
}

func NewChainAppProvider(providers ...AppProvider) ChainAppProvider {
	p := ChainAppProvider{
		mu:              new(sync.Mutex),
		providers:       providers,
		providersByName: map[string]AppProvider{},
	}
	for _, provider := range providers {
		p.providersByName[provider.String()] = provider
	}
	return p
}

func (a ChainAppProvider) String() string {
	var providerNames []string
	for _, provider := range a.providers {
		providerNames = append(providerNames, provider.String())
	}
	return fmt.Sprintf("Chain(%s)", strings.Join(providerNames, " -> "))
}

func (a ChainAppProvider) GetAppFromProvider(name string, providerName string) (*App, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if provider, ok := a.providersByName[providerName]; ok {
		return provider.GetApp(name)
	}
	return nil, errors.Errorf("no provider named %q", providerName)
}

func (a ChainAppProvider) GetApp(name string, providerPriority []string) (*App, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, providerName := range providerPriority {
		if provider, ok := a.providersByName[providerName]; ok {
			app, err := provider.GetApp(name)
			if err == nil {
				return app, nil
			} else {
				if !IsErrAppNotFound(err) {
					return nil, err
				}
			}
		}
	}
	return nil, ErrAppNotFound(name)
}

func (a ChainAppProvider) GetAllApps(providerPriority []string) (map[string]*App, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	out := map[string]*App{}
	for _, providerName := range providerPriority {
		if provider, ok := a.providersByName[providerName]; ok {
			apps, err := provider.GetAllApps()
			if err != nil {
				return nil, err
			}
			for name, app := range apps {
				// use the earliest version returned
				if _, ok := out[name]; !ok {
					out[name] = app
				}
			}
		}
	}
	return out, nil
}

func (a ChainAppProvider) GetAllAppsList(providerNames []string) (AppList, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	out := AppList{}
	for _, providerName := range providerNames {
		provider, ok := a.providersByName[providerName]
		if ok {
			apps, err := provider.GetAllApps()
			if err != nil {
				return nil, err
			}

			for _, app := range apps {
				app.Provider = provider
				out = append(out, app)
			}
		}
	}
	return out, nil
}

func (a ChainAppProvider) GetAllVersionsOfApp(name string, providerNames []string) (AppList, error) {
	out := AppList{}
	for _, providerName := range providerNames {
		provider, ok := a.providersByName[providerName]
		if ok {
			app, err := provider.GetApp(name)
			if err != nil {
				continue
			}
			app.Provider = provider
			out = append(out, app)
		}
	}
	return out, nil
}

type FilePathAppProvider struct {
	apps map[string]*App
	log  *logrus.Entry
}

func NewFilePathAppProvider(log *logrus.Entry) FilePathAppProvider {
	return FilePathAppProvider{
		apps: map[string]*App{},
		log:  log.WithField("provider", FileProviderName),
	}
}

func (a FilePathAppProvider) String() string {
	return FileProviderName
}

func (a FilePathAppProvider) GetApp(path string) (*App, error) {

	return a.GetAppByPathAndName(path, "")
}

func (a FilePathAppProvider) GetAppByPathAndName(path, name string) (*App, error) {
	if !strings.HasSuffix(path, ".yaml") {
		a.log.Debugf("Provider can only get apps if path to bosun file is provided (path was %q).", path)
		return nil, ErrAppNotFound(name)
	}

	bosunFile, _ := filepath.Abs(path)
	if _, err := os.Stat(bosunFile); err != nil {
		a.log.Debugf("Bosun file not found at %q, looking in directory....", path)
		dir := filepath.Dir(bosunFile)
		bosunFile, err = util.FindFileInDirOrAncestors(dir, "bosun.yaml")
		if err != nil {
			return nil, ErrAppNotFound(name + "@" + path)
		}
	}

	c := &File{
		FromPath: bosunFile,
		AppRefs:  map[string]*Dependency{},
	}

	err := pkg.LoadYaml(bosunFile, &c)
	if err != nil {
		return nil, errors.Wrapf(err, "load bosun file from %q", bosunFile)
	}

	var appConfig *AppConfig
	switch len(c.Apps) {
	case 0:
		return nil, errors.Errorf("bosun file %q contained no apps", bosunFile)
	case 1:
		appConfig = c.Apps[0]
	default:
		index := -1
		var appNames []string
		for i, ac := range c.Apps {
			appNames = append(appNames, ac.Name)
			if ac.Name == name {
				index = i
			}
		}

		if index < 0 {

			sort.Strings(appNames)

			if !terminal.IsTerminal(int(os.Stdout.Fd())) {
				return nil, errors.Errorf("multiple apps found in %q, but no user available to choose; try importing the bosun file and referencing the app by name (apps were: %s)", bosunFile, strings.Join(appNames, ", "))
			}

			p := &promptui.Select{
				Label: fmt.Sprintf("Multiple apps found in %q, please choose one.", bosunFile),
				Items: appNames,
			}

			index, _, err = p.Run()
			if err != nil {
				return nil, err
			}
		}

		appConfig = c.Apps[index]
	}
	appConfig.SetParent(c)

	repoPath, _ := git.GetRepoPath(bosunFile)

	app := &App{
		AppConfig: appConfig,
		Repo: &Repo{
			LocalRepo: &LocalRepo{
				Name: appConfig.RepoName,
				Path: repoPath,
			},
		},
	}

	a.apps[app.Name] = app

	return app, nil
}

func (a FilePathAppProvider) GetAllApps() (map[string]*App, error) {
	return a.apps, nil
}
