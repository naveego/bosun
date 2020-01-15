package bosun

import (
	"github.com/naveego/bosun/pkg/util/stringsn"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
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
	ctx := d.Bosun.NewContext()

	deploySettings := DeploySettings{
		AppManifests:       map[string]*AppManifest{},
		AppDeploySettings:  map[string]AppDeploySettings{},
		Environment:        d.Bosun.GetCurrentEnvironment(),
		ValueSets:          []values.ValueSet{deploymentPlan.ValueOverrides},
		IgnoreDependencies: true,
	}

	env := ctx.Environment()

	if deploymentPlan.FromPath != "" {
		if deploymentPlan.EnvironmentDeployProgress == nil {
			deploymentPlan.EnvironmentDeployProgress = map[string][]string{}
		}

		deploySettings.AfterDeploy = func(app *AppDeploy, err error) {
			if err == nil {
				afterLog := ctx.Log().WithFields(logrus.Fields{
					"app":       app.Name,
					"cluster":   app.Cluster,
					"namespace": app.Namespace,
				})
				afterLog.Info("App deployed, saving progress in plan file.")

				deploymentPlan.EnvironmentDeployProgress[env.Name] = stringsn.AppendIfNotPresent(deploymentPlan.EnvironmentDeployProgress[env.Name], app.Name)
				saveErr := deploymentPlan.SavePlanFileOnly()
				if saveErr != nil {
					afterLog.WithError(saveErr).Error("Progress save failed: %s")
				}
			}
		}

	}

	for _, appPlan := range deploymentPlan.Apps {

		appCtx := d.Bosun.NewContext().WithLogField("app", appPlan.Name).(BosunContext)

		if len(deploymentPlan.DeployApps) > 0 && !deploymentPlan.DeployApps[appPlan.Name] {
			appCtx.Log().Infof("Skipping app because it is false in plan.deployApps.")
			continue
		}

		if stringsn.Contains(appPlan.Name, deploymentPlan.EnvironmentDeployProgress[env.Name]) {
			appCtx.Log().Infof("Skipping app because it has already been deployed from this plan to this environment (delete from environmentDeployProgress list to reset).")
			continue
		}

		appManifest := appPlan.Manifest
		appManifest.AppConfig.IsFromManifest = true

		appDeploySettings := AppDeploySettings{}

		for _, platformAppConfig := range d.Platform.GetApps(appCtx) {
			if platformAppConfig.Name == appPlan.Name {
				appDeploySettings.PlatformAppConfig = platformAppConfig
			}
		}

		deploySettings.AppManifests[appPlan.Name] = appManifest
		deploySettings.AppDeploySettings[appPlan.Name] = appDeploySettings
		deploySettings.AppOrder = append(deploySettings.AppOrder, appPlan.Name)
	}

	if len(deploySettings.AppOrder) == 0 {
		ctx.Log().Info("All apps excluded or deployed already.")
		return nil
	}

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
