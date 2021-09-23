package bosun

import (
	"fmt"
	"github.com/manifoldco/promptui"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/vcs"
	"github.com/naveego/bosun/pkg/yaml"
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

var DefaultAppProviderPriority = []string{WorkspaceProviderName, SlotUnstable, SlotStable, FileProviderName}

type AppProvider interface {
	fmt.Stringer
	ProvideApp(req AppProviderRequest) (*App, error)
	GetAllApps() (map[string]*App, error)
}

type AppConfigAppProvider struct {
	appConfigs map[string]*AppConfig
	apps       map[string]*App
	ws         *Workspace
	mu         *sync.Mutex
	log        *logrus.Entry
}

func NewAppConfigAppProvider(ws *Workspace, log *logrus.Entry) AppConfigAppProvider {

	p := AppConfigAppProvider{
		ws:         ws,
		appConfigs: map[string]*AppConfig{},
		apps:       map[string]*App{},
		mu:         new(sync.Mutex),
		log:        log,
	}

	for _, appConfig := range ws.MergedBosunFile.Apps {
		p.appConfigs[appConfig.Name] = appConfig
	}

	return p
}

func (a AppConfigAppProvider) String() string {
	return WorkspaceProviderName
}

func (a AppConfigAppProvider) ProvideApp(req AppProviderRequest) (*App, error) {
	var app *App
	var appConfig *AppConfig
	var ok bool
	a.mu.Lock()
	app, ok = a.apps[req.Name]
	a.mu.Unlock()
	if ok && !req.ForceReload {
		return app, nil
	}
	appConfig, ok = a.appConfigs[req.Name]
	if !ok {
		return nil, ErrAppNotFound(req.Name)
	}

	if req.ForceReload {
		provider := NewFilePathAppProvider(a.log)

		reloadedApp, reloadErr := provider.ProvideApp(AppProviderRequest{Name: req.Name, Path: appConfig.FromPath})
		if reloadErr != nil {
			return nil, errors.Wrap(reloadErr, "failed to reload app from path")
		}
		appConfig = reloadedApp.AppConfig
		a.appConfigs[req.Name] = reloadedApp.AppConfig
	}

	app = &App{
		Provider:  a,
		AppConfig: appConfig,
		isCloned:  true,
	}
	if app.RepoName == "" {
		repoDir, err := git.GetRepoPath(app.FromPath)
		if err == nil {
			app.RepoName = git.GetRepoRefFromPath(repoDir).String()
		}
	}

	if app.RepoName != "" {
		app.Repo = &Repo{
			RepoConfig: RepoConfig{
				Branching: app.Branching.WithDefaults(),
				ConfigShared: core.ConfigShared{
					Name: app.RepoName,
				},
			},
		}

		if app.Repo.LocalRepo, ok = a.ws.LocalRepos[app.FromPath]; !ok {
			localRepoPath, err := git.GetRepoPath(app.FromPath)

			if err == nil {
				app.Repo.LocalRepo = &vcs.LocalRepo{
					Name: app.RepoName,
					Path: localRepoPath,
				}
			}
			a.ws.LocalRepos[app.FromPath] = app.Repo.LocalRepo
		}
	}

	a.mu.Lock()
	a.apps[req.Name] = app
	a.mu.Unlock()

	app.ProviderInfo = a.String()

	return app, nil
}

func (a AppConfigAppProvider) GetAllApps() (map[string]*App, error) {
	out := map[string]*App{}
	for name := range a.appConfigs {
		app, err := a.ProvideApp(AppProviderRequest{Name: name})
		if err != nil {
			return nil, err
		} else {
			out[name] = app
		}
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

func (a ReleaseManifestAppProvider) ProvideApp(req AppProviderRequest) (*App, error) {
	app, ok := a.apps[req.Name]
	if ok {
		return app, nil
	}

	appManifest, found, err := a.release.TryGetAppManifest(req.Name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrAppNotFound(req.Name)
	}

	app = &App{
		Provider:    a,
		AppConfig:   appManifest.AppConfig,
		AppManifest: appManifest,
	}

	if app.RepoName != "" {
		app.Repo = &Repo{
			RepoConfig: RepoConfig{
				Branching: app.Branching.WithDefaults(),
				ConfigShared: core.ConfigShared{
					Name: app.RepoName,
				},
			},
		}
	}

	a.apps[req.Name] = app

	app.ProviderInfo = a.String()

	return app, nil
}

func (a ReleaseManifestAppProvider) GetAllApps() (map[string]*App, error) {
	out := map[string]*App{}
	appManifests, err := a.release.GetAppManifests()
	if err != nil {
		return nil, err
	}
	for name := range appManifests {
		app, getErr := a.ProvideApp(AppProviderRequest{Name: name})

		if getErr != nil {
			if !IsErrAppNotFound(getErr) {
				return nil, getErr
			}
		} else {
			out[name] = app
		}
	}

	return out, nil
}

type ChainAppProvider struct {
	mu              *sync.Mutex
	providers       []AppProvider
	providersByName map[string]AppProvider
	defaultPriority []string
}

func NewChainAppProvider(providers ...AppProvider) ChainAppProvider {
	p := ChainAppProvider{
		mu:              new(sync.Mutex),
		providers:       providers,
		providersByName: map[string]AppProvider{},
	}
	for _, provider := range providers {
		p.defaultPriority = append(p.defaultPriority, provider.String())
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
		app, err := provider.ProvideApp(AppProviderRequest{Name: name})
		if err != nil {
			return nil, err
		}
		return app, nil
	}
	return nil, errors.Errorf("no provider named %q", providerName)
}

func (a ChainAppProvider) GetApp(req AppProviderRequest) (*App, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	providerPriority := req.ProviderPriority
	if len(providerPriority) == 0 {
		providerPriority = a.defaultPriority
	}

	for _, providerName := range providerPriority {
		if provider, ok := a.providersByName[providerName]; ok {
			app, err := provider.ProvideApp(req)
			if err != nil {
				if !IsErrAppNotFound(err) {
					return nil, errors.Wrapf(err, "error while trying to get app using request %s", req)
				}
			} else {
				return app, nil
			}
		}
	}
	return nil, ErrAppNotFound(req.String())
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
				if _, foundApp := out[name]; !foundApp {
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
			app, err := provider.ProvideApp(AppProviderRequest{Name: name})
			if err != nil {
				if !IsErrAppNotFound(err) {
					return nil, errors.Wrapf(err, "error while trying to get app %q", name)
				}
			} else {
				app.Provider = provider
				out = append(out, app)
			}
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

func (a FilePathAppProvider) ProvideApp(req AppProviderRequest) (*App, error) {

	name := req.Name
	path := req.Path
	if !strings.HasSuffix(path, ".yaml") {
		a.log.Debugf("Provider can only get apps if path to bosun file is provided (path was %q).", path)
		return nil, ErrAppNotFound(req.Name)
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

	err := yaml.LoadYaml(bosunFile, &c)
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

			core.Log.Infof("Couldn't find requested app %q in file %q which contained %#v.", name, bosunFile, appNames)

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
	appConfig.SetFileSaver(c)
	appConfig.SetFromPath(c.FromPath)

	repoPath := filepath.Dir(bosunFile)

	app := &App{
		AppConfig: appConfig,
		Repo: &Repo{
			RepoConfig: RepoConfig{
				Branching: appConfig.Branching.WithDefaults(),
			},
			LocalRepo: &vcs.LocalRepo{
				Name: appConfig.RepoName,
				Path: repoPath,
			},
		},
	}

	a.apps[app.Name] = app

	app.ProviderInfo = path

	return app, nil
}

func (a FilePathAppProvider) GetAllApps() (map[string]*App, error) {
	return a.apps, nil
}
