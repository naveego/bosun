package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg/git"
	"strings"
)

type AppProvider interface {
	fmt.Stringer
	GetApp(name string) (*App, error)
	GetAllApps() (map[string]*App, error)
}

type AppConfigAppProvider struct {
	appConfigs map[string]*AppConfig
	apps       map[string]*App
	repos      map[string]*Repo
	ws         *Workspace
}

func NewAppConfigAppProvider(ws *Workspace) (AppConfigAppProvider, error) {

	p := AppConfigAppProvider{
		ws:         ws,
		appConfigs: map[string]*AppConfig{},
		apps:       map[string]*App{},
		repos:      map[string]*Repo{},
	}

	for _, appConfig := range ws.MergedBosunFile.Apps {
		p.appConfigs[appConfig.Name] = appConfig
	}

	return p, nil
}

func (a AppConfigAppProvider) String() string {
	return "AppConfig"
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
		var repo *Repo
		// find or add repo for app
		repo, ok = a.repos[app.RepoName]
		if !ok {
			repo = &Repo{
				RepoConfig: RepoConfig{
					ConfigShared: ConfigShared{
						Name: app.Name,
					},
				},
			}
			a.repos[app.RepoName] = repo
		}

		app.Repo = repo

		if repo.LocalRepo, ok = a.ws.LocalRepos[app.RepoName]; !ok {
			localRepoPath, err := git.GetRepoPath(app.FromPath)
			if err == nil {
				repo.LocalRepo = &LocalRepo{
					Name: app.RepoName,
					Path: localRepoPath,
				}
			}
			a.ws.LocalRepos[app.RepoName] = repo.LocalRepo
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
	return fmt.Sprintf("Release %s (%s)", a.release.Name, a.release.Version)
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
	providers []AppProvider
}

func NewChainAppProvider(providers ...AppProvider) ChainAppProvider {
	return ChainAppProvider{
		providers: providers,
	}
}

func (a ChainAppProvider) String() string {
	var providerNames []string
	for _, provider := range a.providers {
		providerNames = append(providerNames, provider.String())
	}
	return fmt.Sprintf("Chain(%s)", strings.Join(providerNames, " -> "))
}

func (a ChainAppProvider) GetApp(name string) (*App, error) {
	for _, provider := range a.providers {
		app, err := provider.GetApp(name)
		if err == nil {
			return app, nil
		} else {
			if !IsErrAppNotFound(err) {
				return nil, err
			}
		}
	}
	return nil, ErrAppNotFound(name)
}

func (a ChainAppProvider) GetAllApps() (map[string]*App, error) {
	out := map[string]*App{}
	for _, provider := range a.providers {
		apps, err := provider.GetAllApps()
		if err != nil {
			return nil, err
		}
		for name, app := range apps {
			// use the earliest version returned
			if _, ok := out[name]; ok {
				out[name] = app
			}
		}
	}
	return out, nil
}

func (a ChainAppProvider) GetAllAppVersions() (map[string][]*App, error) {
	out := map[string][]*App{}
	for _, provider := range a.providers {
		apps, err := provider.GetAllApps()
		if err != nil {
			return nil, err
		}

		for name, app := range apps {
			out[name] = append(out[name], app)
		}
	}
	return out, nil
}
