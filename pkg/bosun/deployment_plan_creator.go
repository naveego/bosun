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
	Path                  string
	ManifestDirPath       string
	ProviderPriority      []string
	Apps                  []string
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

		appPlan := &AppDeploymentPlan{
			Name:           app.Name,
			ValueOverrides: values.ValueSet{},
		}

		if app.IsFromManifest {
			appPlan.Manifest = app.AppManifest
			appPlan.ManifestPath = app.FromPath

			appPlan.Tag = appPlan.Manifest.GetTagBasedOnVersionAndBranch()

		} else {

			appPlan.Manifest, err = app.GetManifest(ctx)
			appPlan.Tag = appPlan.Manifest.GetTagBasedOnVersionAndBranch()
			if err != nil {
				return nil, errors.Wrapf(err, "getting manifest for app %q from provider %q", app.Name, req.ProviderPriority)
			}

			err = appPlan.Manifest.MakePortable()
			if err != nil {
				return nil, errors.Wrapf(err, "making manifest portable for app %q from provider %q", app.Name, req.ProviderPriority)
			}

			manifestPath := filepath.Join(req.ManifestDirPath, appPlan.Name)

			appPlan.ManifestPath, _ = filepath.Rel(req.ManifestDirPath, manifestPath)
		}

		plan.Apps = append(plan.Apps, appPlan)
		plan.DeployApps[appPlan.Name] = true
	}

	return plan, nil
}
