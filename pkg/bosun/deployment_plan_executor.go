package bosun

import (
	"github.com/naveego/bosun/pkg/docker"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/util/stringsn"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/tomb.v2"
	"path/filepath"
	"sync"
)

type DeploymentPlanExecutor struct {
	Platform *Platform
	Bosun    *Bosun
}

type ExecuteDeploymentPlanRequest struct {
	Path           string
	Plan           *DeploymentPlan
	IncludeApps    []string
	ValueSets      values.ValueSets
	Recycle        bool
	Validate       bool
	ValidateOnly   bool
	DumpValuesOnly bool
	DiffOnly       bool
	UseSudo        bool
}


type ExecuteDeploymentPlanResponse struct {
	ValidationErrors map[string]error
}

func NewDeploymentPlanExecutor(bosun *Bosun, platform *Platform) DeploymentPlanExecutor {
	return DeploymentPlanExecutor{
		Bosun:    bosun,
		Platform: platform,
	}
}

func (d DeploymentPlanExecutor) Execute(req ExecuteDeploymentPlanRequest) (ExecuteDeploymentPlanResponse, error) {

	response := ExecuteDeploymentPlanResponse{}

	var err error
	if req.Plan == nil {
		if req.Path == "" {
			req.Path = filepath.Join(filepath.Dir(d.Platform.FromPath), "deployments/default/plan.yaml")
		}

		req.Plan, err = LoadDeploymentPlanFromFile(req.Path)
		if err != nil {
			return response, err
		}
	}
	ctx := d.Bosun.NewContext()

	if req.Validate {
		response.ValidationErrors, err = d.validateDeploymentPlan(req)
	}

	if req.ValidateOnly  {
		return response, nil
	}

	if len(response.ValidationErrors) > 0 {
		return response, errors.New("one or more apps are invalid")
	}

	deploymentPlan := req.Plan
	deploySettings := DeploySettings{
		SharedDeploySettings: SharedDeploySettings{
			Environment:    d.Bosun.GetCurrentEnvironment(),
			Recycle:        req.Recycle,
			DumpValuesOnly: req.DumpValuesOnly,
			DiffOnly:       req.DiffOnly,
		},
		AppManifests:       map[string]*AppManifest{},
		AppDeploySettings:  map[string]AppDeploySettings{},
		ValueSets:          append([]values.ValueSet{deploymentPlan.ValueOverrides}, req.ValueSets...),
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

		deployRequested := stringsn.Contains(appPlan.Name, req.IncludeApps)
		deployDenied := len(req.IncludeApps) > 0 && !stringsn.Contains(appPlan.Name, req.IncludeApps)

		if deployDenied {
			appCtx.Log().Infof("Skipping app because it is not included in the requested apps %v.", req.IncludeApps)
			continue
		}


		if !deployRequested {
			if len(deploymentPlan.DeployApps) > 0 && !deploymentPlan.DeployApps[appPlan.Name] {
				appCtx.Log().Infof("Skipping app because it is false in plan.deployApps.")
				continue
			}

			if stringsn.Contains(appPlan.Name, deploymentPlan.EnvironmentDeployProgress[env.Name]) {
				appCtx.Log().Infof("Skipping app because it has already been deployed from this plan to this environment (delete from environmentDeployProgress list to reset).")
				continue
			}

			if len(env.Apps) > 0 {
				if _, ok := env.Apps[appPlan.Name]; !ok {
					appCtx.Log().Infof("Skipping app because it is not included in the apps list for the environment (request it explicitly to force deployment) (environment apps: %v).", util.SortedKeys(env.Apps))
					continue
				}
			} else if stringsn.Contains(appPlan.Name, env.AppBlacklist) {
				appCtx.Log().Infof("Skipping app because it is in the blacklist for the environment (request it explicitly to force deployment).")
				continue
			}
		}

		appManifest := appPlan.Manifest
		appManifest.AppConfig.IsFromManifest = true
		appManifest.PinnedReleaseVersion = deploymentPlan.ReleaseVersion

		appDeploySettings := AppDeploySettings{
		}
		if appPlan.Tag != "" {
			appDeploySettings.ValueSets = []values.ValueSet{
				values.ValueSet{
					Source:"app plan",
					Static: values.Values{
						"tag": appPlan.Tag,
					},
				},
			}
		}

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
		return response, nil
	}

	deploy, err := NewDeploy(ctx, deploySettings)
	if err != nil {
		return response, err
	}

	err = deploy.Deploy(ctx)

	if err != nil {
		return response, errors.Wrapf(err, "execute deployment plan from %q", req.Path)
	}

	return response, nil
}


func (d DeploymentPlanExecutor) validateDeploymentPlan(req ExecuteDeploymentPlanRequest) (map[string]error, error) {

	plan := req.Plan

	ctx := d.Bosun.NewContext()

	apps := plan.Apps

	response :=  map[string]error{}

	t := new(tomb.Tomb)

	mu := new(sync.Mutex)

	for appIndex := range apps {

		app := apps[appIndex]
		included := len(req.IncludeApps) == 0
		for _,  includedName := range req.IncludeApps {
			if includedName == app.Name {
				included = true
			}
		}
		if !included {
			continue
		}

		imageConfigs := app.Manifest.AppConfig.GetImages()
		appLog := ctx.Log().WithField("app", app.Name)

		appLog.Infof("Verifying images...")
		for i := range imageConfigs {
			imageConfig := imageConfigs[i]
			t.Go(func() error {

				imageName := imageConfig.GetFullNameWithTag(app.Tag)
				imageLog := appLog.WithField("image", imageName)

				imageLog.Infof("Verifying image...")

				err := docker.CheckImageExists(imageName, req.UseSudo)
				mu.Lock()
				if err != nil {
					imageLog.WithError(err).Warnf("Image invalid")
					response[app.Name] = err
				} else {
					imageLog.Info("Image OK")
				}
				mu.Unlock()
				return nil
			})
		}
	}

	_ = t.Wait()

	return response, nil
}

