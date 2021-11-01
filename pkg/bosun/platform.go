package bosun

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/brns"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environment"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/util/stringsn"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/vcs"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	SlotStable       = "stable"
	SlotUnstable     = "unstable"
	SlotPrevious     = "previous"
	PlanFileName     = "plan.yaml"
	ManifestFileName = "manifest.yaml"
)

var (
	UnstableVersion = semver.New("0.0.0-unstable")
	MaxVersion      = semver.Version{Major: math.MaxInt64}
)

// Platform is a collection of releasable apps which work together in a single cluster.
// The platform contains a history of all releases created for the platform.
type Platform struct {
	core.ConfigShared            `yaml:",inline"`
	BosunVersion                 semver.Version                   `yaml:"bosunVersion"`
	DefaultChartRepo             string                           `yaml:"defaultChartRepo"`
	Branching                    git.BranchSpec                   `yaml:"branching"`
	ReleaseBranchFormat_OBSOLETE string                           `yaml:"releaseBranchFormat,omitempty"`
	MasterBranch_OBSOLETE        string                           `yaml:"masterBranch,omitempty"`
	ReleaseDirectory             string                           `yaml:"releaseDirectory" json:"releaseDirectory"`
	AppConfigDirectory           string                           `yaml:"appConfigDirectory,omitempty"`
	EnvironmentDirectory         string                           `yaml:"environmentDirectory,omitempty" json:"environmentDirectory"`
	BundleDirectory              string                           `yaml:"bundleDirectory,omitempty" json:"bundleDirectory"`
	EnvironmentPaths             []string                         `yaml:"environmentPaths,omitempty" json:"environmentPaths"`
	ClusterPaths                 []string                         `yaml:"clusterPaths,omitempty" json:"clusterPaths"`
	EnvironmentRoles             []core.EnvironmentRoleDefinition `yaml:"environmentRoles"`
	ClusterRoles                 []core.ClusterRoleDefinition     `yaml:"clusterRoles"`
	NamespaceRoles               []core.NamespaceRoleDefinition   `yaml:"namespaceRoles"`
	ValueOverrides               *values.ValueSetCollection       `yaml:"valueOverrides,omitempty"`
	ReleaseMetadata              []*ReleaseMetadata               `yaml:"releases" json:"releases"`
	Apps                         PlatformAppConfigs               `yaml:"apps,omitempty"`
	StoryHandlers                map[string]values.Values         `yaml:"storyHandlers"`
	releaseManifests             map[string]*ReleaseManifest      `yaml:"-"`
	environmentConfigs           []*environment.Config            `yaml:"-" json:"-"`
	_clusterConfigs              kube.ClusterConfigs              `yaml:"-" json:"-"`
	bosun                        *Bosun                           `yaml:"-"`
	// set to true if this platform is a dummy created for automation purposes
	isAutomationDummy bool          `yaml:"-"`
	log               *logrus.Entry `yaml:"-"`
	RepoName          string        `yaml:"-"`
}

func (p *Platform) MarshalYAML() (interface{}, error) {
	if p == nil {
		return nil, nil
	}
	type proxy Platform
	px := proxy(*p)

	bosunVersion := core.GetVersion()
	if px.BosunVersion.LessThan(bosunVersion) {
		px.BosunVersion = bosunVersion
	}

	return &px, nil
}

func (p *Platform) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type proxy Platform
	var px proxy
	if p != nil {
		px = proxy(*p)
	}

	err := unmarshal(&px)

	if err == nil {
		*p = Platform(px)
	}

	if p.ReleaseDirectory == "" {
		p.ReleaseDirectory = "releases"
	}

	if p.AppConfigDirectory == "" {
		p.AppConfigDirectory = "apps"
	}
	if p.BundleDirectory == "" {
		p.BundleDirectory = "bundles"
	}

	p.Branching.Master = util.DefaultString(p.Branching.Master, p.MasterBranch_OBSOLETE, "master")
	p.Branching.Develop = util.DefaultString(p.Branching.Develop, "develop")
	p.Branching.Release = util.DefaultString(p.Branching.Release, p.ReleaseBranchFormat_OBSOLETE, "release/{{.Version}}")

	if p.releaseManifests == nil {
		p.releaseManifests = map[string]*ReleaseManifest{}
	}

	if versionErr := core.CheckCompatibility(p.BosunVersion); versionErr != nil {
		fmt.Println()
		color.Red("Platform may be incompatible: %s", versionErr)
		fmt.Println()
	}

	return err
}

func (p *Platform) GetCurrentBranch() (git.BranchName, error) {
	g, err := git.NewGitWrapper(p.FromPath)
	if err != nil {
		return git.BranchName(""), err
	}
	return git.BranchName(g.Branch()), nil
}

func (p *Platform) GetEnvironmentConfigs() ([]*environment.Config, error) {
	if p.environmentConfigs == nil {

		var envVarImport = os.Getenv("BOSUN_BUNDLE_ENV")
		if len(p.EnvironmentPaths) == 0 && envVarImport != "" {
			p.EnvironmentPaths = append(p.EnvironmentPaths, envVarImport)
		}

		for _, path := range p.EnvironmentPaths {

			path = p.ResolveRelative(path)

			config, err := environment.LoadConfig(path)
			if err != nil {
				return nil, errors.Wrapf(err, "load environment from %s", path)
			}
			p.environmentConfigs = append(p.environmentConfigs, config)
		}
	}
	return p.environmentConfigs, nil
}

func (p *Platform) GetClusterByBrn(stack brns.StackBrn) (*kube.ClusterConfig, error) {
	return p._clusterConfigs.GetClusterConfigByBrn(stack)
}

func (p *Platform) GetClusters() (kube.ClusterConfigs, error) {
	return p._clusterConfigs, nil
}

func (p *Platform) GetCurrentRelease() (*ReleaseManifest, error) {
	g, err := git.NewGitWrapper(p.FromPath)
	if err != nil {
		return nil, err
	}
	if !p.Branching.IsRelease(git.BranchName(g.Branch())) {
		return p.GetReleaseManifestBySlot(SlotUnstable)
	}

	return p.GetReleaseManifestBySlot(SlotStable)
}

func (p *Platform) GetPreviousRelease() (*ReleaseManifest, error) {

	stable, err := p.GetStableRelease()
	if err != nil {
		return nil, errors.Wrap(err, "must be able to get stable release to get previous release")
	}

	g, err := git.NewGitWrapper(p.FromPath)

	branches := g.Branches()

	p.log.WithField("branches", branches).Debug("Got listing of previous branches.")

	currentVersion := stable.Version

	var maxPreviousVersion semver.Version
	var maxPreviousBranch git.BranchName
	for _, branch := range branches {
		name := git.BranchName(strings.Trim(branch, " *"))
		if !p.Branching.IsRelease(name) {
			continue
		}
		_, rawVersion, branchErr := p.Branching.GetReleaseNameAndVersion(name)
		if branchErr != nil {
			continue
		}

		version, versionErr := semver.Parse(rawVersion)
		if versionErr != nil {
			continue
		}
		if version == currentVersion {
			continue
		}
		if !version.LessThan(currentVersion) {
			// p.log.Infof("skip %s: %s >= %s (current)", name, version, currentVersion)
			continue
		}
		if version.LessThan(maxPreviousVersion) {
			// p.log.Infof("skip %s: %s < %s (max)", name, version, currentVersion)
			continue
		}
		// p.log.Infof("use %s: %s", name, version)
		maxPreviousVersion = version
		maxPreviousBranch = name
	}

	if maxPreviousBranch == "" {
		return nil, errors.Errorf("no branch is previous to %s", currentVersion)
	}

	// try looking for a "current" release for backwards compatibility
	manifest, err := p.GetReleaseManifestBySlotAndBranch("current", SlotPrevious, maxPreviousBranch)
	if err != nil {
		// if there is no "current" slot, use "stable"
		manifest, err = p.GetReleaseManifestBySlotAndBranch(SlotStable, SlotPrevious, maxPreviousBranch)
	}

	return manifest, err
}

func (p *Platform) GetStableRelease() (*ReleaseManifest, error) {
	return p.GetReleaseManifestBySlot(SlotStable)
}

func (p *Platform) GetUnstableRelease() (*ReleaseManifest, error) {
	return p.GetReleaseManifestBySlot(SlotUnstable)
}

func (p *Platform) MustGetStableRelease() *ReleaseManifest {
	return p.MustGetReleaseManifestBySlot(SlotStable)
}

func (p *Platform) MustGetUnstableRelease() *ReleaseManifest {
	return p.MustGetReleaseManifestBySlot(SlotUnstable)
}

func (p *Platform) GetReleaseMetadataSortedByVersion(descending bool) []*ReleaseMetadata {
	out := make([]*ReleaseMetadata, len(p.ReleaseMetadata))
	copy(out, p.ReleaseMetadata)
	if descending {
		sort.Sort(sort.Reverse(releaseMetadataSorting(out)))
	} else {
		sort.Sort(releaseMetadataSorting(out))
	}

	return out
}

func (p *Platform) MakeReleaseBranchName(version semver.Version) string {
	if version == UnstableVersion {
		return p.Branching.Develop
	}
	name, _ := p.Branching.RenderRelease(git.BranchParts{git.BranchPartVersion: version.String()})
	return name
}


func (p *Platform) SwitchToReleaseBranch(ctx BosunContext, branch string) error {
	log := ctx.Log()

	platformRepoPath, err := git.GetRepoPath(p.FromPath)
	if err != nil {
		return err
	}

	localRepo := &vcs.LocalRepo{Path: platformRepoPath}
	if localRepo.GetCurrentBranch() == git.BranchName(branch) {
		log.Debugf("Repo at %s is already on branch %s.", platformRepoPath, branch)
		return nil
	}

	if localRepo.IsDirty() {
		return errors.Errorf("repo at %s is dirty, commit or stash your changes before adding it to the release", localRepo.Path)
	}

	log.Debug("Checking if release branch exists...")

	parentBranch := localRepo.GetCurrentBranch().String()

	branchExists, err := localRepo.DoesBranchExist(ctx, branch)
	if err != nil {
		return err
	}
	if branchExists {
		log.Info("Release branch already exists, switching to it.")
		err = localRepo.SwitchToBranchAndPull(ctx.Services(), branch)
		if err != nil {
			return errors.Wrap(err, "switching to release branch")
		}
	} else {
		log.Info("Creating release branch...")
		err = localRepo.SwitchToNewBranch(ctx, parentBranch, branch)
		if err != nil {
			return errors.Wrap(err, "creating release branch")
		}
	}

	return nil

}


// Save saves the platform. This will update the file containing the platform,
// and will write out any release manifests which have been loaded in this platform.
func (p *Platform) Save(ctx BosunContext) error {

	var err error

	if ctx.GetParameters().DryRun {
		ctx.Log().Warn("Skipping platform save because dry run flag was set.")
	}

	ctx.Log().Info("Saving platform...")
	sort.Sort(sort.Reverse(releaseMetadataSorting(p.ReleaseMetadata)))

	manifests := p.releaseManifests

	// save the release manifests
	for _, manifest := range manifests {

		err = manifest.Save(ctx)
		if err != nil {
			return err
		}
	}

	appConfigDir := p.ResolveRelative(p.AppConfigDirectory)
	_ = os.MkdirAll(appConfigDir, 0700)

	for _, app := range p.Apps {
		appPath := filepath.Join(appConfigDir, app.Name+".yaml")
		if err = yaml.SaveYaml(appPath, app); err != nil {
			return errors.Wrapf(err, "save app %q to %q", app.Name, appPath)
		}
	}

	apps := p.Apps
	p.Apps = nil
	err = p.FileSaver.Save()
	p.Apps = apps

	if err != nil {
		return err
	}

	ctx.Log().Info("Platform saved.")
	return nil
}

func writeYaml(path string, value interface{}) error {
	y, err := yaml.Marshal(value)
	if err != nil {
		return errors.Wrapf(err, "marshal value to be written to %q", path)
	}

	err = ioutil.WriteFile(path, y, 0600)
	return err
}

type AppsAndDependencies struct {
	Apps             map[string]*App
	Dependencies     map[string][]string
	TopologicalOrder []string
}

func (p *Platform) GetAppsAndDependencies(b *Bosun, req CreateDeploymentPlanRequest) (AppsAndDependencies, error) {
	apps := map[string]*App{}
	dependencies := map[string][]string{}
	out := AppsAndDependencies{
		Apps:         apps,
		Dependencies: dependencies,
	}
	err := p.buildAppsAndDepsRec(b, req, req.Apps, apps, dependencies)
	if err != nil {
		return out, err
	}

	topology, err := GetDependenciesInTopologicalOrder(dependencies, req.Apps...)

	if err != nil {
		return out, errors.Wrapf(err, "apps could not be sorted in dependency order (apps: %#v)", req.Apps)
	}

	out.TopologicalOrder = topology

	return out, nil
}

func (p *Platform) buildAppsAndDepsRec(b *Bosun, req CreateDeploymentPlanRequest, appNames []string, apps map[string]*App, deps map[string][]string) error {
	ctx := b.NewContext()
	for len(appNames) > 0 {
		appName := appNames[0]
		appNames = appNames[1:]

		if _, added := apps[appName]; added {
			continue
		}

		platformApp, err := p.getPlatformApp(appName, ctx)
		if err != nil {
			return err
		}
		var app *App

		appReq := req.AppOptions[appName]
		appReq.Name = appName
		app, err = b.ProvideApp(appReq)
		if err != nil {
			return errors.Wrapf(err, "get app %q from anywhere", appName)
		}

		apps[appName] = app
		appDeps := deps[app.Name]

		for _, dep := range app.AppConfig.DependsOn {
			appDeps = stringsn.AppendIfNotPresent(appDeps, dep.Name)
		}
		for _, dep := range platformApp.Dependencies {
			appDeps = stringsn.AppendIfNotPresent(appDeps, dep)
		}
		deps[app.Name] = appDeps
		if req.AutomaticDependencies {
			err = p.buildAppsAndDepsRec(b, req, appDeps, apps, deps)
		}
	}
	return nil
}

func (p *Platform) GetReleaseMetadataByVersion(v semver.Version) (*ReleaseMetadata, error) {
	for _, rm := range p.ReleaseMetadata {
		if rm.Version.Equal(v) {
			return rm, nil
		}
	}

	return nil, errors.Errorf("this platform has no release with version %q", v)
}

func (p *Platform) GetPreviousReleaseMetadata(version semver.Version) *ReleaseMetadata {

	for _, r := range p.GetReleaseMetadataSortedByVersion(true) {
		if r.Version.LessThan(version) {
			return r
		}
	}

	return nil
}

func (p *Platform) GetManifestDirectoryPath(name string) string {
	dir := filepath.Join(filepath.Dir(p.FromPath), p.ReleaseDirectory, name)
	return dir
}

func (p *Platform) MustGetReleaseManifestBySlot(name string) *ReleaseManifest {
	releaseMetadata, err := p.GetReleaseManifestBySlot(name)
	if err != nil {
		color.Red("Could not get release %q:\n%+v", err)
		os.Exit(1)
	}
	return releaseMetadata
}

func (p *Platform) GetReleaseManifestBySlot(slot string) (*ReleaseManifest, error) {
	branch, err := p.GetCurrentBranch()
	if err != nil {
		return nil, err
	}
	return p.GetReleaseManifestBySlotAndBranch(slot, slot, branch)
}

func (p *Platform) GetReleaseManifestBySlotAndBranch(fromSlot string, asSlot string, branch git.BranchName) (*ReleaseManifest, error) {

	key := fmt.Sprintf("%s->%s@%s", fromSlot, asSlot, branch)

	if manifest, ok := p.releaseManifests[key]; ok {
		return manifest, nil
	}

	g, err := git.NewGitWrapper(p.FromPath)
	if err != nil {
		return nil, err
	}

	var dir string
	var manifest *ReleaseManifest

	if g.Branch() != string(branch) {

		err = g.Fetch()
		if err != nil {
			return nil, err
		}
		worktree, worktreeErr := g.Worktree(branch)
		if worktreeErr != nil {
			return nil, worktreeErr
		}

		defer worktree.Dispose()

		dir = worktree.ResolvePath(p.GetManifestDirectoryPath(fromSlot))
	} else {
		dir = p.GetManifestDirectoryPath(fromSlot)
	}

	if _, err = os.Stat(dir); err != nil {
		return nil, err
	}

	manifestPath := filepath.Join(dir, "manifest.yaml")

	b, err := ioutil.ReadFile(manifestPath)
	if err != nil {
		return nil, errors.Wrapf(err, "read manifest for slot %q from branch %q", fromSlot, branch)
	}

	err = yaml.Unmarshal(b, &manifest)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal manifest for slot %q from branch %q", fromSlot, branch)
	}

	manifest.dir = dir

	manifest.Platform = p

	manifest.repoRef = git.GetRepoRefFromPath(p.FromPath)

	if p.releaseManifests == nil {
		p.releaseManifests = map[string]*ReleaseManifest{}
	}
	p.releaseManifests[key] = manifest
	manifest.Slot = asSlot

	currentBranch, err := p.GetCurrentBranch()
	if err != nil {
		return nil, err
	}

	p.log.Debugf("loading release from slot %s into slot %s on branch %s", fromSlot, asSlot, currentBranch)

	if p.Branching.IsRelease(currentBranch) && asSlot == SlotStable {
		p.log.Debugf("marking release as current")
		manifest.isCurrentRelease = true
	}

	_, err = manifest.GetAppManifests()

	return manifest, err
}

func (p *Platform) IncludeApp(ctx BosunContext, config *PlatformAppConfig) error {

	app, err := ctx.Bosun.GetApp(config.Name)
	if err != nil {
		return err
	}

	appManifest, err := app.GetManifest(ctx)
	if err != nil {
		return err
	}

	var found bool
	for i, knownApp := range p.Apps {
		if knownApp.Name == config.Name {
			found = true
			p.Apps[i] = config
			break
		}
	}
	if !found {
		p.Apps = append(p.Apps, config)
	}

	manifest, err := p.GetUnstableRelease()
	if err != nil {
		return err
	}
	err = manifest.AddOrReplaceApp(appManifest, false)

	return err
}

func (p *Platform) AddAppValuesForCluster(ctx BosunContext, appName string, overridesName string, matchMap filter.MatchMapConfig) error {

	appConfig, err := p.getPlatformApp(appName, ctx)
	if err != nil {
		return err
	}

	app, err := p.bosun.GetApp(appName)
	if err != nil {
		return err
	}

	if appConfig.ValueOverrides == nil {
		appConfig.ValueOverrides = &values.ValueSetCollection{}
	}

	var valueSet values.ValueSet
	index := -1
	for i, vs := range appConfig.ValueOverrides.ValueSets {
		if vs.Name == overridesName {
			valueSet = vs
			index = i
		}
	}
	if index < 0 {
		index = len(appConfig.ValueOverrides.ValueSets)
		appConfig.ValueOverrides.ValueSets = append(appConfig.ValueOverrides.ValueSets, valueSet)
	}

	appConfig.ValueOverrides.ValueSets = append(appConfig.ValueOverrides.ValueSets)

	valueSet = valueSet.WithValues(app.Values.DefaultValues)
	valueSet.Files = nil
	valueSet.Roles = nil
	valueSet.Name = overridesName
	valueSet.ExactMatchFilters = matchMap

	appConfig.ValueOverrides.ValueSets[index] = valueSet

	return nil
}

func (p *Platform) GetValueSetCollection() values.ValueSetCollection {
	if p.ValueOverrides == nil {
		return values.NewValueSetCollection()
	}
	return *p.ValueOverrides
}

// RefreshApp checks updates the specified slot with the specified branch of the named app.
func (p *Platform) RefreshApp(ctx BosunContext, name string, branch string, slot string) error {
	releaseManifest, err := p.GetReleaseManifestBySlot(slot)
	if err != nil {
		return err
	}

	return releaseManifest.RefreshApp(ctx, name, branch)

}

func (p *Platform) GetAppManifestByNameFromSlot(appName string, slot string) (*AppManifest, error) {

	release, err := p.GetReleaseManifestBySlot(slot)
	if err != nil {
		return nil, err
	}

	appManifest, err := release.GetAppManifest(appName)
	return appManifest, err
}

func (p *Platform) GetStableAppMetadata(name string) (*AppMetadata, error) {
	manifest, err := p.GetStableRelease()
	if err != nil {
		return nil, err
	}

	if appMetadata, ok := manifest.GetAllAppMetadata()[name]; ok {
		return appMetadata, nil
	}

	return nil, errors.Errorf("no app %q in stable release", name)
}

func (p *Platform) SetReleaseManifest(slot string, manifest *ReleaseManifest) {

	if slot == SlotPrevious {
		panic("cannot change previous releases")
	}

	manifest.Slot = slot
	p.releaseManifests[slot] = manifest.MarkDirty()
	var updatedMetadata []*ReleaseMetadata
	replaced := false
	for _, metadata := range p.ReleaseMetadata {
		if metadata.Version == manifest.Version {
			updatedMetadata = append(updatedMetadata, manifest.ReleaseMetadata)
			replaced = true
		} else {
			updatedMetadata = append(updatedMetadata, metadata)
		}
	}
	if !replaced {
		updatedMetadata = append(updatedMetadata, manifest.ReleaseMetadata)
	}
	p.ReleaseMetadata = updatedMetadata
}

func (p *Platform) GetApps(ctx filter.MatchMapArgContainer) PlatformAppConfigs {

	matchArgs := ctx.GetMatchMapArgs()

	var out []*PlatformAppConfig
	for _, app := range p.Apps {
		if len(matchArgs) == 0 || app.TargetFilters.Matches(matchArgs) {
			out = append(out, app)
		}
	}

	return out
}

func (p *Platform) getPlatformApp(appName string, ctx filter.MatchMapArgContainer) (*PlatformAppConfig, error) {
	for _, a := range p.GetApps(ctx) {
		if a.Name == appName {
			return a, nil
		}
	}
	return nil, errors.Errorf("no platform app config with name %q matched filters %s", appName, ctx.GetMatchMapArgs())
}

func (p *Platform) LoadChildren() error {
	appPathDir := p.ResolveRelative(p.AppConfigDirectory)
	_ = os.MkdirAll(appPathDir, 0700)
	appPaths, err := ioutil.ReadDir(appPathDir)
	if err != nil {
		return err
	}

	for _, file := range appPaths {
		var app PlatformAppConfig
		appPath := filepath.Join(appPathDir, file.Name())
		err = yaml.LoadYaml(appPath, &app)
		if err != nil {
			return errors.Wrapf(err, "load platform app config from %s", appPath)
		}
		p.Apps = append(p.Apps, &app)
	}

	environmentPaths := p.EnvironmentPaths[:]
	environments := map[string]*environment.Config{}

	// environmentDirs, _ := filepath.Glob(filepath.Join(p.EnvironmentDirectory, "*"))
	//
	// for _, dir := range environmentDirs {
	// 	envName := filepath.Base(dir)
	// 	environmentPaths = append(environmentDirs, filepath.Join(dir, fmt.Sprintf("%s.bosun.yaml", envName)));
	// }

	for _, path := range environmentPaths {
		path = p.ResolveRelative(path)
		if _, ok := environments[path]; ok {
			continue
		}
		var config *environment.Config
		err = yaml.LoadYaml(path, &config)
		if err != nil {
			p.log.WithError(err).Error("could not load environment config")
			continue
		}
		config.SetFromPath(path)
		p.environmentConfigs = append(p.environmentConfigs, config)

		for _, clusterConfig := range config.Clusters {
			clusterConfig.Environment = config.Name
			clusterConfig.Brn = brns.NewStack(config.Name, clusterConfig.Brn.ClusterName, clusterConfig.Brn.EnvironmentName)
			clusterConfig.PullSecrets = config.PullSecrets

			p._clusterConfigs = append(p._clusterConfigs, clusterConfig)
		}
	}

	for _, pattern := range p.ClusterPaths {

		clusterPaths, _ := filepath.Glob(p.ResolveRelative(pattern))
		for _, clusterPath := range clusterPaths {
			var clusterConfig *kube.ClusterConfig

			err = yaml.LoadYaml(clusterPath, &clusterConfig)
			if err != nil {
				p.log.WithError(err).Error("could not load environment config")
				continue
			}

			clusterConfig.SetFromPath(clusterPath)

			for _, stackTemplate := range clusterConfig.StackTemplates {
				stackTemplate.SetFromPath(clusterPath)
			}

			p._clusterConfigs = append(p._clusterConfigs, clusterConfig)
		}
	}

	return nil
}

func (p *Platform) GetPlatformAppUnfiltered(appName string) (*PlatformAppConfig, error) {
	for _, app := range p.Apps {
		if app.Name == appName {
			return app, nil
		}
	}
	return nil, errors.Errorf("no platform app config found with name %q, even disregarding filters", appName)
}

func (p *Platform) GetKnownAppMap() map[string]*PlatformAppConfig {
	out := map[string]*PlatformAppConfig{}
	for _, app := range p.Apps {
		out[app.Name] = app
	}
	return out
}

func (p *Platform) GetDeploymentsDir() string {
	dir := filepath.Join(filepath.Dir(p.FromPath), "deployments")
	_ = os.MkdirAll(dir, 0700)
	return dir
}

func (p *Platform) CreateHotfix(ctx BosunContext, version semver.Version) (*ReleaseManifest, error) {
	var err error

	_, err = p.GetStableRelease()
	if err != nil {
		return nil, err
	}

	ctx.Log().Info("Creating new release plan.")

	branch := p.MakeReleaseBranchName(version)
	if err = p.SwitchToReleaseBranch(ctx, branch); err != nil {
		return nil, err
	}

	manifest, err := p.GetCurrentRelease()
	if err != nil {
		return nil, err
	} else {
		ctx.Log().Infof("Using release %s as current release.", manifest)
		manifest.ReleaseMetadata.Version = version
	}

	ctx.Log().Infof("Created new release plan %s.", manifest)

	p.SetReleaseManifest(SlotStable, manifest)

	err = p.Save(ctx)
	return manifest, err
}

type StatusDiff struct {
	From string
	To   string
}

func NewVersion(version string) (semver.Version, error) {
	v := semver.Version{}

	if err := v.Set(version); err != nil {
		return v, err
	}

	return v, nil
}

type AppHashes struct {
	// Commit hash that this app manifest was created from.
	// Empty if the app is stored in the platform repo.
	Commit string `yaml:"commit,omitempty"`
	// AppConfig hash, created by hashing the bosun file.
	AppConfig string `yaml:"appConfig,omitempty"`
	// Files hash, created by hashing all files included in the app manifest by the bosunfile.
	Files string `yaml:"files,omitempty"`
}

func (a AppHashes) String() string {
	var out []string
	if a.AppConfig != "" {
		out = append(out, fmt.Sprintf("app:%s", stringsn.Truncate(a.AppConfig, 5)))
	}
	if a.Commit != "" {
		out = append(out, fmt.Sprintf("commit:%s", stringsn.Truncate(a.Commit, 5)))
	}
	if a.Files != "" {
		out = append(out, fmt.Sprintf("files:%s", stringsn.Truncate(a.Files, 5)))
	}
	if len(out) == 0 {
		return "unknown"
	}

	return strings.Join(out, ",")
}

func (a AppHashes) Changes(other AppHashes) (string, bool) {
	var out []string
	if a.AppConfig != other.AppConfig {
		out = append(out, "app")
	}
	if a.Commit != other.Commit {
		out = append(out, "commit")
	}
	if a.Files != other.Files {
		out = append(out, "files")
	}
	if len(out) == 0 {
		return "", false
	}
	return strings.Join(out, ","), true
}

// Summarize returns a hash of the hashes, for when you don't need to tell where the changes are.
func (a AppHashes) Summarize() string {
	var out []string
	if a.AppConfig != "" {
		out = append(out, fmt.Sprintf("app:%s", stringsn.Truncate(a.AppConfig, 5)))
	}
	if a.Commit != "" {
		out = append(out, fmt.Sprintf("commit:%s", stringsn.Truncate(a.Commit, 5)))
	}
	if a.Files != "" {
		out = append(out, fmt.Sprintf("files:%s", stringsn.Truncate(a.Files, 5)))
	}
	if len(out) == 0 {
		return "unknown"
	}

	return strings.Join(out, ",")
}