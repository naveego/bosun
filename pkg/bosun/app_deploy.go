package bosun

import (
	"context"
	"fmt"
	"github.com/fatih/color"
	"github.com/google/go-github/v20/github"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/actions"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/workspace"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
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
	return strings.Compare(a[i].AppManifest.Name, a[j].AppManifest.Name) < 0
}

func (a AppReleasesSortedByName) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

// type AppReleaseConfig struct {
// 	Name             string         `yaml:"name" json:"name"`
// 	Namespace        string         `yaml:"namespace" json:"namespace"`
// 	Repo             string         `yaml:"repo" json:"repo"`
// 	Branch           git.BranchName `yaml:"branch" json:"branch"`
// 	GetCurrentCommit           string         `yaml:"commit" json:"commit"`
// 	Version          semver.Version `yaml:"version" json:"version"`
// 	SyncedAt         time.Time      `yaml:"syncedAt" json:"syncedAt"`
// 	Chart            string         `yaml:"chart" json:"chart"`
// 	ImageNames       []string       `yaml:"images,omitempty" json:"images,omitempty"`
// 	ImageTag         string         `yaml:"imageTag,omitempty" json:"imageTag,omitempty"`
// 	ReportDeployment bool           `yaml:"reportDeployment" json:"reportDeployment"`
// 	DependsOn        []string       `yaml:"dependsOn" json:"dependsOn"`
// 	Actions          []*AppAction   `yaml:"actions" json:"actions"`
// 	// Values copied from app repo.
// 	Values ValueSetCollection `yaml:"values" json:"values"`
// 	// Values manually added to this release.
// 	ValueOverrides ValueSetCollection `yaml:"valueOverrides" json:"valueOverrides"`
// 	ParentConfig   *ReleaseConfig         `yaml:"-" json:"-"`
// }

type AppDeploy struct {
	Name        string       `yaml:"name,omitempty"`
	AppManifest *AppManifest `yaml:"appManifest,omitempty"`
	AppConfig   *AppConfig   `yaml:"appConfig,omitempty"`

	FromPath          string             `yaml:"fromPath,omitempty"`
	Excluded          bool               `yaml:"excluded,omitempty"`
	ActualState       workspace.AppState `yaml:"actualState,omitempty"`
	DesiredState      workspace.AppState `yaml:"desiredState,omitempty"`
	Cluster           string             `yaml:"cluster,omitempty"`
	Namespace         string             `yaml:"namespace,omitempty"`
	AppDeploySettings AppDeploySettings  `yaml:"appDeploySettings,omitempty"`

	MatchArgs filter.ExactMatchArgs `yaml:"matchArgs,omitempty"`

	helmRelease *HelmRelease  `yaml:"-"`
	labels      filter.Labels `yaml:"-"`
}

func (a *AppDeploy) Clone() *AppDeploy {

	out := AppDeploy{
		Name:              a.Name,
		AppManifest:       a.AppManifest,
		AppConfig:         a.AppConfig,
		FromPath:          a.FromPath,
		ActualState:       a.ActualState,
		DesiredState:      a.DesiredState,
		Cluster:           a.Cluster,
		Namespace:         a.Namespace,
		AppDeploySettings: a.AppDeploySettings,
		helmRelease:       a.helmRelease,
		labels:            a.labels,
	}
	return &out
}

// Chart gets the path to the chart, or the full name of the chart.
func (a *AppDeploy) Chart(ctx BosunContext) string {

	// var chartHandle helm.ChartHandle
	//
	// if a.AppManifest.AppConfig.IsFromManifest || a.AppManifest.AppConfig.ChartPath == "" {
	// 	chartHandle = helm.ChartHandle(a.AppManifest.AppConfig.Chart)
	// 	if !chartHandle.HasRepo() {
	// 		p, err := ctx.Bosun.GetCurrentPlatform()
	// 		if err == nil {
	// 			defaultChartRepo := p.DefaultChartRepo
	// 			chartHandle = chartHandle.WithRepo(defaultChartRepo)
	// 		}
	// 	}
	// 	return chartHandle.String()
	//
	// }

	return filepath.Join(filepath.Dir(a.AppManifest.AppConfig.FromPath), a.AppManifest.AppConfig.ChartPath)

}

func NewAppDeploy(ctx BosunContext, settings DeploySettings, manifest *AppManifest) (*AppDeploy, error) {

	appDeploySettings := settings.GetAppDeploySettings(manifest.Name)

	// put the tag as the lowest priority of the augmenting value sets, so
	// that it can be overwritten by user-provided value sets.

	bosunAppTemplateValues := values.Values{
		"version":        manifest.Version.String(),
		"releaseVersion": "Transient",
		"tag":            settings.GetImageTag(manifest.AppMetadata),
		"environment":    ctx.Environment().Name,
	}
	if manifest.PinnedReleaseVersion != nil {
		bosunAppTemplateValues["releaseVersion"] = manifest.PinnedReleaseVersion.String()
	}

	appDeploySettings.ValueSets = append([]values.ValueSet{{
		Static: values.Values{
			"tag":   settings.GetImageTag(manifest.AppMetadata),
			"bosun": bosunAppTemplateValues,
		},
	}}, appDeploySettings.ValueSets...)
	appDeploy := &AppDeploy{
		Name:              manifest.Name,
		FromPath:          manifest.AppConfig.FromPath,
		AppManifest:       manifest,
		AppConfig:         manifest.AppConfig,
		AppDeploySettings: appDeploySettings,
	}

	return appDeploy, nil
}

func (a *AppDeploy) GetLabels() filter.Labels {
	if a.labels == nil {
		a.labels = filter.LabelsFromMap(map[string]string{
			core.LabelName:    a.AppManifest.Name,
			core.LabelVersion: a.AppManifest.Version.String(),
			core.LabelBranch:  a.AppManifest.Branch,
			core.LabelCommit:  a.AppManifest.Hashes.Commit,
		})
	}
	return a.labels
}

func (a *AppDeploy) WithValueSet(v values.ValueSet) *AppDeploy {

	shallowCopy := *a

	shallowCopy.AppDeploySettings.ValueSets = append(a.AppDeploySettings.ValueSets, v)

	return &shallowCopy
}

func (a *AppDeploy) LoadActualState(ctx BosunContext, diff bool) error {
	ctx = ctx.WithAppDeploy(a)

	a.ActualState = workspace.AppState{}

	log := ctx.Log()

	log.Debug("Getting actual state...")

	release, err := a.GetHelmRelease(a.AppManifest.Name)

	if err != nil && !strings.Contains(err.Error(), "not found") {
		return err
	}

	if release == nil {
		a.ActualState.Status = workspace.StatusNotFound
		a.ActualState.Routing = workspace.RoutingNA
		a.ActualState.Version = ""
		return nil
	}

	a.ActualState.Status = strings.ToUpper(release.Status)

	if !workspace.KnownHelmChartStatuses[a.ActualState.Status] {
		return errors.Errorf("current status %q is not understood", a.ActualState.Status)
	}
	a.ActualState.Routing = workspace.RoutingCluster

	// check if the app has a service with an ExternalName; if it does, it must have been
	// creating using `app toggle` and is routed to localhost.
	if ctx.Environment().IsLocal && a.AppManifest.AppConfig.Minikube != nil {
		for _, routableService := range a.AppManifest.AppConfig.Minikube.RoutableServices {
			svcYaml, err := pkg.NewShellExe("kubectl", "get", "svc", "--namespace", a.Namespace, routableService.Name, "-o", "yaml").RunOut()
			if err != nil {
				log.WithError(err).Errorf("Error getting service config %q", routableService.Name)
				continue
			}
			if strings.Contains(svcYaml, "ExternalName") {
				a.ActualState.Routing = workspace.RoutingLocalhost
				break
			}
		}
	}

	if diff {
		if a.ActualState.Status == workspace.StatusDeployed {
			a.ActualState.Diff, err = a.diff(ctx)
			if err != nil {
				return errors.Wrap(err, "diff")
			}
		}
	}

	return nil
}

type HelmReleaseResult []*HelmRelease
type HelmRelease struct {
	Name       string `yaml:"name" json:"Name"`
	Revision   string `yaml:"revision" json:"Revision"`
	Updated    string `yaml:"updated" json:"Updated"`
	Status     string `yaml:"status" json:"Status"`
	Chart      string `yaml:"chart" json:"Chart"`
	AppVersion string `yaml:"app_version" json:"AppVersion"`
	Namespace  string `yaml:"namespace" json:"Namespace"`
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

func (a *AppDeploy) GetHelmList(filter string) ([]*HelmRelease, error) {

	if filter == "" {
		filter = ".*"
	}
	args := []string{"list", "--all", "--all-namespaces", "--output", "yaml", "--filter", filter}
	data, err := pkg.NewShellExe("helm", args...).RunOut()
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	var result HelmReleaseResult

	err = yaml.Unmarshal([]byte(data), &result)

	return result, errors.Wrapf(err, "helm list result:\n%s", data)
}

type Plan []PlanStep

type PlanStep struct {
	Name        string
	Description string
	Action      func(ctx BosunContext) error
}

func (a *AppDeploy) PlanReconciliation(ctx BosunContext) (Plan, error) {

	ctx = ctx.WithAppDeploy(a)

	var steps []PlanStep

	actual, desired := a.ActualState, a.DesiredState

	log := ctx.Log().WithField("name", a.AppManifest.Name)

	log.WithField("state", desired.String()).Debug("Desired state.")
	log.WithField("state", actual.String()).Debug("Actual state.")

	var (
		needsDelete   bool
		needsInstall  bool
		needsRollback bool
		needsUpgrade  bool
	)

	if desired.Status == workspace.StatusNotFound || desired.Status == workspace.StatusDeleted {
		needsDelete = actual.Status != workspace.StatusDeleted && actual.Status != workspace.StatusNotFound
	} else {
		needsDelete = actual.Status == workspace.StatusFailed
		needsDelete = needsDelete || actual.Status == workspace.StatusPendingUpgrade
	}

	if desired.Status == workspace.StatusDeployed {
		switch actual.Status {
		case workspace.StatusNotFound:
			needsInstall = true
		case workspace.StatusDeleted:
			needsRollback = true
			needsUpgrade = true
		default:
			needsUpgrade = actual.Status != workspace.StatusDeployed
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

	if desired.Status == workspace.StatusDeployed {
		for i := range a.AppManifest.AppConfig.Actions {
			action := a.AppManifest.AppConfig.Actions[i]
			if action.When.Contains(actions.ActionBeforeDeploy) && action.WhereFilter.Matches(ctx.GetExactMatchArgs()) {
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

	if desired.Status == workspace.StatusDeployed {
		for i := range a.AppManifest.AppConfig.Actions {
			action := a.AppManifest.AppConfig.Actions[i]
			if action.When.Contains(actions.ActionAfterDeploy) && action.WhereFilter.Matches(ctx.GetExactMatchArgs()) {
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

// GetResolvedValues handles loading and merging all values needed for the
// deployment of the app, including reading the default helm chart values,
// loading any values files, and resolving any dynamic values.
func (a *AppDeploy) GetResolvedValues(ctx BosunContext) (*values.PersistableValues, error) {

	matchArgs := ctx.GetExactMatchArgs()
	bosunValues := values.Values{}
	for k, v := range matchArgs {
		bosunValues[k] = v
	}

	resolvedValues := values.NewValueSet().WithValues(
		values.ValueSet{
			Source: "bosun context",
			Static: bosunValues,
		})

	// Make environment values available
	if err := resolvedValues.Static.AddEnvAsPath(core.EnvPrefix, core.EnvAppVersion, a.AppManifest.Version); err != nil {
		return nil, err
	}
	if err := resolvedValues.Static.AddEnvAsPath(core.EnvPrefix, core.EnvAppBranch, a.AppManifest.Branch); err != nil {
		return nil, err
	}
	if err := resolvedValues.Static.AddEnvAsPath(core.EnvPrefix, core.EnvAppCommit, a.AppManifest.Hashes.Commit); err != nil {
		return nil, err
	}

	if chartValues, err := a.AppManifest.AppConfig.LoadChartValues(); err != nil {
		return nil, errors.Wrapf(err, "load chart values")
	} else {
		resolvedValues = resolvedValues.WithValues(chartValues)
	}

	if platformValues, err := ResolveValues(ctx.GetPlatform(), ctx); err != nil {
		return nil, errors.Wrapf(err, "resolve platform values")
	} else {
		resolvedValues = resolvedValues.WithValues(platformValues.WithSource("platform overrides"))
	}

	env := ctx.Environment()
	if environmentValues, err := ResolveValues(env, ctx); err != nil {
		return nil, errors.Wrapf(err, "resolve environment values")
	} else {
		resolvedValues = resolvedValues.WithValues(environmentValues.WithDefaultSource(fmt.Sprintf("%s environment", env.Name)))
	}

	cluster := env.Cluster
	if clusterValues, err := ResolveValues(cluster, ctx); err != nil {
		return nil, errors.Wrapf(err, "resolve cluster values")
	} else {
		resolvedValues = resolvedValues.WithValues(clusterValues.WithDefaultSource(fmt.Sprintf("%s cluster", cluster.Name)))
	}

	if appConfigValues, err := ResolveValues(a.AppConfig, ctx); err != nil {
		return nil, errors.Wrapf(err, "load value set from app config")
	} else {
		resolvedValues = resolvedValues.WithValues(appConfigValues.WithDefaultSource("app config"))
	}

	for _, v := range a.AppDeploySettings.ValueSets {
		resolvedValues = resolvedValues.WithValues(v.WithDefaultSource("app deploy settings"))
	}

	// ApplyToValues any overrides from parameters passed to this invocation of bosun.
	for k, v := range ctx.GetParameters().ValueOverrides {
		var err error
		resolvedValues, err = resolvedValues.WithValueSetAtPath(k, v, "command line parameter")
		if err != nil {
			return nil, errors.Errorf("applying overrides with path %q: %s", k, err)
		}
	}

	// Finally apply any value mappings
	err := a.AppManifest.AppConfig.ValueMappings.ApplyToValues(resolvedValues.Static)
	if err != nil {
		return nil, err
	}
	r := &values.PersistableValues{
		Values: resolvedValues.Static,
	}

	resolvedDump, _ := yaml.MarshalString(resolvedValues)

	fmt.Println("Resolved values:")
	fmt.Println(resolvedDump)
	fmt.Println()

	return r, nil
}

func (a *AppDeploy) Reconcile(ctx BosunContext) error {
	ctx = ctx.WithAppDeploy(a)
	log := ctx.Log()

	if a.DesiredState.Status == workspace.StatusUnchanged {
		log.Infof("Desired state is %q, nothing to do here.", workspace.StatusUnchanged)
		return nil
	}

	resolvedValues, err := a.GetResolvedValues(ctx)
	if err != nil {
		return errors.Errorf("create values map for app %q: %s", a.AppManifest.Name, err)
	}

	valuesYaml, _ := yaml.MarshalString(resolvedValues)
	log.Debugf("Created release values for app:\n%s", valuesYaml)

	_, err = resolvedValues.PersistValues()
	if err != nil {
		return errors.Errorf("persist values for app %q: %s", a.AppManifest.Name, err)
	}
	defer resolvedValues.Cleanup()

	ctx = ctx.WithPersistableValues(resolvedValues).(BosunContext)

	// clear helm release cache after work is done
	defer func() { a.helmRelease = nil }()

	err = a.LoadActualState(ctx, true)
	if err != nil {
		return errors.Errorf("error checking actual state for %q: %s", a.AppManifest.Name, err)
	}

	params := ctx.GetParameters()
	env := ctx.Environment()

	reportDeploy := !params.DryRun &&
		!params.NoReport &&
		!a.AppDeploySettings.Environment.IsLocal &&
		a.DesiredState.Status == workspace.StatusDeployed &&
		!env.IsLocal &&
		a.AppManifest.AppConfig.ReportDeployment

	log.Info("Planning reconciliation...")

	plan, err := a.PlanReconciliation(ctx)

	if err != nil {
		return errors.Wrap(err, "planning reconciliation")
	}

	if len(plan) == 0 {
		log.Info("No actions needed to reconcile state.")
		return nil
	}

	if reportDeploy {

		cleanup, reportErr := a.ReportDeployment(ctx)
		if reportErr != nil {
			return reportErr
		}

		// ensure that the deployment is updated when we return.
		defer func() {
			if r := recover(); r != nil {
				var ok bool
				err, ok = r.(error)
				if ok {
					err = errors.Errorf("panicked with error: %s\n%s", err, debug.Stack())
				} else {
					err = errors.Errorf("panicked: %v\n%s", r, debug.Stack())
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
		stepCtx.Log().Info("Executing step...")
		err = step.Action(stepCtx)
		if err != nil {
			stepCtx.Log().WithError(err).Error("Deploy failed.")
			return errors.Wrapf(err, "step %q failed", step.Name)
		}
		stepCtx.Log().Info("Step complete.")
	}

	log.Debug("Plan executed.")

	return nil
}

func (a *AppDeploy) ReportDeployment(ctx BosunContext) (cleanup func(error), err error) {

	log := ctx.Log()
	env := ctx.Environment()

	log.Info("Deploy progress will be reported to github.")

	deployer, err := ctx.Bosun.GetDeployer(a.AppManifest.RepoRef())
	if err != nil {
		return nil, err
	}

	// create the deployment
	deployID, err := deployer.CreateDeploy(a.AppManifest.Branch, env.Name)
	if err != nil {
		return nil, errors.Wrap(err, "create deploy")
	}

	// ensure that the deployment is updated when we return.
	return func(failure error) {
		if failure != nil {
			_ = deployer.UpdateDeploy(deployID, "failure", failure.Error())
		} else {

			// log.Info("Move ready to go stories to UAT")
			// repoPath, err := git.GetRepoPath(a.AppManifest.AppConfig.FromPath)
			// if err != nil {
			// 	err = errors.Wrap(err, "get repo path")
			// }
			//
			// log.Info("Move ready to go stories to UAT if deploy succeed")
			// issueSvc, err := ctx.Bosun.GetIssueService(repoPath)
			// if err != nil {
			// 	err = errors.Wrap(err, "get issue service")
			// }
			//
			// segs := strings.Split(a.Repo, "/")
			// if len(segs) < 2 {
			// 	err = errors.Wrap(err, "incorrect segs")
			// }
			// org, repoName := segs[0], segs[1]
			// log.Infof("current org: %s", org)
			//
			// ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
			// // find the last successful deployment time
			// since, err := getLastSuccessfulDeploymentTime(client.Repositories, ctx, org, repoName)
			//
			//
			// closedIssues, err := issueSvc.GetIssuesFromCommitsSince(org, repoName, since)
			// if err != nil {
			// 	err = errors.Wrap(err, "get closed issues")
			// }
			//
			// log.Info("closed issues:", closedIssues)
			//
			// for _, closedIssue := range closedIssues {
			// 	issueNum := closedIssue.Number
			// 	issueRef := issues.NewIssueRef(org, repoName, issueNum)
			// 	parents, err := issueSvc.GetParentRefs(issueRef)
			// 	if err != nil {
			// 		err = errors.Wrap(err, "get parents for closed issue")
			// 	}
			//
			// 	log.Info("get parents ", parents)
			//
			// 	if len(parents) <= 0 {
			// 		continue
			// 	}
			// 	parent := parents[0]
			// 	parent.Repo = "stories"
			// 	parentIssueRef := issues.NewIssueRef(parent.Org, parent.Repo, parent.Number)
			// 	log.Info("dealing with parent story #", parent.Number)
			//
			// 	allChildren, err := issueSvc.GetChildRefs(parentIssueRef)
			// 	if err != nil {
			// 		err = errors.Wrap(err, "get all children of parent issue")
			// 	}
			//
			// 	var ok = true
			// 	for _, child := range allChildren {
			// 		if !child.IsClosed {
			// 			ok = false
			// 			break
			// 		}
			// 	}
			// 	if ok {
			// 		err = issueSvc.SetProgress(parentIssueRef, issues.ColumnWaitingForUAT)
			// 		if err != nil {
			// 			err = errors.Wrap(err, "error when move parent story to Waiting for UAT")
			// 		}
			// 		log.Info("move parent story to Waiting for UAT ", parentIssueRef.String())
			//
			_ = deployer.UpdateDeploy(deployID, "success", "")

		}
	}, err
}

func getLastSuccessfulDeploymentTime(rs *github.RepositoriesService, ctx context.Context, org, repo string) (string, error) {

	deployments, _, err := rs.ListDeployments(ctx, org, repo, nil)
	if err != nil {
		return "", errors.Wrap(err, "get deployments")
	}
	if len(deployments) < 1 {
		return "", nil
	}
	mostRecent := deployments[0]
	since := mostRecent.UpdatedAt
	return since.String(), nil
}

func (a *AppDeploy) diff(ctx BosunContext) (string, error) {

	args := omitStrings(a.makeHelmArgs(ctx), "--dry-run", "--debug")

	msg, err := pkg.NewShellExe("helm", "diff", "upgrade", a.AppManifest.Name, a.Chart(ctx)).
		WithArgs(args...).
		RunOut()

	if err != nil {
		return "", err
	} else {
		if msg == "" {
			ctx.Log().Debug("Diff detected no changes.")
		} else {
			ctx.Log().Debugf("Diff result:\n%s\n", msg)
		}
	}

	return msg, nil
}

func (a *AppDeploy) Delete(ctx BosunContext) error {
	args := []string{"delete"}
	if a.DesiredState.Status == workspace.StatusNotFound {
		args = append(args, "--purge")
	}
	args = append(args, a.AppManifest.Name)

	out, err := pkg.NewShellExe("helm", args...).RunOut()
	ctx.Log().Debug(out)
	return errors.Wrapf(err, "delete using args %v", args)
}

func (a *AppDeploy) Rollback(ctx BosunContext) error {
	args := []string{"rollback"}
	args = append(args, a.AppManifest.Name, a.helmRelease.Revision)
	// args = append(args, a.getNamespaceFlag(ctx)...)
	args = append(args, a.getHelmDryRunArgs(ctx)...)

	out, err := pkg.NewShellExe("helm", args...).RunOut()
	ctx.Log().Debug(out)
	return errors.Wrapf(err, "rollback using args %v", args)
}

func (a *AppDeploy) Install(ctx BosunContext) error {
	args := append([]string{"install", a.AppManifest.Name, a.Chart(ctx)}, a.makeHelmArgs(ctx)...)
	out, err := pkg.NewShellExe("helm", args...).RunOut()
	ctx.Log().Debug(out)
	return errors.Wrapf(err, "install using args %v", args)
}

func (a *AppDeploy) Upgrade(ctx BosunContext) error {
	args := append([]string{"upgrade", a.AppManifest.Name, a.Chart(ctx)}, a.makeHelmArgs(ctx)...)
	if a.DesiredState.Force {
		args = append(args, "--force")
	}
	out, err := pkg.NewShellExe("helm", args...).RunOut()
	ctx.Log().Debug(out)
	return errors.Wrapf(err, "upgrade using args %v", args)
}

func (a *AppDeploy) GetStatus() (string, error) {
	release, err := a.GetHelmRelease(a.AppManifest.Name)
	if err != nil {
		return "", err
	}
	if release == nil {
		return "NOTFOUND", nil
	}

	return release.Status, nil
}

func (a *AppDeploy) RouteToLocalhost(ctx BosunContext, namespace string) error {

	ctx = ctx.WithAppDeploy(a)

	ctx.Log().Info("Configuring app to route traffic to localhost.")

	if a.AppManifest.AppConfig.Minikube == nil || len(a.AppManifest.AppConfig.Minikube.RoutableServices) == 0 {
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

	client, err := kube.GetKubeClient()
	if err != nil {
		return errors.Wrap(err, "get kube client for tweaking service")
	}

	for _, routableService := range a.AppManifest.AppConfig.Minikube.RoutableServices {
		log := ctx.Log().WithField("routable_service", routableService.Name)

		log.Info("Updating service and endpoint...")

		svcClient := client.CoreV1().Services(namespace)
		svc, err := svcClient.Get(routableService.Name, metav1.GetOptions{})
		if err != nil {
			return errors.Wrap(err, "get service if it exists")
		}

		localhostPort := routableService.LocalhostPort
		if localhostPort == 0 {
			localhostPort = routableService.ExternalPort
		}
		internalPort := routableService.InternalPort
		if internalPort == 0 {
			internalPort = routableService.LocalhostPort
		}

		svc.Spec.ClusterIP = ""
		svc.Spec.Type = "ExternalName"
		svc.Spec.Ports = []v1.ServicePort{
			{
				Port:       int32(internalPort),
				TargetPort: intstr.IntOrString{},
				Name:       routableService.PortName,
				Protocol:   v1.ProtocolTCP,
			},
		}
		svc.Spec.Selector = nil

		log.Info("Updating service...")
		svc, err = svcClient.Update(svc)

		log.Info("Updated service.")

		endpointClient := client.CoreV1().Endpoints(namespace)

		endpoint, err := endpointClient.Get(routableService.Name, metav1.GetOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			return errors.Wrap(err, "get endpoint if it exists")
		}

		endpointExists := true
		if endpoint == nil {
			endpointExists = false
			endpoint = &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name: routableService.Name,
				},
			}
		}

		endpoint.Subsets = []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{IP: hostIP},
				},
				Ports: []v1.EndpointPort{
					{
						Port: int32(localhostPort),
						Name: routableService.PortName,
					},
				},
			},
		}

		if endpointExists {
			log.Info("Creating endpoint...")
			endpoint, err = endpointClient.Update(endpoint)
			log.Info("Created endpoint.")
		} else {
			log.Info("Updating endpoint...")
			endpoint, err = endpointClient.Create(endpoint)
			log.Info("Updated endpoint.")
		}

		if err != nil {
			return errors.Wrap(err, "creating endpoint")
		}

		log.Info("Updated service and endpoint.")
	}

	return nil
}

func (a *AppDeploy) makeHelmArgs(ctx BosunContext) []string {

	var args []string

	if !a.AppDeploySettings.UseLocalContent {
		args = append(args, "--version", a.AppManifest.Version.String())
	}

	args = append(args, a.getNamespaceFlag(ctx)...)

	args = append(args, "-f", ctx.Values.FilePath)

	args = append(args, a.getHelmDryRunArgs(ctx)...)

	return args
}

func (a *AppDeploy) getNamespaceFlag(ctx BosunContext) []string {
	namespace := a.getNamespaceName()

	return []string{"--namespace", namespace}
}

func (a *AppDeploy) getNamespaceName() string {
	if a.Namespace == "" {
		return "default"
	}
	return a.Namespace
}

func (a *AppDeploy) getHelmDryRunArgs(ctx BosunContext) []string {
	if ctx.GetParameters().Verbose {
		return []string{"--debug"}
	}
	if ctx.GetParameters().DryRun {
		return []string{"--dry-run"}
	}
	return []string{}
}

func (a *AppDeploy) Recycle(ctx BosunContext) error {
	ctx = ctx.WithAppDeploy(a)
	ctx.Log().Info("Deleting pods...")
	err := pkg.NewShellExe("kubectl", "delete", "--namespace", a.getNamespaceName(), "pods", "--selector=release="+a.AppManifest.AppConfig.Name).RunE()
	if err != nil {
		return err
	}
	ctx.Log().Info("Pods deleted, waiting for recreated pods to be ready.")

	for {
		podsReady := true
		out, err := pkg.NewShellExe("kubectl", "get", "pods", "--namespace", a.getNamespaceName(), "--selector=release="+a.AppManifest.AppConfig.Name,
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
		ctx.Log().Debugf("Waiting %s to check readiness again...", wait)
		select {
		case <-time.After(wait):
		case <-ctx.Ctx().Done():
			return ctx.Ctx().Err()
		}
	}

	ctx.Log().Info("Recycle complete.")

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
