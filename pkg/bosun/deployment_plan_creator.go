package bosun

import (
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"path/filepath"
)

type DeploymentPlanCreator struct {
	Platform *Platform
	Bosun    *Bosun
}

type CreateDeploymentPlanRequest struct {
	Path             string
	ManifestDirPath  string
	ProviderPriority []string
	// Basic way to request apps with default behaviors
	Apps []string
	// Advanced parameter to customize how apps get resolved
	AppOptions            map[string]AppProviderRequest
	IgnoreDependencies    bool
	AutomaticDependencies bool
	ReleaseVersion        *semver.Version
	BasedOnHash           string
}

func NewDeploymentPlanCreator(bosun *Bosun, platform *Platform) DeploymentPlanCreator {
	return DeploymentPlanCreator{
		Bosun:    bosun,
		Platform: platform,
	}
}

func (d DeploymentPlanCreator) CreateDeploymentPlan(req CreateDeploymentPlanRequest) (*DeploymentPlan, error) {

	ctx := d.Bosun.NewContext()
	p := d.Platform

	if req.Path == "" {
		req.Path = filepath.Join(filepath.Dir(p.FromPath), "deployments/default/plan.yaml")
	}
	dir := filepath.Dir(req.Path)
	if req.ManifestDirPath == "" {
		req.ManifestDirPath = dir
	}

	autoPopulateApps := len(req.Apps) == 0

	if len(req.Apps) == 0 {
		if len(req.AppOptions) == 0 {
			return nil, errors.New("req.Apps or req.AppOptions must be populated")
		}

	}

	// If only app names are populated, populate all the app options with default values.
	if len(req.Apps) > 0  && len(req.AppOptions) == 0 {
		req.AppOptions = map[string]AppProviderRequest{}
		for _, appName := range req.Apps{
			req.AppOptions[appName] = AppProviderRequest{
				Name:             appName,
			}
		}
	}

	for appName := range req.AppOptions {
		if autoPopulateApps {
			req.Apps = append(req.Apps, appName)
		}

		app := req.AppOptions[appName]

		if len(req.ProviderPriority) > 0 && len(app.ProviderPriority) == 0 {
			app.ProviderPriority = req.ProviderPriority
		}
		req.AppOptions[appName] = app
	}

	plan := &DeploymentPlan{
		DirectoryPath:            dir,
		ProviderPriority:         req.ProviderPriority,
		SkipDependencyValidation: req.IgnoreDependencies,
		DeployApps:               map[string]bool{},
		ReleaseVersion:           req.ReleaseVersion,
		BasedOnHash:              req.BasedOnHash,
	}
	apps := map[string]*App{}
	dependencies := map[string][]string{}
	err := p.buildAppsAndDepsRec(ctx.Bosun, req, req.Apps, apps, dependencies)
	if err != nil {
		return nil, err
	}

	topology, err := GetDependenciesInTopologicalOrder(dependencies, req.Apps...)

	if err != nil {
		return nil, errors.Wrapf(err, "apps could not be sorted in dependency order (apps: %#v)", req.Apps)
	}

	for _, dep := range topology {
		app, ok := apps[dep]
		if !ok {
			if !req.IgnoreDependencies {
				if _, err = p.GetPlatformAppUnfiltered(dep); err != nil {
					return nil, errors.Wrapf(err, "an app specifies a dependency that could not be found: %q (topological order: %v)", dep, topology)
				}
			}
			continue
		}

		appReq := req.AppOptions[app.Name]

		appPlan := &AppDeploymentPlan{
			Name:           app.Name,
			ValueOverrides: values.ValueSet{},
		}

		if app.IsFromManifest {
			appPlan.Manifest = app.AppManifest
			appPlan.ManifestPath = app.FromPath

			appPlan.Tag = appPlan.Manifest.GetTagBasedOnVersionAndBranch()

		} else {

			if appReq.Branch != "" {
				appPlan.Manifest, err = app.GetManifestFromBranch(ctx, appReq.Branch, true)
				if err != nil {
					return nil, errors.Wrapf(err, "getting manifest for app %q from branch %q", app.Name, appReq.Branch)
				}
			} else {
				appPlan.Manifest, err = app.GetManifest(ctx)
				if err != nil {
					return nil, errors.Wrapf(err, "getting manifest for app %q", app.Name)
				}
				err = appPlan.Manifest.MakePortable()
				if err != nil {
					return nil, errors.Wrapf(err, "making manifest portable for app %q from provider %q", app.Name, req.ProviderPriority)
				}
			}

			appPlan.Tag = appPlan.Manifest.GetTagBasedOnVersionAndBranch()

			manifestPath := filepath.Join(req.ManifestDirPath, appPlan.Name)

			appPlan.ManifestPath, _ = filepath.Rel(req.ManifestDirPath, manifestPath)
		}

		plan.Apps = append(plan.Apps, appPlan)
		plan.DeployApps[appPlan.Name] = true
	}

	return plan, nil
}
