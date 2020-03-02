package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environment"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/workspace"
	"github.com/pkg/errors"
	"regexp"
	"sort"
)

//
// type ReleaseConfig struct {
// 	Name        string           `yaml:"name" json:"name"`
// 	Version     string           `yaml:"version" json:"version"`
// 	Description string           `yaml:"description" json:"description"`
// 	FromPath    string           `yaml:"fromPath" json:"fromPath"`
// 	Manifest    *ReleaseManifest `yaml:"manifest"`
// 	// AppReleaseConfigs map[string]*AppReleaseConfig `yaml:"apps" json:"apps"`
// 	Exclude     map[string]bool        `yaml:"exclude,omitempty" json:"exclude,omitempty"`
// 	IsPatch     bool                   `yaml:"isPatch,omitempty" json:"isPatch,omitempty"`
// 	Parent      *File                  `yaml:"-" json:"-"`
// 	BundleFiles map[string]*BundleFile `yaml:"bundleFiles,omitempty"`
// }

type BundleFile struct {
	App     string `yaml:"namespace"`
	Path    string `yaml:"path"`
	Content []byte `yaml:"-"`
}

//
// func (r *ReleaseConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
// 	type rcm ReleaseConfig
// 	var proxy rcm
// 	err := unmarshal(&proxy)
// 	if err == nil {
// 		if proxy.Exclude == nil {
// 			proxy.Exclude = map[string]bool{}
// 		}
// 		*r = ReleaseConfig(proxy)
// 	}
// 	return err
// }
//
// func (r *ReleaseConfig) Save() error {
//
// 	return nil
// }

type Deploy struct {
	*DeploySettings
	AppDeploys []*AppDeploy
	Filtered   map[string]bool // contains any app deploys which were part of the release but which were filtered out
}

// SharedDeploySettings are copied from the DeploySettings to the AppDeploySettings
type SharedDeploySettings struct {
	Environment        *environment.Environment
	PreviewOnly bool
	UseLocalContent    bool
	Recycle            bool
}

type DeploySettings struct {
	SharedDeploySettings
	ValueSets          []values.ValueSet
	Manifest           *ReleaseManifest
	Apps               map[string]*App
	AppManifests       map[string]*AppManifest
	AppDeploySettings  map[string]AppDeploySettings
	AppOrder           []string
	Clusters           map[string]bool
	Filter             *filter.Chain // If set, only apps which match the filter will be deployed.
	IgnoreDependencies bool
	ForceDeployApps    map[string]bool
	AfterDeploy        func(app *AppDeploy, err error)
	// if set, called after a deploy
}



func (d DeploySettings) WithValueSets(valueSets ...values.ValueSet) DeploySettings {
	d.ValueSets = append(d.ValueSets, valueSets...)
	return d
}

func (d DeploySettings) GetImageTag(appMetadata *AppMetadata) string {
	if d.Manifest != nil {
		if d.Manifest.Name == SlotUnstable {
			return "latest"
		}

		// If app is not pinned, use the version from this release.
		if appMetadata.PinnedReleaseVersion == nil {
			return fmt.Sprintf("%s-%s", appMetadata.Version, d.Manifest.Version.String())
		}
	}

	if appMetadata.PinnedReleaseVersion == nil {
		return appMetadata.Version.String()
	}

	return fmt.Sprintf("%s-%s", appMetadata.Version, appMetadata.PinnedReleaseVersion)
}

type AppDeploySettings struct {
	SharedDeploySettings
	ValueSets         []values.ValueSet
	PlatformAppConfig *PlatformAppConfig
}

func (d DeploySettings) GetAppDeploySettings(name string) AppDeploySettings {

	var valueSets []values.ValueSet

	// If the release manifest has value sets defined, they should be at a lower priority than the app value sets.
	if d.Manifest != nil {
		if d.Manifest.ValueSets != nil {
			valueSet := d.Manifest.ValueSets.ExtractValueSetByName(d.Environment.Name)
			valueSets = append(valueSets, valueSet)
		}
	}

	// Then add value sets from the deploy
	valueSets = append(valueSets, d.ValueSets...)

	appSettings := d.AppDeploySettings[name]

	// combine deploy and app value sets, with app value sets at a higher priority:
	valueSets = append(valueSets, appSettings.ValueSets...)

	appSettings.ValueSets = valueSets

	appSettings.SharedDeploySettings = d.SharedDeploySettings

	return appSettings
}

func NewDeploy(ctx BosunContext, settings DeploySettings) (*Deploy, error) {
	deploy := &Deploy{
		DeploySettings: &settings,
		Filtered:       map[string]bool{},
	}

	appDeployMap := AppDeployMap{}

	if settings.Manifest != nil {
		appManifests, err := settings.Manifest.GetAppManifests()
		if err != nil {
			return nil, err
		}
		for _, manifest := range appManifests {
			if !settings.Manifest.UpgradedApps[manifest.Name] {
				if !settings.ForceDeployApps[manifest.Name] {
					ctx.Log().Debugf("Skipping %q because it is not default nor forced.", manifest.Name)
					deploy.Filtered[manifest.Name] = true
					continue
				}
			}

			appDeploy, newDeployErr := NewAppDeploy(ctx, settings, manifest)
			if newDeployErr != nil {
				return nil, errors.Wrapf(newDeployErr, "create app deploy from manifest for %q", manifest.Name)
			}
			appDeployMap[appDeploy.Name] = appDeploy
		}
	} else if len(settings.Apps) > 0 {

		for _, app := range settings.Apps {
			appManifest, err := app.GetManifest(ctx)
			if err != nil {
				return nil, errors.Wrapf(err, "create app manifest for %q", app.Name)
			}
			appDeploy, err := NewAppDeploy(ctx, settings, appManifest)
			if err != nil {
				return nil, errors.Wrapf(err, "create app deploy from manifest for %q", appManifest.Name)
			}
			appDeployMap[appDeploy.Name] = appDeploy
		}
	} else if len(settings.AppManifests) > 0 {
		for _, appManifest := range settings.AppManifests {
			appDeploy, err := NewAppDeploy(ctx, settings, appManifest)
			if err != nil {
				return nil, errors.Wrapf(err, "create app deploy from manifest for %q", appManifest.Name)
			}
			appDeployMap[appDeploy.Name] = appDeploy
		}
	} else {
		return nil, errors.New("either settings.Manifest, settings.Apps, or settings.AppManifests must be populated")
	}

	if settings.Filter != nil {
		unfilteredAppDeployMap := appDeployMap
		filtered, err := settings.Filter.FromErr(unfilteredAppDeployMap)
		if err != nil {
			return nil, errors.Wrap(err, "all apps were filtered out")
		}
		appDeployMap = filtered.(AppDeployMap)
		for name := range unfilteredAppDeployMap {
			if _, ok := appDeployMap[name]; !ok {
				ctx.Log().Warnf("App %q was filtered out of the release.", name)
				deploy.Filtered[name] = true
			}
		}
	}

	if len(deploy.AppOrder) == 0 {
		var requestedAppNames []string
		dependencies := map[string][]string{}
		for _, app := range appDeployMap {
			requestedAppNames = append(requestedAppNames, app.Name)
			for _, dep := range app.AppConfig.DependsOn {
				dependencies[app.Name] = append(dependencies[app.Name], dep.Name)
			}
		}

		topology, err := GetDependenciesInTopologicalOrder(dependencies, requestedAppNames...)

		if err != nil {
			return nil, errors.Errorf("repos could not be sorted in dependency order: %s", err)
		}

		for _, dep := range topology {
			app, ok := appDeployMap[dep]
			if !ok {
				if deploy.IgnoreDependencies {
					continue
				}
				if filtered := deploy.Filtered[dep]; filtered {
					continue
				}

				return nil, errors.Errorf("an app specifies a dependency that could not be found: %q (filtered: %#v)", dep, deploy.Filtered)
			}

			if app.DesiredState.Status == workspace.StatusUnchanged {
				ctx.WithAppDeploy(app).Log().Infof("Skipping deploy because desired state was %q.", workspace.StatusUnchanged)
				continue
			}

			deploy.AppOrder = append(deploy.AppOrder, app.Name)
		}
	}

	env := ctx.Environment()

	clustersRoles := map[string][]string{}
	for _, cluster := range env.Clusters {
		clustersRoles[cluster.Name] = cluster.Roles.Strings()
	}

	for _, appName := range deploy.AppOrder {

		appCtx := ctx.WithMatchMapArgs(filter.MatchMapArgs{core.KeyAppName: appName}).(BosunContext)

		app := appDeployMap[appName]

		log := appCtx.WithLogField("app", appName).Log()

		clusterRoles := core.ClusterRoles{core.ClusterRoleDefault}
		namespaceRoles := core.NamespaceRoles{core.NamespaceRoleDefault}
		if app.AppDeploySettings.PlatformAppConfig != nil {
			clusterRoles = app.AppDeploySettings.PlatformAppConfig.ClusterRoles
			namespaceRoles = app.AppDeploySettings.PlatformAppConfig.NamespaceRoles
		}

		deployedToClusterForRole := map[string]core.ClusterRole{}

		for _, clusterRole := range clusterRoles {
			clusters, err := env.GetClustersForRole(clusterRole)
			if err != nil {
				return nil, errors.Wrapf(err, "find cluster to deploy %q with role %q", app.Name, clusterRole)
			}
			for _, cluster := range clusters {



				if len(settings.Clusters) > 0 && !settings.Clusters[cluster.Name] {
					log.Infof("Skipping deploy to cluster %s because it was excluded.")
					continue
				}

				log = log.WithField("cluster", cluster.Name)

				if deployedForClusterRole, ok := deployedToClusterForRole[cluster.Name]; ok {
					log.Infof("Already prepared deploy to cluster %q for role %q, skipping additional deploys.", cluster.Name, deployedForClusterRole)
					continue
				}

				appCtx = appCtx.WithMatchMapArgs(filter.MatchMapArgs{
					core.KeyCluster:     cluster.Name,
					core.KeyClusterRole: string(clusterRole),
				}).(BosunContext)

				// store deployment to cluster to prevent re-deploy if one cluster has two roles
				deployedToClusterForRole[cluster.Name] = clusterRole

				app = app.Clone()

				app.Cluster = cluster.Name

				// add the namespace mappings for the cluster to the values which will be resolved
				app = app.WithValueSet(values.ValueSet{
					Static: values.Values{
						"bosun": values.Values{
							"namespaces": cluster.Namespaces.ToStringMap(),
						},
					},
				})

				deployedToNamespaceForRole := map[string]core.NamespaceRole{}
				for _, namespaceRole := range namespaceRoles {

					var namespace kube.NamespaceConfig
					namespace, err = cluster.GetNamespace(namespaceRole)
					if err != nil {
						return nil, errors.Wrapf(err, "mapping namespace for %q", app.Name)
					}

					log = log.WithField("namespace", namespace.Name)

					if deployedForNamespaceRole, ok := deployedToNamespaceForRole[namespace.Name]; ok {
						log.Infof("Already prepared deploy to namespace %q for role %q, skipping additional deploys.", namespace.Name, deployedForNamespaceRole)
						continue
					}

					log.Infof("Configuring app deploy...")

					deployedToNamespaceForRole[namespace.Name] = namespaceRole

					app = app.Clone()

					matchArgs := filter.MatchMapArgs{
						core.KeyEnvironment:     env.Name,
						core.KeyEnvironmentRole: env.Role.String(),
						core.KeyNamespace:       namespace.Name,
						core.KeyNamespaceRole:   string(namespaceRole),
						core.KeyCluster:         cluster.Name,
						core.KeyClusterRole:     string(clusterRole),
						core.KeyClusterProvider: cluster.Provider,
					}

					app.MatchArgs = matchArgs

					appCtx = appCtx.WithMatchMapArgs(matchArgs).(BosunContext)

					var releaseVersion string
					if app.AppManifest.PinnedReleaseVersion != nil {
						releaseVersion = app.AppManifest.PinnedReleaseVersion.String()
					} else if len(app.AppConfig.ReleaseHistory) > 0{
						releaseVersion = app.AppConfig.ReleaseHistory[0].ReleaseVersion
					} else {
						releaseVersion = "0.0.0"
					}

					app.Namespace = namespace.Name
					app = app.WithValueSet(values.ValueSet{
						Static: values.Values{
							"bosun": TemplateBosunValues{
								ReleaseVersion:  releaseVersion,
								AppName:         app.Name,
								AppVersion:      app.AppManifest.Version.String(),
								Environment:     env.Name,
								EnvironmentRole: env.Role.String(),
								Namespace:       namespace.Name,
								NamespaceRole:   string(namespaceRole),
								NamespaceRoles:  namespaceRoles.Strings(),
								Cluster:         cluster.Name,
								ClusterRole:     string(clusterRole),
								ClusterRoles:    cluster.Roles.Strings(),
								ClusterProvider: cluster.Provider,
								ClustersRoles:   clustersRoles,

							}.ToValues(),
						},
					})

					if app.AppDeploySettings.PlatformAppConfig != nil && app.AppDeploySettings.PlatformAppConfig.ValueOverrides != nil {
						appPlatformOverrides := app.AppDeploySettings.PlatformAppConfig.ValueOverrides.ExtractValueSet(values.ExtractValueSetArgs{
							ExactMatch: appCtx.GetMatchMapArgs(),
						})

						app = app.WithValueSet(appPlatformOverrides)
					}

					if env.ValueOverrides != nil {
						envOverrides := env.ValueOverrides.ExtractValueSet(values.ExtractValueSetArgs{
							ExactMatch: appCtx.GetMatchMapArgs(),
						})
						app = app.WithValueSet(envOverrides)
					}

					deploy.AppDeploys = append(deploy.AppDeploys, app)
				}
			}
		}

	}

	return deploy, nil

}

type AppDeployMap map[string]*AppDeploy

func (a AppDeployMap) GetAppsSortedByName() AppReleasesSortedByName {
	var out AppReleasesSortedByName
	for _, app := range a {
		out = append(out, app)
	}

	sort.Sort(out)
	return out
}


//
// func (r *Deploy) IncludeDependencies(ctx BosunContext) error {
// 	ctx = ctx.WithRelease(r)
// 	deps := ctx.Bosun.GetAppDependencyMap()
// 	var appNames []string
// 	for _, app := range r.AppReleaseConfigs {
// 		appNames = append(appNames, app.Name)
// 	}
//
// 	// this is inefficient but it gets us all the dependencies
// 	topology, err := GetDependenciesInTopologicalOrder(deps, appNames...)
//
// 	if err != nil {
// 		return errors.Errorf("repos could not be sorted in dependency order: %s", err)
// 	}
//
// 	for _, dep := range topology {
// 		if r.AppReleaseConfigs[dep] == nil {
// 			if _, ok := r.Exclude[dep]; ok {
// 				ctx.Log().Warnf("Dependency %q is not being added because it is in the exclude list. "+
// 					"Add it using the `add` command if want to override this exclusion.", dep)
// 				continue
// 			}
// 			app, err := ctx.Bosun.GetApp(dep)
// 			if err != nil {
// 				return errors.Errorf("an app or dependency %q could not be found: %s", dep, err)
// 			} else {
// 				err = r.MakeAppAvailable(ctx, app)
// 				if err != nil {
// 					return errors.Errorf("could not include app %q: %s", app.Name, err)
// 				}
// 			}
// 		}
// 	}
// 	return nil
// }

func (d *Deploy) Deploy(ctx BosunContext) error {

	env := ctx.Environment()

	for _, app := range d.AppDeploys {

		appCtx := ctx.WithAppDeploy(app).WithMatchMapArgs(app.MatchArgs).(BosunContext)

		if app.Cluster == "" {
			return errors.Errorf("cluster was empty on app %q", app.Name)
		}

		cluster, err := env.GetClusterByName(app.Cluster)
		if err != nil {
			return err
		}
		err = env.SwitchToCluster(ctx, cluster)
		if err != nil {
			return errors.Wrapf(err, "switch to cluster %q to deploy %q", app.Cluster, app.Name)
		}

		appCtx = appCtx.WithLogField("cluster", app.Cluster).
			WithLogField("namespace", app.Namespace).(BosunContext)

		app.DesiredState.Status = workspace.StatusDeployed
		if app.DesiredState.Routing == "" {
			app.DesiredState.Routing = workspace.RoutingCluster
		}

		app.DesiredState.Force = appCtx.GetParameters().Force

		err = app.Reconcile(appCtx)

		if d.AfterDeploy != nil {
			d.AfterDeploy(app, err)
		}

		if err != nil {
			return err
		}
		if d.Recycle {
			err = app.Recycle(ctx)
			if err != nil {
				return err
			}
		}

	}

	return nil
}

//
// func (r *Deploy) MakeAppAvailable(ctx BosunContext, app *App) error {
//
// 	var err error
// 	var config *AppReleaseConfig
// 	if r.AppReleaseConfigs == nil {
// 		r.AppReleaseConfigs = map[string]*AppReleaseConfig{}
// 	}
//
// 	ctx = ctx.WithRelease(r)
//
// 	config, err = app.GetAppReleaseConfig(ctx)
// 	if err != nil {
// 		return errors.Wrap(err, "make app release")
// 	}
// 	r.AppReleaseConfigs[app.Name] = config
//
// 	r.Apps[app.Name], err = NewAppRelease(ctx, config)
//
// 	return nil
// }
//
// func (r *Deploy) AddBundleFile(app string, path string, content []byte) string {
// 	key := fmt.Sprintf("%s|%s", app, path)
// 	shortPath := safeFileNameRE.ReplaceAllString(strings.TrimLeft(path, "./\\"), "_")
// 	bf := &BundleFile{
// 		App:     app,
// 		Dir:    shortPath,
// 		Content: content,
// 	}
// 	if r.BundleFiles == nil {
// 		r.BundleFiles = map[string]*BundleFile{}
// 	}
// 	r.BundleFiles[key] = bf
// 	return filepath.Join("./", r.Name, app, shortPath)
// }
//
// // GetBundleFileContent returns the content and path to a bundle file, or an error if it fails.
// func (r *Deploy) GetBundleFileContent(app, path string) ([]byte, string, error) {
// 	key := fmt.Sprintf("%s|%s", app, path)
// 	bf, ok := r.BundleFiles[key]
// 	if !ok {
// 		return nil, "", errors.Errorf("no bundle for app %q and path %q", app, path)
// 	}
//
// 	bundleFilePath := filepath.Join(filepath.Dir(r.FromPath), r.Name, bf.App, bf.Dir)
// 	content, err := ioutil.ReadFile(bundleFilePath)
// 	return content, bundleFilePath, err
// }
//
// func (r *ReleaseConfig) SaveBundle() error {
// 	bundleDir := filepath.Join(filepath.Dir(r.FromPath), r.Name)
//
// 	err := os.MkdirAll(bundleDir, 0770)
// 	if err != nil {
// 		return err
// 	}
//
// 	for _, bf := range r.BundleFiles {
// 		if bf.Content == nil {
// 			continue
// 		}
//
// 		appDir := filepath.Join(bundleDir, bf.App)
// 		err := os.MkdirAll(bundleDir, 0770)
// 		if err != nil {
// 			return err
// 		}
//
// 		bundleFilepath := filepath.Join(appDir, bf.Dir)
// 		err = ioutil.WriteFile(bundleFilepath, bf.Content, 0770)
// 		if err != nil {
// 			return errors.Wrapf(err, "writing bundle file for app %q, path %q", bf.App, bf.Dir)
// 		}
// 	}
//
// 	return nil
// }

var safeFileNameRE = regexp.MustCompile(`([^A-z0-9_.]+)`)
