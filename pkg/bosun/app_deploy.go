package bosun

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/helm"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
)

type AppReleasesSortedByName []*AppDeploy

func (a AppReleasesSortedByName) Len() int {
	return len(a)
}

func (a AppReleasesSortedByName) Less(i, j int) bool {
	return strings.Compare(a[i].Name, a[j].Name) < 0
}

func (a AppReleasesSortedByName) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

// type AppReleaseConfig struct {
// 	Name             string         `yaml:"name" json:"name"`
// 	Namespace        string         `yaml:"namespace" json:"namespace"`
// 	Repo             string         `yaml:"repo" json:"repo"`
// 	Branch           git.BranchName `yaml:"branch" json:"branch"`
// 	Commit           string         `yaml:"commit" json:"commit"`
// 	Version          semver.Version `yaml:"version" json:"version"`
// 	SyncedAt         time.Time      `yaml:"syncedAt" json:"syncedAt"`
// 	Chart            string         `yaml:"chart" json:"chart"`
// 	ImageNames       []string       `yaml:"images,omitempty" json:"images,omitempty"`
// 	ImageTag         string         `yaml:"imageTag,omitempty" json:"imageTag,omitempty"`
// 	ReportDeployment bool           `yaml:"reportDeployment" json:"reportDeployment"`
// 	DependsOn        []string       `yaml:"dependsOn" json:"dependsOn"`
// 	Actions          []*AppAction   `yaml:"actions" json:"actions"`
// 	// Values copied from app repo.
// 	Values ValueSetMap `yaml:"values" json:"values"`
// 	// Values manually added to this release.
// 	ValueOverrides ValueSetMap `yaml:"valueOverrides" json:"valueOverrides"`
// 	ParentConfig   *ReleaseConfig         `yaml:"-" json:"-"`
// }

type AppDeploy struct {
	FromPath     string
	*AppManifest `yaml:"-" json:"-"`
	// App          *App `yaml:"-" json:"-"`
	Excluded          bool `yaml:"-" json:"-"`
	ActualState       AppState
	DesiredState      AppState
	helmRelease       *HelmRelease
	labels            filter.Labels
	AppDeploySettings AppDeploySettings
}

// Chart gets the path to the chart, or the full name of the chart.
func (a *AppDeploy) Chart(ctx BosunContext) string {

	var chartHandle helm.ChartHandle

	if a.AppConfig.IsFromManifest || a.AppConfig.ChartPath == "" {
		chartHandle = helm.ChartHandle(a.AppConfig.Chart)
		if !chartHandle.HasRepo() {
			p, err := ctx.Bosun.GetCurrentPlatform()
			if err == nil {
				defaultChartRepo := p.DefaultChartRepo
				chartHandle = chartHandle.WithRepo(defaultChartRepo)
			}
		}
		return chartHandle.String()

	}

	return filepath.Join(filepath.Dir(a.AppConfig.FromPath), a.AppConfig.ChartPath)

}

func NewAppDeploy(context BosunContext, settings DeploySettings, manifest *AppManifest) (*AppDeploy, error) {

	appDeploy := &AppDeploy{
		AppManifest:       manifest,
		AppDeploySettings: settings.GetAppDeploySettings(manifest.Name),
	}

	appDeploy.AppDeploySettings.ValueSets = append(appDeploy.AppDeploySettings.ValueSets,
		ValueSet{
			Static: Values{
				"tag": manifest.GetImageTag(),
			},
		})

	return appDeploy, nil
}

func (a *AppDeploy) GetLabels() filter.Labels {
	if a.labels == nil {
		a.labels = filter.LabelsFromMap(map[string]string{
			LabelName:    a.Name,
			LabelVersion: a.Version.String(),
			LabelBranch:  a.Branch,
			LabelCommit:  a.Hashes.Commit,
		})
	}
	return a.labels
}

func (a *AppDeploy) LoadActualState(ctx BosunContext, diff bool) error {
	ctx = ctx.WithAppDeploy(a)

	a.ActualState = AppState{}

	log := ctx.Log

	if !ctx.Bosun.IsClusterAvailable() {
		log.Debug("Cluster not available.")

		a.ActualState.Unavailable = true

		return nil
	}

	log.Debug("Getting actual state...")

	release, err := a.GetHelmRelease(a.Name)

	if err != nil || release == nil {
		if release == nil || strings.Contains(err.Error(), "not found") {
			a.ActualState.Status = StatusNotFound
			a.ActualState.Routing = RoutingNA
			a.ActualState.Version = ""
		} else {
			a.ActualState.Error = err
		}
		return nil
	}

	a.ActualState.Status = release.Status
	a.ActualState.Routing = RoutingCluster

	// check if the app has a service with an ExternalName; if it does, it must have been
	// creating using `app toggle` and is routed to localhost.
	if ctx.Env.IsLocal && a.AppConfig.Minikube != nil {
		for _, routableService := range a.AppConfig.Minikube.RoutableServices {
			svcYaml, err := pkg.NewCommand("kubectl", "get", "svc", "--namespace", a.AppConfig.Namespace, routableService.Name, "-o", "yaml").RunOut()
			if err != nil {
				log.WithError(err).Errorf("Error getting service config %q", routableService.Name)
				continue
			}
			if strings.Contains(svcYaml, "ExternalName") {
				a.ActualState.Routing = RoutingLocalhost
				break
			}
		}
	}

	if diff {
		if a.ActualState.Status == StatusDeployed {
			a.ActualState.Diff, err = a.diff(ctx)
			if err != nil {
				return errors.Wrap(err, "diff")
			}
		}
	}

	return nil
}

type HelmReleaseResult struct {
	Releases []*HelmRelease `yaml:"Releases" json:"Releases"`
}
type HelmRelease struct {
	Name       string `yaml:"Name" json:"Name"`
	Revision   string `yaml:"Revision" json:"Revision"`
	Updated    string `yaml:"Updated" json:"Updated"`
	Status     string `yaml:"Status" json:"Status"`
	Chart      string `yaml:"Chart" json:"Chart"`
	AppVersion string `yaml:"AppVersion" json:"AppVersion"`
	Namespace  string `yaml:"Namespace" json:"Namespace"`
}

func (a *AppDeploy) GetHelmRelease(name string) (*HelmRelease, error) {

	if a.helmRelease == nil {
		releases, err := a.GetHelmList(fmt.Sprintf(`^%s$`, name))
		if err != nil {
			return nil, err
		}

		if len(releases) == 0 {
			return nil, nil
		}

		a.helmRelease = releases[0]
	}

	return a.helmRelease, nil
}

func (a *AppDeploy) GetHelmList(filter ...string) ([]*HelmRelease, error) {

	args := append([]string{"list", "--all", "--output", "yaml"}, filter...)
	data, err := pkg.NewCommand("helm", args...).RunOut()
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	var result HelmReleaseResult

	err = yaml.Unmarshal([]byte(data), &result)

	return result.Releases, err
}

type Plan []PlanStep

type PlanStep struct {
	Name        string
	Description string
	Action      func(ctx BosunContext) error
}

func (a *AppDeploy) PlanReconciliation(ctx BosunContext) (Plan, error) {

	ctx = ctx.WithAppDeploy(a)

	if !ctx.Bosun.IsClusterAvailable() {
		return nil, errors.New("cluster not available")
	}

	var steps []PlanStep

	actual, desired := a.ActualState, a.DesiredState

	log := ctx.Log.WithField("name", a.Name)

	log.WithField("state", desired.String()).Debug("Desired state.")
	log.WithField("state", actual.String()).Debug("Actual state.")

	var (
		needsDelete   bool
		needsInstall  bool
		needsRollback bool
		needsUpgrade  bool
	)

	if desired.Status == StatusNotFound || desired.Status == StatusDeleted {
		needsDelete = actual.Status != StatusDeleted && actual.Status != StatusNotFound
	} else {
		needsDelete = actual.Status == StatusFailed
		needsDelete = needsDelete || actual.Status == StatusPendingUpgrade
	}

	if desired.Status == StatusDeployed {
		switch actual.Status {
		case StatusNotFound:
			needsInstall = true
		case StatusDeleted:
			needsRollback = true
			needsUpgrade = true
		default:
			needsUpgrade = actual.Status != StatusDeployed
			needsUpgrade = needsUpgrade || actual.Routing != desired.Routing
			needsUpgrade = needsUpgrade || actual.Version != desired.Version
			needsUpgrade = needsUpgrade || actual.Diff != ""
			needsUpgrade = needsUpgrade || desired.Force
		}
	}

	if needsDelete {
		steps = append(steps, PlanStep{
			Name:        "Delete",
			Description: "Delete release from kubernetes.",
			Action:      a.Delete,
		})
	}

	if desired.Status == StatusDeployed {
		for i := range a.AppConfig.Actions {
			action := a.AppConfig.Actions[i]
			if strings.Contains(string(action.When), ActionBeforeDeploy) {
				steps = append(steps, PlanStep{
					Name:        action.Name,
					Description: action.Description,
					Action: func(ctx BosunContext) error {
						return action.Execute(ctx)
					},
				})
			}
		}
	}

	if needsInstall {
		steps = append(steps, PlanStep{
			Name:        "Install",
			Description: "Install chart to kubernetes.",
			Action:      a.Install,
		})
	}

	if needsRollback {
		steps = append(steps, PlanStep{
			Name:        "Rollback",
			Description: "Rollback existing release in kubernetes to allow upgrade.",
			Action:      a.Rollback,
		})
	}

	if needsUpgrade {
		steps = append(steps, PlanStep{
			Name:        "Upgrade",
			Description: "Upgrade existing release in kubernetes.",
			Action:      a.Upgrade,
		})
	}

	if desired.Status == StatusDeployed {
		for i := range a.AppConfig.Actions {
			action := a.AppConfig.Actions[i]
			if strings.Contains(string(action.When), ActionAfterDeploy) {
				steps = append(steps, PlanStep{
					Name:        action.Name,
					Description: action.Description,
					Action: func(ctx BosunContext) error {
						return action.Execute(ctx)
					},
				})
			}
		}
	}

	return steps, nil

}

type PersistableValues struct {
	Values   Values
	FilePath string
}

func (r *PersistableValues) PersistValues() (string, error) {
	if r.FilePath == "" {

		// b, err := r.Values.YAML()
		// if err != nil {
		// 	return "", err
		// }
		// r.FilePath = server.GetDefaultServer().AddValueFile(uuid.New().String(), []byte(b))

		tmp, err := ioutil.TempFile(os.TempDir(), "bosun-release-*.yaml")
		if err != nil {
			return "", err
		}
		defer tmp.Close()
		err = r.Values.Encode(tmp)
		if err != nil {
			return "", err
		}
		r.FilePath = tmp.Name()
		return r.FilePath, nil
	}
	return r.FilePath, nil

}

func (r *PersistableValues) Cleanup() {
	err := os.Remove(r.FilePath)
	if err != nil && !os.IsNotExist(err) {
		pkg.Log.WithError(err).WithField("path", r.FilePath).
			Fatal("Failed to clean up persisted values file, which make contain secrets. You must manually delete this file.")
	}
}

func (a *AppDeploy) GetResolvedValues(ctx BosunContext) (*PersistableValues, error) {
	r := &PersistableValues{
		Values: Values{},
	}

	// Make environment values available
	if err := r.Values.AddEnvAsPath(EnvPrefix, EnvAppVersion, a.Version); err != nil {
		return nil, err
	}
	if err := r.Values.AddEnvAsPath(EnvPrefix, EnvAppBranch, a.Branch); err != nil {
		return nil, err
	}
	if err := r.Values.AddEnvAsPath(EnvPrefix, EnvAppCommit, a.Hashes.Commit); err != nil {
		return nil, err
	}

	importedValues := a.AppConfig.Values.ExtractValueSetByNames(ctx.Env.ValueSets...)

	appValues := append([]ValueSet{importedValues}, a.AppDeploySettings.ValueSets...)

	for _, v := range appValues {

		r.Values.Merge(v.Static)

		// Get the values defined using the `dynamic` element:
		for k, v := range v.Dynamic {
			value, err := v.Resolve(ctx)
			if err != nil {
				return nil, errors.Errorf("resolving dynamic values for app %q for key %q: %s", a.Name, k, err)
			}
			err = r.Values.SetAtPath(k, value)
			if err != nil {
				return nil, errors.Errorf("merging dynamic values for app %q for key %q: %s", a.Name, k, err)
			}
		}
	}

	// Finally, apply any overrides from parameters passed to this invocation of bosun.
	for k, v := range ctx.GetParams().ValueOverrides {
		err := r.Values.SetAtPath(k, v)
		if err != nil {
			return nil, errors.Errorf("applying overrides with path %q: %s", k, err)
		}

	}

	return r, nil
}

func (a *AppDeploy) Reconcile(ctx BosunContext) error {
	ctx = ctx.WithAppDeploy(a)
	log := ctx.Log

	if a.DesiredState.Status == StatusUnchanged {
		log.Infof("Desired state is %q, nothing to do here.", StatusUnchanged)
		return nil
	}

	values, err := a.GetResolvedValues(ctx)
	if err != nil {
		return errors.Errorf("create values map for app %q: %s", a.Name, err)
	}

	valuesYaml, _ := values.Values.YAML()
	log.Debugf("Created release values for app:\n%s", valuesYaml)

	_, err = values.PersistValues()
	if err != nil {
		return errors.Errorf("persist values for app %q: %s", a.Name, err)
	}
	defer values.Cleanup()

	ctx = ctx.WithPersistableValues(values)

	// clear helm release cache after work is done
	defer func() { a.helmRelease = nil }()

	err = a.LoadActualState(ctx, true)
	if err != nil {
		return errors.Errorf("error checking actual state for %q: %s", a.Name, err)
	}

	params := ctx.GetParams()
	env := ctx.Env

	reportDeploy := !params.DryRun &&
		!params.NoReport &&
		!a.AppDeploySettings.Environment.IsLocal &&
		a.DesiredState.Status == StatusDeployed &&
		!env.IsLocal &&
		a.AppConfig.ReportDeployment

	log.Info("Planning reconciliation...")

	plan, err := a.PlanReconciliation(ctx)

	if err != nil {
		return err
	}

	if len(plan) == 0 {
		log.Info("No actions needed to reconcile state.")
		return nil
	}

	if reportDeploy {

		cleanup, err := a.ReportDeployment(ctx)
		if err != nil {
			return err
		}

		// ensure that the deployment is updated when we return.
		defer func() {
			if r := recover(); r != nil {
				var ok bool
				err, ok = r.(error)
				if ok {
					err = errors.Errorf("%s: panicked with error: %s\n%s", err, debug.Stack())
				} else {
					err = errors.Errorf("%s: panicked: %v\n%s", r, debug.Stack())
				}
			}

			cleanup(err)
		}()
	}

	for _, step := range plan {
		log.WithField("step", step.Name).WithField("description", step.Description).Info("Planned step.")
	}

	log.Info("Planning complete.")

	log.Debug("Executing plan...")

	for _, step := range plan {
		stepCtx := ctx.WithLog(log.WithField("step", step.Name))
		stepCtx.Log.Info("Executing step...")
		err = step.Action(stepCtx)
		if err != nil {
			return err
		}
		stepCtx.Log.Info("Step complete.")
	}

	log.Debug("Plan executed.")

	return nil
}

func (a *AppDeploy) ReportDeployment(ctx BosunContext) (cleanup func(error), err error){

	log := ctx.Log
	env := ctx.Env

	log.Info("Deploy progress will be reported to github.")
	gitToken, err := ctx.Bosun.GetGithubToken()
	if err != nil {
		return nil, err
	}
	client := git.NewGithubClient(gitToken)
	// create the deployment
	deployID, err := git.CreateDeploy(client, a.Repo, a.Branch, env.Name)
	if err != nil {
		return nil, err
	}

	issueSvc , err := ctx.Bosun.GetIssueService()

	org, repoName := git.GetCurrentOrgAndRepo()

	closedIssues, err := issueSvc.GetClosedIssue(org, repoName)
	if err != nil {
		return nil, errors.New("get closed issues")
	}

	for _, closedIssue := range closedIssues {
		issueRef := issues.NewIssueRef(org, repoName,closedIssue.Number)
		parents, err := issueSvc.GetParents(issueRef)
		if err != nil {
			return nil, errors.New("get parents for closed issue")
		}
		if len(parents) <= 0 {
			continue
		}
		parent := parents[0]
		if parent.ProgressState == issues.ColumnWaitingForDeploy {
			parentRef := issues.NewIssueRef(parent.Org, parent.Repo, parent.Number)
			err = issueSvc.SetProgress(parentRef, issues.ColumnWaitingForUAT)
			if err != nil {
				return nil, errors.New("move parent story to Waiting for UAT")
			}
		}
	}


	// ensure that the deployment is updated when we return.
	return func(failure error) {
		if failure != nil {
			_ = git.UpdateDeploy(client, a.Repo, deployID, "failure")
		} else {
			_ = git.UpdateDeploy(client, a.Repo, deployID, "success")
		}
	}, nil
}

func (a *AppDeploy) diff(ctx BosunContext) (string, error) {

	args := omitStrings(a.makeHelmArgs(ctx), "--dry-run", "--debug")

	msg, err := pkg.NewCommand("helm", "diff", "upgrade", a.Name, a.Chart(ctx)).
		WithArgs(args...).
		RunOut()

	if err != nil {
		return "", err
	} else {
		if msg == "" {
			ctx.Log.Debug("Diff detected no changes.")
		} else {
			ctx.Log.Debugf("Diff result:\n%s\n", msg)
		}
	}

	return msg, nil
}

func (a *AppDeploy) Delete(ctx BosunContext) error {
	args := []string{"delete"}
	if a.DesiredState.Status == StatusNotFound {
		args = append(args, "--purge")
	}
	args = append(args, a.Name)

	out, err := pkg.NewCommand("helm", args...).RunOut()
	ctx.Log.Debug(out)
	return err
}

func (a *AppDeploy) Rollback(ctx BosunContext) error {
	args := []string{"rollback"}
	args = append(args, a.Name, a.helmRelease.Revision)
	// args = append(args, a.getHelmNamespaceArgs(ctx)...)
	args = append(args, a.getHelmDryRunArgs(ctx)...)

	out, err := pkg.NewCommand("helm", args...).RunOut()
	ctx.Log.Debug(out)
	return err
}

func (a *AppDeploy) Install(ctx BosunContext) error {
	args := append([]string{"install", "--name", a.Name, a.Chart(ctx)}, a.makeHelmArgs(ctx)...)
	out, err := pkg.NewCommand("helm", args...).RunOut()
	ctx.Log.Debug(out)
	return err
}

func (a *AppDeploy) Upgrade(ctx BosunContext) error {
	args := append([]string{"upgrade", a.Name, a.Chart(ctx)}, a.makeHelmArgs(ctx)...)
	if a.DesiredState.Force {
		args = append(args, "--force")
	}
	out, err := pkg.NewCommand("helm", args...).RunOut()
	ctx.Log.Debug(out)
	return err
}

func (a *AppDeploy) GetStatus() (string, error) {
	release, err := a.GetHelmRelease(a.Name)
	if err != nil {
		return "", err
	}
	if release == nil {
		return "NOTFOUND", nil
	}

	return release.Status, nil
}

func (a *AppDeploy) RouteToLocalhost(ctx BosunContext) error {

	ctx = ctx.WithAppDeploy(a)

	ctx.Log.Info("Configuring app to route traffic to localhost.")

	if a.AppConfig.Minikube == nil || len(a.AppConfig.Minikube.RoutableServices) == 0 {
		return errors.New(`to route to localhost, app must have a minikube entry like this:
  minikube:
    routableServices:
    - name: # the name of the service that your ingress points at
      portName: http # name of the port the ingress points at      
      localhostPort: # port that your service runs on when its running on your localhost
`)
	}

	hostIP := ctx.Bosun.ws.Minikube.HostIP
	if hostIP == "" {
		return errors.New("minikube.hostIP is not set in root config file; it should be the IP of your machine reachable from the minikube VM")
	}

	for _, routableService := range a.AppConfig.Minikube.RoutableServices {
		log := ctx.Log.WithField("routable_service", routableService.Name)

		log.Info("Updating service...")

		realSvcYaml, err := pkg.NewCommand("kubectl", "get", "svc", routableService.Name, "-o", "yaml").RunOut()
		if err != nil {
			return errors.Errorf("getting service config for %q: %s", routableService.Name, err)
		}

		routedSvc, err := ReadValues([]byte(realSvcYaml))
		if err != nil {
			return err
		}

		localhostPort := routableService.LocalhostPort
		if localhostPort == 0 {
			localhostPort = routableService.ExternalPort
		}

		_ = routedSvc.SetAtPath("spec.clusterIP", "")
		_ = routedSvc.SetAtPath("spec.type", "ExternalName")
		_ = routedSvc.SetAtPath("spec.externalName", hostIP)
		_ = routedSvc.SetAtPath("spec.ports", []Values{
			{
				"port":     localhostPort,
				"protocol": "TCP",
				"name":     routableService.PortName,
			},
		})

		routedSvcYaml, _ := routedSvc.YAML()

		err = func() error {
			tmp, err := util.NewTempFile("routed-service", []byte(routedSvcYaml))
			if err != nil {
				return errors.Wrap(err, "create service file")
			}
			defer tmp.CleanUp()

			err = pkg.NewCommand("kubectl", "delete", "service", routableService.Name).RunE()
			if err != nil {
				return errors.Wrap(err, "delete service")
			}
			err = pkg.NewCommand("kubectl", "apply", "-f", tmp.Path).RunE()
			if err != nil {
				return errors.Errorf("error applying service\n%s\n---\n%s", routedSvcYaml, err)
			}
			return nil
		}()
		if err != nil {
			return errors.Wrapf(err, "updating service for %q", a.Name)
		}

		log.Info("Updated service.")
	}

	return nil
}

func (a *AppDeploy) makeHelmArgs(ctx BosunContext) []string {

	var args []string

	if !a.AppDeploySettings.UseLocalContent {
		args = append(args, "--version", a.Version.String())
	}

	args = append(args,
		"--set", fmt.Sprintf("domain=%s", ctx.Env.Domain))

	args = append(args, a.getHelmNamespaceArgs(ctx)...)

	args = append(args, "-f", ctx.Values.FilePath)

	args = append(args, a.getHelmDryRunArgs(ctx)...)

	return args
}

func (a *AppDeploy) getHelmNamespaceArgs(ctx BosunContext) []string {
	namespace := "default"
	if a.AppConfig.Namespace != "" {
		namespace = a.AppConfig.Namespace
	}

	return []string{"--namespace", namespace}
}

func (a *AppDeploy) getHelmDryRunArgs(ctx BosunContext) []string {
	if ctx.IsDryRun() {
		return []string{"--dry-run", "--debug"}
	}
	return []string{}
}

func (a *AppDeploy) Recycle(ctx BosunContext) error {
	ctx = ctx.WithAppDeploy(a)
	ctx.Log.Info("Deleting pods...")
	err := pkg.NewCommand("kubectl", "delete", "--namespace", a.AppConfig.GetNamespace(), "pods", "--selector=release="+a.AppConfig.Name).RunE()
	if err != nil {
		return err
	}
	ctx.Log.Info("Pods deleted, waiting for recreated pods to be ready.")

	for {
		podsReady := true
		out, err := pkg.NewCommand("kubectl", "get", "pods", "--namespace", a.AppConfig.GetNamespace(), "--selector=release="+a.AppConfig.Name,
			"-o", `jsonpath={range .items[*]}{@.metadata.name}:{@.status.conditions[?(@.type=='Ready')].status};{end}`).RunOut()
		if err != nil {
			return err
		}
		pods := strings.Split(out, ";")
		for _, pod := range pods {
			segs := strings.Split(pod, ":")
			if len(segs) != 2 {
				continue
			}
			podName, rawReady := segs[0], segs[1]
			if rawReady == "True" {
				color.Green("%s: Ready\n", podName)
			} else {
				color.Red("%s: Not ready\n", podName)
			}
			podsReady = podsReady && (rawReady == "True")
		}
		if podsReady {
			break
		}

		color.White("...")

		wait := 5 * time.Second
		ctx.Log.Debugf("Waiting %s to check readiness again...", wait)
		select {
		case <-time.After(wait):
		case <-ctx.Ctx().Done():
			return ctx.Ctx().Err()
		}
	}

	ctx.Log.Info("Recycle complete.")

	return nil
}

func getTagFromImage(image string) string {
	segs := strings.Split(image, ":")
	switch len(segs) {
	case 0, 1:
		return "latest"
	default:
		return segs[1]
	}
}
