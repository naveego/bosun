package bosun

import (
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"path/filepath"
)

type DeploymentPlanExecutor struct {
	Platform *Platform
	Bosun    *Bosun
}

type ExecuteDeploymentPlanRequest struct {
	Path    string
	Plan    *DeploymentPlan
	Include []string
}

func NewDeploymentPlanExecutor(bosun *Bosun, platform *Platform) DeploymentPlanExecutor {
	return DeploymentPlanExecutor{
		Bosun:    bosun,
		Platform: platform,
	}
}

func (d DeploymentPlanExecutor) Execute(req ExecuteDeploymentPlanRequest) error {

	var err error
	deploymentPlan := req.Plan
	if deploymentPlan == nil {
		if req.Path == "" {
			req.Path = filepath.Join(filepath.Dir(d.Platform.FromPath), "deployments/default/plan.yaml")
		}

		deploymentPlan, err = LoadDeploymentPlanFromFile(req.Path)
		if err != nil {
			return err
		}
	}

	deploySettings := DeploySettings{
		AppManifests:       map[string]*AppManifest{},
		Environment:        d.Bosun.GetCurrentEnvironment(),
		ValueSets:          []values.ValueSet{deploymentPlan.ValueOverrides},
		IgnoreDependencies: deploymentPlan.IgnoreDependencies,
	}

	for _, appPlan := range deploymentPlan.Apps {

		appManifest := appPlan.Manifest
		appManifest.AppConfig.IsFromManifest = true

		for _, platformAppConfig := range d.Platform.Apps {
			if platformAppConfig.Name == appPlan.Name {
				appManifest.PlatformAppConfig = platformAppConfig
			}
		}

		deploySettings.AppManifests[appPlan.Name] = appManifest
	}

	ctx := d.Bosun.NewContext()

	deploy, err := NewDeploy(ctx, deploySettings)
	if err != nil {
		return err
	}

	err = deploy.Deploy(ctx)

	if err != nil {
		return errors.Wrapf(err, "execute deployment plan from %s", req.Path)
	}

	return nil
}
