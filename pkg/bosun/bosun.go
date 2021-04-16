package bosun

import (
	"context"
	"fmt"
	"github.com/Azure/go-autorest/autorest/to"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/naveego/bosun/pkg/vault"

	"github.com/naveego/bosun/pkg/brns"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environment"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/mirror"
	"github.com/naveego/bosun/pkg/script"
	"github.com/naveego/bosun/pkg/util/stringsn"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/vcs"
	"github.com/naveego/bosun/pkg/workspace"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Bosun struct {
	mu                   *sync.Mutex
	params               cli.Parameters
	ws                   *Workspace
	file                 *File
	env                  *environment.Environment
	log                  *logrus.Entry
	environmentConfirmed *bool
	repos                map[string]*Repo
	platform             *Platform
	appProvider          ChainAppProvider
	appProviders         []AppProvider
	workspaceAppProvider AppConfigAppProvider
}

func New(params cli.Parameters, ws *Workspace) (*Bosun, error) {
	if params.ProviderPriority == nil {
		params.ProviderPriority = DefaultAppProviderPriority
	}

	b := &Bosun{
		mu:     new(sync.Mutex),
		params: params,
		ws:     ws,
		file:   ws.MergedBosunFile,
		log:    core.Log,
		repos:  map[string]*Repo{},
	}

	if params.DryRun {
		b.log = b.log.WithField("*DRYRUN*", "")
		b.log.Info("DRY RUN")
	}

	err := b.initializeAppProviders()

	if err != nil {
		return nil, err
	}
	//
	// for _, dep := range b.file.AppRefs {
	// 	b.apps[dep.Name] = NewAppFromDependency(dep)
	// }
	//
	// for _, a := range b.file.DeployedApps {
	// 	if a != nil {
	// 		_, err := b.addApp(a)
	// 		if err != nil {
	// 			return nil, errors.Wrapf(err, "add app %q", a.Name)
	// 		}
	// 	}
	// }

	if !params.NoEnvironment {
		envErr := b.configureCurrentEnv()
		if envErr != nil {
			return nil, envErr
		}
	}

	return b, nil
}

func (b *Bosun) initializeAppProviders() error {

	b.workspaceAppProvider = NewAppConfigAppProvider(b.ws)

	b.appProviders = []AppProvider{
		b.workspaceAppProvider,
	}

	p, err := b.GetCurrentPlatform()
	if err != nil {
		return err
	}
	if !p.isAutomationDummy {

		for _, slot := range []string{SlotUnstable, SlotStable} {
			release, releaseErr := p.GetReleaseManifestBySlot(slot)
			if releaseErr != nil {
				b.log.WithError(releaseErr).Errorf("Error loading release manifest for slot %q", slot)
			} else {
				b.appProviders = append(b.appProviders, NewReleaseManifestAppProvider(release))
			}
		}
	}

	b.appProviders = append(b.appProviders, NewFilePathAppProvider(b.log))

	b.appProvider = NewChainAppProvider(b.appProviders...)

	return nil
}

// GetAllVersionsOfAllApps gets all apps from all providers, ignoring provider priority.
func (b *Bosun) GetAllVersionsOfAllApps(providerPriority ...string) (AppList, error) {
	if len(providerPriority) == 0 {
		providerPriority = b.params.ProviderPriority
	}

	apps, err := b.appProvider.GetAllAppsList(providerPriority)

	return apps, err

}

func (b *Bosun) GetAllProviderNames() []string {
	return b.params.ProviderPriority
}

func (b *Bosun) GetAllApps() AppMap {

	apps, err := b.appProvider.GetAllApps(b.params.ProviderPriority)
	if err != nil {
		b.log.WithError(err).Error("Could not get apps.")
		apps = map[string]*App{}
	}

	return apps
}

func (b *Bosun) GetPlatformApps() map[string]*App {

	apps, err := b.appProvider.GetAllApps(b.params.ProviderPriority)
	if err != nil {
		b.log.WithError(err).Error("Could not get apps.")
		apps = map[string]*App{}
	}

	return b.removeNonPlatformAppsFromMap(apps)
}

func (b *Bosun) removeNonPlatformAppsFromMap(in map[string]*App) map[string]*App {
	p, _ := b.GetCurrentPlatform()

	knownApps := p.GetKnownAppMap()

	out := map[string]*App{}
	for k, app := range in {
		if _, ok := knownApps[k]; ok {
			out[app.Name] = app
		}
	}
	return out
}

func (b *Bosun) GetAppDesiredStates() map[string]workspace.AppState {
	return b.ws.AppStates[b.env.Name]
}

func (b *Bosun) GetAppDependencyMap() map[string][]string {
	deps := map[string][]string{}
	for _, app := range b.GetPlatformApps() {
		for _, dep := range app.DependsOn {
			deps[app.Name] = append(deps[app.Name], dep.Name)
		}
	}
	return deps
}

func (b *Bosun) GetAppDependencies(name string) ([]string, error) {

	visited := map[string]bool{}

	return b.getAppDependencies(name, visited)
}

func (b *Bosun) getAppDependencies(name string, visited map[string]bool) ([]string, error) {
	visited[name] = true

	app, err := b.GetApp(name)
	if err != nil {
		return nil, err
	}

	var out []string

	for _, dep := range app.DependsOn {
		if visited[dep.Name] {
			continue
		}
		visited[dep.Name] = true
		out = append(out, dep.Name)

		children, appErrs := b.getAppDependencies(dep.Name, visited)
		if appErrs != nil {
			return nil, errors.Errorf("%s:%s", name, appErrs)
		}
		out = append(out, children...)
	}
	return out, nil
}

func (b *Bosun) GetVaultClient() (*vaultapi.Client, error) {

	vaultClient, err := vault.NewVaultLowlevelClient("", "")

	return vaultClient, err
}

func (b *Bosun) GetScripts() []*script.Script {
	env := b.GetCurrentEnvironment()

	scripts := make([]*script.Script, len(env.Scripts))
	copy(scripts, env.Scripts)
	copy(scripts, b.GetMergedConfig().Scripts)
	for _, app := range b.GetAllApps() {
		for _, script := range app.Scripts {
			script.Name = fmt.Sprintf("%s-%s", app.Name, script.Name)
			scripts = append(scripts, script)
		}
	}

	return scripts
}

func (b *Bosun) GetScript(name string) (*script.Script, error) {
	for _, script := range b.GetScripts() {
		if script.Name == name {
			return script, nil
		}
	}

	return nil, errors.Errorf("no script found with name %q", name)
}

func (b *Bosun) GetApp(name string, providerPriority ...string) (*App, error) {
	if len(providerPriority) == 0 {
		providerPriority = b.params.ProviderPriority
	}
	app, err := b.appProvider.GetApp(name, providerPriority)
	return app, err
}

func (b *Bosun) GetAppFromProvider(appName, providerName string) (*App, error) {
	app, err := b.appProvider.GetAppFromProvider(appName, providerName)
	return app, err
}

func (b *Bosun) GetAppFromWorkspace(appName string) (*App, error) {
	app, err := b.appProvider.GetAppFromProvider(appName, WorkspaceProviderName)
	return app, err
}

func (b *Bosun) ReloadApp(name string) (*App, error) {

	b.mu.Lock()
	defer b.mu.Unlock()

	app, err := b.GetApp(name)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get app named %q", name)
	}

	app, err = b.GetOrAddAppForPath(app.FromPath)
	return app, err
}

func (b *Bosun) GetOrAddAppForPath(path string) (*App, error) {

	provider := NewFilePathAppProvider(b.log)
	app, err := provider.GetApp(path)

	if err != nil {
		return nil, errors.Errorf("could not get app from path %q", path)
	}

	_, err = b.GetApp(app.Name)
	if err != nil {
		ctx := b.NewContext()
		ctx.Log().Infof("Adding app %s at path %s to workspace...", app.Name)

		b.AddImport(path)

		err = b.ws.Save()
		return app, err
	}

	return app, err
}

// Configures the workspace to use the specified environment and cluster, and activates them.
func (b *Bosun) UseStack(stack brns.StackBrn) error {
	var err error

	envConfig, err := b.GetEnvironmentConfig(stack.EnvironmentName)

	if err != nil {
		return err
	}

	b.env, err = envConfig.Builder(b.NewContextWithoutEnvironment()).WithBrn(stack).Build()
	if err != nil {
		return err
	}

	b.params.NoEnvironment = false

	b.ws.CurrentEnvironment = b.env.Name
	b.ws.CurrentCluster = b.env.Cluster().Name
	b.ws.CurrentStack = b.env.Stack().Name

	b.ws.CurrentKubeconfig = b.ws.ClusterKubeconfigPaths[b.ws.CurrentCluster]
	if b.ws.CurrentKubeconfig == "" {
		b.ws.CurrentKubeconfig = os.ExpandEnv("$HOME/.kube/config")
	}

	err = b.ws.Save()
	return err
}

func (b *Bosun) GetEnvironmentAndCluster(stack brns.StackBrn) (*environment.Config, *kube.ClusterConfig, error) {

	var env *environment.Config
	var clusterConfig *kube.ClusterConfig

	p, err := b.GetCurrentPlatform()
	if err != nil {
		return nil, nil, err
	}
	clusters, err := p.GetClusters()
	if err != nil {
		return nil, nil, err
	}

	clusterConfig, err = clusters.GetClusterConfigByBrn(stack)
	if err != nil {
		return nil, nil, err
	}

	if clusterConfig.Environment != stack.EnvironmentName && stack.EnvironmentName != "" {
		return nil, nil, errors.Errorf("found cluster %s using stack brn %s, but the environment of the cluster does not match the requested environment", clusterConfig.Brn, stack)
	}

	envs, err := b.GetEnvironments()
	if err != nil {
		return nil, nil, err
	}
	for _, e := range envs {
		if e.Name == stack.EnvironmentName {
			env = e
			break
		}
	}

	if env == nil {
		return nil, nil, errors.Errorf("could not resolve environment using stack %s", stack)
	}

	return env, clusterConfig, nil
}

func (b *Bosun) GetCurrentEnvironment() *environment.Environment {
	if b.params.NoEnvironment {
		panic(errors.New("bosun was created with NoEnvironment flag set; either the code calling this method or the code which created the bosun instance needs to be changed"))
	}

	return b.env
}

func (b *Bosun) GetCurrentBrn() (stack brns.StackBrn, err error) {

	found := false
	if stack, found = core.GetInternalEnvironmentAndCluster(); found {
		return
	}

	var env *environment.Config

	if b.ws.CurrentEnvironment == "" {
		var envs []*environment.Config
		envs, err = b.GetEnvironments()
		if err != nil {
			return
		}
		switch len(envs) {
		case 0:
			err = errors.New("no environments configured")
			return
		case 1:
			// if only one environment exists, it's the current one
			env = envs[0]
		default:
			var envNames []string
			for _, config := range envs {
				envNames = append(envNames, config.Name)
				for _, cluster := range config.Clusters {
					envNames = append(envNames, cluster.EnvironmentAlias+"(alias)")
				}
			}
			err = errors.Errorf("no environment set (available: %v)", envNames)
			return
		}
	} else {
		env, err = b.GetEnvironmentConfig(b.ws.CurrentEnvironment)
		if err != nil {
			return
		}
	}

	clusterName := b.ws.CurrentCluster
	if clusterName == "" {
		var clusterConfig *kube.ClusterConfig
		clusterConfig, err = env.GetDefaultClusterConfig()
		if err != nil {
			return brns.StackBrn{}, nil
		}
		clusterName = clusterConfig.Name
	}

	stackName := b.ws.CurrentStack
	if stackName == "" {
		stackName = kube.DefaultStackName
	}

	return brns.NewStack(env.Name, clusterName, stackName), nil
}

func (b *Bosun) SetDesiredState(app string, state workspace.AppState) {
	env := b.env
	if b.ws.AppStates == nil {
		b.ws.AppStates = workspace.AppStatesByEnvironment{}
	}
	m, ok := b.ws.AppStates[env.Name]
	if !ok {
		m = workspace.AppStateMap{}
		b.ws.AppStates[env.Name] = m
	}
	m[app] = state
}

func (b *Bosun) Save() error {

	ws := b.ws
	data, err := yaml.Marshal(ws)
	if err != nil {
		return errors.Wrap(err, "marshalling for save")
	}

	err = ioutil.WriteFile(ws.Path, data, 0600)
	if err != nil {
		return errors.Wrap(err, "writing for save")
	}

	return nil
}

func (b *Bosun) SaveAndReload() error {

	path := b.ws.Path

	err := b.Save()
	if err != nil {
		return err
	}

	config, err := LoadWorkspace(path)
	params := b.params

	reloaded, err := New(params, config)
	if err != nil {
		return err
	}

	*b = *reloaded

	return nil
}

func (b *Bosun) SetInWorkspace(path string, value interface{}) error {

	ws := b.ws
	data, err := yaml.Marshal(ws)
	if err != nil {
		return errors.Wrap(err, "marshalling for save")
	}
	v, err := values.ReadValues(data)
	if err != nil {
		panic(err)
	}

	err = v.SetAtPath(path, value)
	if err != nil {
		return err
	}

	yml, err := v.YAML()
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(ws.Path, []byte(yml), 0600)
	if err != nil {
		return errors.Wrap(err, "writing for save")
	}

	return nil
}

func (b *Bosun) GetInWorkspace(path string) (interface{}, error) {

	ws := b.ws
	data, err := yaml.Marshal(ws)
	if err != nil {
		return values.Values{}, errors.Wrap(err, "marshalling for save")
	}
	v, err := values.ReadValues(data)
	if err != nil {
		panic(err)
	}

	return v.GetAtPath(path)
}

func (b *Bosun) GetWorkspace() *Workspace {
	return b.ws
}

func (b *Bosun) GetMergedConfig() File {
	return *b.file
}

func (b *Bosun) AddImport(file string) bool {
	for _, i := range b.ws.Imports {
		if i == file {
			return false
		}
	}
	b.ws.Imports = append(b.ws.Imports, file)

	return true
}

func (b *Bosun) ClearImports() {
	b.ws.Imports = []string{}
}

func (b *Bosun) GetEnvironmentConfig(environmentOrClusterName string) (*environment.Config, error) {

	environmentName := environmentOrClusterName

	p, err := b.GetCurrentPlatform()
	if err != nil {
		return nil, err
	}
	clusters, err := p.GetClusters()
	if err != nil {
		return nil, err
	}
	for _, cluster := range clusters {
		if cluster.Name == environmentOrClusterName || cluster.EnvironmentAlias == environmentOrClusterName {
			environmentName = cluster.Environment
		}
	}

	envs, err := b.GetEnvironments()
	if err != nil {
		return nil, err
	}
	for _, env := range envs {
		if env.Name == environmentName {
			return env, nil
		}
	}

	return nil, errors.Errorf("no environment or cluster named %q", environmentOrClusterName)
}

func (b *Bosun) GetEnvironments() ([]*environment.Config, error) {

	p, err := b.GetCurrentPlatform()
	if err != nil {
		return nil, err
	}
	out, err := p.GetEnvironmentConfigs()
	if err != nil {
		return nil, err
	}

	out = append(out, b.file.Environments...)

	return out, nil
}

func (b *Bosun) GetValueSet(name string) (values.ValueSet, error) {
	for _, vs := range b.file.ValueSets {
		if vs.Name == name {
			return vs, nil
		}
	}
	return values.ValueSet{}, errors.Errorf("no valueSet named %q", name)
}

func (b *Bosun) GetValueSetSlice(names []string) ([]values.ValueSet, error) {
	var out []values.ValueSet
	want := map[string]bool{}
	for _, name := range names {
		want[name] = false
	}

	for _, vs := range b.file.ValueSets {
		if _, wanted := want[vs.Name]; wanted {
			out = append(out, vs)
			want[vs.Name] = true
		}
	}

	for name, found := range want {
		if !found {
			return nil, errors.Errorf("wanted value set %q was not found", name)
		}
	}

	return out, nil
}

func (b *Bosun) GetValueSetsForEnv(env *environment.Config) ([]values.ValueSet, error) {
	vss := map[string]values.ValueSet{}
	for _, vs := range b.file.ValueSets {
		vss[vs.Name] = vs
	}

	var out []values.ValueSet
	for _, name := range env.ValueSetNames {
		vs, ok := vss[name]
		if !ok {
			return nil, errors.Errorf("no valueSet with name %q", name)
		}
		out = append(out, vs)
	}

	return out, nil
}

func (b *Bosun) GetValueSets() values.ValueSets {
	out := make([]values.ValueSet, len(b.file.ValueSets))
	copy(out, b.file.ValueSets)

	mirror.Sort(out, func(a, b *values.ValueSet) bool {
		return a.Name < b.Name
	})

	return out
}

func (b *Bosun) NewContext() BosunContext {

	dir, _ := os.Getwd()

	ctx := BosunContext{
		Bosun: b,

		log: b.log,
	}
	if !b.params.NoEnvironment {
		ctx = ctx.WithEnv(b.GetCurrentEnvironment()).(BosunContext)
	}

	ctx = ctx.WithDir(dir).WithContext(context.Background())

	return ctx

}

func (b *Bosun) NewContextWithoutEnvironment() BosunContext {

	dir, _ := os.Getwd()

	return BosunContext{
		Bosun: b,
		log:   b.log,
	}.WithDir(dir).WithContext(context.Background())
}

func (b *Bosun) GetStableReleaseManifest() (*ReleaseManifest, error) {
	p, err := b.GetCurrentPlatform()
	if err != nil {
		return nil, err
	}

	rm, err := p.GetStableRelease()
	return rm, err
}

func (b *Bosun) GetCurrentPlatform() (*Platform, error) {
	if b.platform != nil {
		return b.platform, nil
	}

	switch len(b.file.Platforms) {
	case 0:
		b.log.Debug("No platforms found, using dummy platform...")

		return b.setCurrentPlatform(&Platform{
			ConfigShared: core.ConfigShared{
				Name:     "Default",
				FromPath: filepath.Join(os.TempDir(), "bosun.platform.yaml"),
			},
			isAutomationDummy: true,
			environmentConfigs: []*environment.Config{
				{
					ConfigShared: core.ConfigShared{
						Name:     "default-env",
						FromPath: filepath.Join(os.TempDir(), "bosun.platform.yaml"),
					},
				},
			},
		})
	case 1:
		return b.setCurrentPlatform(b.file.Platforms[0])
	default:
		if b.ws.CurrentPlatform == "" {
			return nil, errors.New("no current platform selected; use `bosun platform use-platform` to set it")
		}
		for _, p := range b.file.Platforms {
			if p.Name == b.ws.CurrentPlatform {
				return b.setCurrentPlatform(p)
			}
		}
		return nil, errors.Errorf("current platform %q is not found", b.ws.CurrentPlatform)
	}
}

func (b *Bosun) setCurrentPlatform(platform *Platform) (*Platform, error) {
	platform.log = b.log.WithField("cmd", "Platform:"+platform.Name)
	b.platform = platform
	platform.bosun = b
	if !b.platform.isAutomationDummy {

		err := b.platform.LoadChildren()
		if err != nil {
			return nil, err
		}
	}
	platform.RepoName = git.GetRepoRefFromPath(platform.FromPath).String()
	return platform, nil
}

func (b *Bosun) GetCurrentClusterName(env *environment.Config) (string, error) {
	if b.ws.CurrentCluster != "" {
		return b.ws.CurrentCluster, nil
	}

	if cluster, ok := os.LookupEnv(core.EnvCluster); ok {
		return cluster, nil
	}

	if env == nil {
		envx := b.GetCurrentEnvironment()
		env = &envx.Config
	}

	if env.DefaultCluster != "" {
		return env.DefaultCluster, nil
	}

	return env.Clusters[0].Name, nil
}

func (b *Bosun) GetPlatform(name string) (*Platform, error) {
	for _, p := range b.file.Platforms {
		if p.Name == name {
			return b.setCurrentPlatform(p)
		}
	}
	return nil, errors.Errorf("current platform %q is not found", b.ws.CurrentPlatform)
}

func (b *Bosun) GetPlatforms() ([]*Platform, error) {
	out := make([]*Platform, len(b.file.Platforms))
	copy(out, b.file.Platforms)
	mirror.Sort(out, func(a, b *Platform) bool {
		return a.Name < b.Name
	})

	return out, nil
}

func (b *Bosun) UsePlatform(name string) error {
	for _, p := range b.file.Platforms {
		if p.Name == name {
			b.ws.CurrentPlatform = name
			return nil
		}
	}
	return errors.Errorf("no platform named %q", name)
}

func (b *Bosun) UseRelease(name string) error {

	p, err := b.GetCurrentPlatform()
	if err != nil {
		return err
	}
	release, err := p.GetReleaseMetadataByNameOrVersion(name)
	if err != nil {
		return err
	}

	b.ws.CurrentRelease = name

	err = p.SwitchToReleaseBranch(b.NewContext(), release.Branch)
	return err
}

func (b *Bosun) GetGitRoots() []string {
	return b.ws.GitRoots
}

func (b *Bosun) AddGitRoot(s string) {
	b.ws.GitRoots = append(b.ws.GitRoots, s)
}

// TidyWorkspace updates the ClonePaths in the workspace based on the apps found in the imported files.
func (b *Bosun) TidyWorkspace() {
	ctx := b.NewContext()
	log := ctx.Log()
	var importMap = map[string]struct{}{}

	repos := b.GetRepos()

	for _, gitRoot := range b.ws.GitRoots {

		_ = filepath.Walk(gitRoot, func(path string, info os.FileInfo, err error) error {

			relPath, _ := filepath.Rel(gitRoot, path)
			depth := len(strings.Split(relPath, string(os.PathSeparator)))
			// don't go too deep
			if depth > 2 {
				log.Debugf("Depth = %d @ (%s)/(%s) => skipping", depth, gitRoot, relPath)
				return filepath.SkipDir
			}

			if !strings.HasSuffix(path, "bosun.yaml") {
				return nil
			}

			if b.ws.ImportedBosunFiles[path] != nil {
				return nil
			}

			log.Infof("Adding discovered bosun file %s", path)
			b.ws.Imports = append(b.ws.Imports, path)

			return nil
		})
	}

	for _, repo := range repos {
		if repo.CheckCloned() != nil {
			for _, root := range b.ws.GitRoots {
				clonedFolder := filepath.Join(root, repo.Name)
				if _, err := os.Stat(clonedFolder); err != nil {
					if os.IsNotExist(err) {
						log.Debugf("Repo %s not found at %s", repo.Name, clonedFolder)
					} else {
						log.Warnf("Error looking for app %s: %s", repo.Name, err)
					}
				}
				bosunFilePath := filepath.Join(clonedFolder, "bosun.yaml")
				if _, err := os.Stat(bosunFilePath); err != nil {
					if os.IsNotExist(err) {
						log.Warnf("Repo %s seems to be cloned to %s, but there is no bosun.yaml file in that folder", repo.Name, clonedFolder)
					} else {
						log.Warnf("Error looking for bosun.yaml in repo %s: %s", repo.Name, err)
					}
				} else {
					log.Infof("Found cloned repo %s at %s, will add to known local repos.", repo.Name, bosunFilePath)
					localRepo := &vcs.LocalRepo{
						Name: repo.Name,
						Path: clonedFolder,
					}
					b.AddLocalRepo(localRepo)
					b.ws.Imports = stringsn.AppendIfNotPresent(b.ws.Imports, bosunFilePath)
					break
				}
			}
		}
	}

	for _, app := range b.GetAllApps() {
		if app.IsRepoCloned() {
			importMap[app.FromPath] = struct{}{}
			log.Debugf("App %s found at %s", app.Name, app.FromPath)

			repo, err := b.GetRepo(app.RepoName)
			if err != nil || repo.LocalRepo == nil {
				log.Infof("App %s is cloned but its repo is not registered. Registering repo %s...", app.Name, app.RepoName)
				path, repoErr := app.GetLocalRepoPath()
				if repoErr != nil {
					log.WithError(repoErr).Errorf("Error getting local repo path for %s.", app.Name)
				}
				b.AddLocalRepo(&vcs.LocalRepo{
					Name: app.RepoName,
					Path: path,
				})
			}

			continue
		}
		log.Debugf("Found app with no cloned repo: %s from %s", app.Name, app.RepoName)
		for _, root := range b.ws.GitRoots {
			clonedFolder := filepath.Join(root, app.RepoName)
			if _, err := os.Stat(clonedFolder); err != nil {
				if os.IsNotExist(err) {
					log.Debugf("App %s not found at %s", app.Name, clonedFolder)
				} else {
					log.Warnf("Error looking for app %s: %s", app.Name, err)
				}
			}
			bosunFilePath := filepath.Join(clonedFolder, "bosun.yaml")
			if _, err := os.Stat(bosunFilePath); err != nil {
				if os.IsNotExist(err) {
					log.Warnf("App %s seems to be cloned to %s, but there is no bosun.yaml file in that folder", app.Name, clonedFolder)
				} else {
					log.Warnf("Error looking for bosun.yaml for app %s: %s", app.Name, err)
				}
			} else {
				log.Infof("Found bosun.yaml for app ref %s at %s, will add to imports.", app.Name, bosunFilePath)
				b.AddImport(bosunFilePath)
				break
			}
		}

	}

	for _, importPath := range b.ws.Imports {
		if _, err := os.Stat(importPath); os.IsNotExist(err) {
			log.Infof("Import path %s points to a file which no longer exists. It will be removed.", importPath)
		} else {
			importMap[importPath] = struct{}{}
		}
	}
	var imports []string
	for k := range importMap {
		imports = append(imports, k)
	}

	b.ws.Imports = imports
}

func (b *Bosun) configureCurrentEnv() error {

	b.params.NoEnvironment = true
	b.params.NoCluster = true

	stack, err := b.GetCurrentBrn()
	if err != nil {
		return err
	}

	envConfig, err := b.GetEnvironmentConfig(stack.EnvironmentName)
	if err != nil {
		return errors.Errorf("get environment %q: %s", b.ws.CurrentEnvironment, err)
	}

	b.env, err = envConfig.Builder(b.NewContextWithoutEnvironment()).WithBrn(stack).Build()

	if err != nil {
		return err
	}

	err = b.env.ValidateConsistency()

	b.params.NoEnvironment = false
	b.params.NoCluster = false

	return err
}

// Confirm environment checks that the environment has been confirmed by the
// user if the environment is marked as protected.
func (b *Bosun) ConfirmEnvironment() error {

	if !b.env.Protected {
		return nil
	}

	if b.environmentConfirmed == nil {

		envName := b.GetCurrentEnvironment().Name
		if b.params.ConfirmedEnv != "" {
			if b.params.ConfirmedEnv == envName {
				b.environmentConfirmed = to.BoolPtr(true)
			} else {
				return errors.Errorf("The --confirm-env flag was set to %q, but you are targeting the %q environment!\nSwitch environments or unset the flag.", b.params.ConfirmedEnv, b.env.Name)
			}
		} else {
			confirmed := cli.RequestConfirmFromUser("Do you really want to run this command against the %q environment?", envName)
			b.environmentConfirmed = &confirmed
		}
	}

	if *b.environmentConfirmed {
		return nil
	}

	return errors.Errorf("The %q environment is protected, so you must confirm that you want to perform this action.\n(you can do this by setting the --confirm-env to the name of the environment)", b.env.Name)
}

func (b *Bosun) GetTools() []*ToolDef {
	return b.ws.MergedBosunFile.Tools
}
func (b *Bosun) GetTool(name string) (ToolDef, error) {
	for _, tool := range b.ws.MergedBosunFile.Tools {
		if tool.Name == name {
			return *tool, nil
		}
	}
	return ToolDef{}, errors.Errorf("no tool named %q is known", name)
}

func (b *Bosun) RequireTool(name string) error {
	tool, err := b.GetTool(name)
	if err != nil {
		return err
	}

	if _, execErr := tool.GetExecutable(); execErr != nil {
		return errors.Wrapf(execErr, "required tool %q is not installed", name)
	}
	return nil
}

func (b *Bosun) EnsureTool(name string) error {
	tool, err := b.GetTool(name)
	if err != nil {
		return err
	}

	if _, execErr := tool.GetExecutable(); execErr == nil {
		return nil
	}

	installer, err := tool.GetInstaller()
	if err != nil {
		return errors.Errorf("required tool %q is not installable: %s", name, err)
	}
	ctx := b.NewContext()

	err = installer.Execute(ctx)
	return err
}

func (b *Bosun) GetTestSuiteConfigs() []*E2ESuiteConfig {
	return b.ws.MergedBosunFile.TestSuites
}

func (b *Bosun) GetTestSuite(name string) (*E2ESuite, error) {
	var suite *E2ESuite
	var err error
	for _, c := range b.GetTestSuiteConfigs() {
		if c.Name == name {
			ctx := b.NewContext()
			suite, err = NewE2ESuite(ctx, c)
			return suite, err
		}
	}

	return nil, errors.Errorf("no test suite found with name %q", name)
}

func (b *Bosun) GetRepo(name string) (*Repo, error) {
	repos := b.GetRepos()
	for _, repo := range repos {
		if repo.Name == name {
			return repo, nil
		}
	}
	return nil, errors.Errorf("no repo with name %q", name)
}

func (b *Bosun) GetRepos() []*Repo {

	if len(b.repos) == 0 {
		b.repos = map[string]*Repo{}
		for _, app := range b.ws.MergedBosunFile.Apps {
			var repo *Repo
			for _, repoConfig := range b.ws.MergedBosunFile.Repos {
				if app.RepoName == repoConfig.Name {
					var ok bool
					if repo, ok = b.repos[repoConfig.Name]; !ok {
						repo = &Repo{
							RepoConfig: *repoConfig,
							Apps:       map[string]*AppConfig{},
						}
						if lr, repoFound := b.ws.LocalRepos[repo.Name]; repoFound {
							repo.LocalRepo = lr
						}
						b.repos[repo.Name] = repo
					}
					repo.Apps[app.Name] = app
				}
			}
			if repo == nil {
				resolvedApp, err := b.GetAppFromProvider(app.Name, WorkspaceProviderName)
				if err == nil {
					repo = resolvedApp.Repo
					if repo.Apps == nil {
						repo.Apps = map[string]*AppConfig{}
					}
					repo.Apps[app.Name] = app
					b.repos[repo.Name] = repo
				}
			}
		}
		for _, app := range b.GetAllApps() {
			b.repos[app.RepoName] = app.Repo
		}

	}

	var names []string
	for name := range b.repos {
		names = append(names, name)
	}

	sort.Strings(names)

	var out []*Repo

	for _, name := range names {
		out = append(out, b.repos[name])
	}

	return out
}

func (b *Bosun) AddLocalRepo(localRepo *vcs.LocalRepo) {
	if b.ws.LocalRepos == nil {
		b.ws.LocalRepos = map[string]*vcs.LocalRepo{}
	}
	b.ws.LocalRepos[localRepo.Name] = localRepo

	if repo, ok := b.repos[localRepo.Name]; ok {
		repo.LocalRepo = localRepo
	}
}

func (b *Bosun) GetIssueService() (issues.IssueService, error) {

	gc := &git.Config{
	}
	var err error
	gc.GithubToken, err = b.GetGithubToken()

	// zc.ZenhubToken, err = b.GetZenhubToken()
	// if err != nil {
	// 	return nil, errors.Wrap(err, "get zenhub token")
	// }

	gis, err := git.NewIssueService(*gc, core.Log.WithField("cmp", "github"))
	if err != nil {
		return nil, errors.Wrapf(err, "get github issue service with tokens %q", gc.GithubToken)
	}

	return gis, nil

}

func (b *Bosun) GetStoryHandlerConfiguration() []values.Values {
	w := b.GetWorkspace()
	return w.StoryHandlers
}

func (b *Bosun) GetGithubToken() (string, error) {
	ws := b.ws
	var err error

	token, ok := os.LookupEnv("GITHUB_TOKEN")
	if !ok {

		ctx := b.NewContext().WithDir(ws.Path)
		if ws.GithubToken == nil {
			fmt.Println("Github token was not found. Please provide a command that can be run to obtain a github token.")
			fmt.Println("Your token should have scopes [read, read:packages]")
			fmt.Println(`Simple example: echo "9uha09h39oenhsir98snegcu"`)
			fmt.Println(`Better example: cat $HOME/.tokens/github.token"`)
			fmt.Println(`Secure example: lpass show "Tokens/GithubCLIForBosun" --notes"`)
			scriptContent := cli.RequestStringFromUser("ShellExe")

			ws.GithubToken = &command.CommandValue{
				Command: command.Command{
					Script: scriptContent,
				},
			}

			_, err = ws.GithubToken.Resolve(ctx)
			if err != nil {
				return "", errors.Errorf("script failed: %s\nscript:\n%s", err, scriptContent)
			}

			err = b.Save()
			if err != nil {
				return "", errors.Errorf("save failed: %s", err)
			}
		}

		token, err = ws.GithubToken.Resolve(ctx)
		if err != nil {
			return "", err
		}

		err = os.Setenv("GITHUB_TOKEN", token)
		if err != nil {
			return "", err
		}
	}

	return token, nil
}

func (b *Bosun) GetDeployer(repo issues.RepoRef) (*git.Deployer, error) {

	token, err := b.GetGithubToken()
	if err != nil {
		return nil, err
	}
	client := git.NewGithubClient(token)
	svc, err := b.GetIssueService()

	deployer, err := git.NewDeployer(repo, client, svc)
	return deployer, err
}

func (b *Bosun) GetCluster(cluster brns.StackBrn) (*kube.ClusterConfig, error) {

	p, err := b.GetCurrentPlatform()
	if err != nil {
		return nil, err
	}

	return p.GetClusterByBrn(cluster)
}

func (b *Bosun) NormalizeStackBrn(hint string) (brns.StackBrn, error) {

	brn, alternatives, err := b.normalizeStackBrn(hint)

	if err != nil {
		return brn, err
	}

	if brn.String() != hint {
		if len(alternatives) > 0 {
			b.log.Infof("Interpreted %q as %s (alternatives were %v)", hint, brn, alternatives)
		} else {
			b.log.Infof("Interpreted %q as %s", hint, brn)
		}
	}

	return brn, nil
}

func (b *Bosun) normalizeStackBrn(hint string) (brns.StackBrn, []string, error) {

	p, err := b.GetCurrentPlatform()
	if err != nil {
		return brns.StackBrn{}, nil, err
	}

	var envNames []string
	var environmentConfig *environment.Config
	environmentConfigs, err := p.GetEnvironmentConfigs()
	if err != nil {
		return brns.StackBrn{}, nil, err
	}
	for _, ec := range environmentConfigs {
		envNames = append(envNames, ec.Name)
		if ec.Name == hint || strings.HasPrefix(hint, ec.Name) {
			environmentConfig = ec
			break
		}
	}

	approxBrn := brns.TryParseStack(hint)

	var clusterNames []string
	var candidates kube.ClusterConfigs
	var candidateMatches kube.ClusterConfigs

	if environmentConfig == nil {
		candidates, err = p.GetClusters()
		if err != nil {
			return brns.StackBrn{}, nil, errors.Wrap(err, "could get all clusters")
		}
	} else {
		candidates = environmentConfig.Clusters
	}

	for _, c := range candidates {
		clusterNames = append(clusterNames, fmt.Sprintf("%q or %q or %v", c.Name, c.EnvironmentAlias, c.Aliases))
		if c.Environment == approxBrn.EnvironmentOrCluster ||
			c.Environment == approxBrn.Environment ||
			c.Name == approxBrn.EnvironmentOrCluster ||
			c.Name == approxBrn.Cluster ||
			c.EnvironmentAlias == approxBrn.Cluster ||
			stringsn.Contains(approxBrn.Cluster, c.Aliases) {
			candidateMatches = append(candidateMatches, c)
		}
	}

	var clusterConfig *kube.ClusterConfig

	switch len(candidateMatches) {
	case 0:
		break
	case 1:
		clusterConfig = candidateMatches[0]
	default:
		var candidateBrns []string
		for _, c := range candidateMatches {
			candidateBrns = append(candidateBrns, c.Brn.String())
			if c.IsDefaultCluster {
				clusterConfig = c
				break
			}
			if environmentConfig != nil && environmentConfig.DefaultCluster == c.Name {
				clusterConfig = c
				break
			}
		}
	}

	if clusterConfig == nil {
		return brns.StackBrn{}, nil, errors.Errorf("No clusters matched hint %s; environments: %v; clusters: %v", hint, envNames, clusterNames)
	}

	environmentName := clusterConfig.Environment

	stackName := kube.DefaultStackName

	if strings.Contains(hint, "/") {
		parts := strings.Split(hint, "/")
		stackName = parts[1]
	}

	var alternatives []string

	for _, v := range candidateMatches {
		if v.Name != clusterConfig.Name {
			alternatives = append(alternatives, v.Name)
		}
	}

	return brns.NewStack(environmentName, clusterConfig.Name, stackName), alternatives, nil
}

func (b *Bosun) GetCurrentCluster() (*kube.Cluster, error) {

	return b.env.Cluster(), nil
}

func (b *Bosun) GetCurrentStack() (*kube.Stack, error) {

	brn, err := b.GetCurrentBrn()
	if err != nil {
		return nil, err
	}

	c, err := b.GetCurrentCluster()

	if err != nil {
		return nil, err
	}

	stack, err := c.GetStack(brn.StackName)

	return stack, err
}
