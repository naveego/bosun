package bosun

import (
	"fmt"
	"github.com/Masterminds/semver"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/helm"
	"github.com/pkg/errors"
	"github.com/stevenle/topsort"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type App struct {
	*AppConfig
	HelmRelease  *HelmRelease
	DesiredState AppState
	ActualState  AppState
	branch       string
	commit       string
	gitTag       string
	isCloned     bool
}

type AppsSortedByName []*App
type DependenciesSortedByTopology []Dependency

func NewApp(appConfig *AppConfig) *App {
	return &App{
		AppConfig: appConfig,
		isCloned:  true,
	}
}

func NewAppFromDependency(dep *Dependency) *App {
	return &App{
		AppConfig: &AppConfig{
			Name:    dep.Name,
			Version: dep.Version,
			Repo:    dep.Repo,
		},
		isCloned: false,
	}
}

func (a AppsSortedByName) Len() int {
	return len(a)
}

func (a AppsSortedByName) Less(i, j int) bool {
	return strings.Compare(a[i].Name, a[j].Name) < 0
}

func (a AppsSortedByName) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a *App) CheckRepoCloned() error {
	if !a.IsRepoCloned() {
		return ErrNotCloned
	}
	return nil
}

func (a *App) CloneRepo(ctx BosunContext, githubDir string) error {
	if a.IsRepoCloned() {
		return errors.New("repo is already cloned")
	}

	dir := filepath.Join(githubDir, a.Repo)
	err := pkg.NewCommand("git", "clone",
		"--depth", "1",
		"--no-single-branch",
		fmt.Sprintf("git@github.com:%s.git", a.Repo),
		dir).
		RunE()

	if err != nil {
		return err
	}

	return nil
}

func (a *App) PullRepo(ctx BosunContext) error {
	err := a.CheckRepoCloned()
	if err != nil {
		return err
	}

	g, _ := git.NewGitWrapper(a.FromPath)
	err = g.Pull()

	return err
}

func (a *App) IsRepoCloned() bool {

	if a.FromPath == "" {
		return false
	}

	if _, err := os.Stat(a.FromPath); os.IsNotExist(err) {
		return false
	}

	return true
}

func (a *App) GetBranch() string {
	if a.IsRepoCloned() {
		if a.branch == "" {
			g, _ := git.NewGitWrapper(a.FromPath)
			a.branch = g.Branch()
		}
	}
	return a.branch
}

func (a *App) GetReleaseFromBranch() string {
	b := a.GetBranch()
	if b != "" && strings.HasPrefix(b, "release/") {
		return strings.Replace(b, "release/", "", 1)
	}
	return ""
}

func (a *App) GetCommit() string {
	if a.IsRepoCloned() && a.commit == "" {
		g, _ := git.NewGitWrapper(a.FromPath)
		a.commit = strings.Trim(g.Commit(), "'")
	}
	return a.commit
}

func (a *App) HasChart() bool {
	return a.ChartPath != "" || a.Chart != ""
}

func (a *App) LoadActualState(diff bool, ctx BosunContext) error {
	ctx = ctx.WithDir(a.FromPath)

	a.ActualState = AppState{}

	log := pkg.Log.WithField("name", a.Name)

	if !a.HasChart() {
		return nil
	}

	if !ctx.Bosun.IsClusterAvailable() {
		log.Debug("Cluster not available.")

		a.ActualState.Status = "unknown"
		a.ActualState.Routing = "unknown"
		a.ActualState.Version = "unknown"

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

	releaseData, _ := pkg.NewCommand("helm", "get", a.Name).RunOut()

	if strings.Contains(releaseData, "routeToHost: true") {
		a.ActualState.Routing = RoutingLocalhost
	} else {
		a.ActualState.Routing = RoutingCluster
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

func (a *App) Dir() string {
	return filepath.Dir(a.FromPath)
}

func (a *App) GetRunCommand() (*exec.Cmd, error) {

	if a.RunCommand == nil || len(a.RunCommand) == 0 {
		return nil, errors.Errorf("no runCommand in %q", a.FromPath)
	}

	c := exec.Command(a.RunCommand[0], a.RunCommand[1:]...)
	c.Dir = a.Dir()
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	return c, nil
}

type Plan []PlanStep

type PlanStep struct {
	Name        string
	Description string
	Action      func(ctx BosunContext) error
}

func (a *App) PlanReconciliation(ctx BosunContext) (Plan, error) {

	ctx = ctx.WithDir(a.FromPath)

	if !ctx.Bosun.IsClusterAvailable() {
		return nil, errors.New("cluster not available")
	}

	var steps []PlanStep

	actual, desired := a.ActualState, a.DesiredState

	log := pkg.Log.WithField("name", a.Name)

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
		for i := range a.Actions {
			action := a.Actions[i]
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
		for i := range a.Actions {
			action := a.Actions[i]
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

func (a *App) Reconcile(ctx BosunContext) error {
	ctx = ctx.WithDir(a.FromPath).WithLog(ctx.Log.WithField("app", a.Name))

	log := ctx.Log

	if !a.HasChart() {
		log.Info("No chart defined for this app.")
		return nil
	}

	err := a.LoadActualState(true, ctx)
	if err != nil {
		return errors.Errorf("error checking actual state for %q: %s", a.Name, err)
	}

	params := ctx.GetParams()
	env := ctx.Env
	reportDeploy := !params.DryRun &&
		!params.NoReport &&
		a.DesiredState.Status == StatusDeployed &&
		!env.IsLocal &&
		a.ReportDeployment

	values, err := a.GetValuesMap(ctx)
	if err != nil {
		return errors.Errorf("create values map for app %q: %s", a.Name, err)
	}

	ctx = ctx.WithValues(values)

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
		log.Info("Deploy progress will be reported to github.")
		// create the deployment
		deployID, err := git.CreateDeploy(ctx.Dir, env.Name)

		// ensure that the deployment is updated when we return.
		defer func() {
			if err != nil {
				git.UpdateDeploy(ctx.Dir, deployID, "failure")
			} else {
				git.UpdateDeploy(ctx.Dir, deployID, "success")
			}
		}()

		if err != nil {
			return err
		}
	}

	for _, step := range plan {
		log.WithField("step", step.Name).WithField("description", step.Description).Info("Planned step.")
	}

	log.Info("Planning complete.")

	log.Debug("Executing plan...")

	for _, step := range plan {
		stepCtx := ctx.WithLog(log.WithField("step", step.Name))
		stepCtx.Log.Info("Executing step...")
		err := step.Action(stepCtx)
		if err != nil {
			return err
		}
		stepCtx.Log.Info("Step complete.")
	}

	log.Debug("Plan executed.")

	return nil
}

func (a *App) diff(ctx BosunContext) (string, error) {

	args := omitStrings(a.makeHelmArgs(ctx), "--dry-run", "--debug")

	msg, err := pkg.NewCommand("helm", "diff", "upgrade", a.Name, a.getChartRef(), "--version", a.Version).
		WithArgs(args...).
		RunOut()

	if err != nil {
		return "", err
	} else {
		if msg == "" {
			pkg.Log.Debug("Diff detected no changes.")
		} else {
			pkg.Log.Debugf("Diff result:\n%s\n", msg)

		}
	}

	return msg, nil
}

func (a *App) Delete(ctx BosunContext) error {
	args := []string{"delete"}
	if a.DesiredState.Status == StatusNotFound {
		args = append(args, "--purge")
	}
	args = append(args, a.Name)

	out, err := pkg.NewCommand("helm", args...).RunOut()
	pkg.Log.Debug(out)
	return err
}

func (a *App) Rollback(ctx BosunContext) error {
	args := []string{"rollback"}
	args = append(args, a.Name, a.HelmRelease.Revision)
	args = append(args, a.getHelmNamespaceArgs(ctx)...)
	args = append(args, a.getHelmDryRunArgs(ctx)...)

	out, err := pkg.NewCommand("helm", args...).RunOut()
	pkg.Log.Debug(out)
	return err
}

func (a *App) Install(ctx BosunContext) error {
	args := append([]string{"install", "--name", a.Name, a.getChartRef()}, a.makeHelmArgs(ctx)...)
	out, err := pkg.NewCommand("helm", args...).RunOut()
	pkg.Log.Debug(out)
	return err
}

func (a *App) Upgrade(ctx BosunContext) error {
	args := append([]string{"upgrade", a.Name, a.getChartRef()}, a.makeHelmArgs(ctx)...)
	out, err := pkg.NewCommand("helm", args...).RunOut()
	pkg.Log.Debug(out)
	return err
}

func (a *App) GetStatus() (string, error) {
	release, err := a.GetHelmRelease(a.Name)
	if err != nil {
		return "", err
	}
	if release == nil {
		return "NOTFOUND", nil
	}

	return release.Status, nil
}

func (a *App) GetValuesMap(ctx BosunContext) (map[string]interface{}, error) {
	values := Values{}.AddEnvAsPath(EnvPrefix, EnvAppVersion, a.Version).
		AddEnvAsPath(EnvPrefix, EnvAppBranch, a.GetBranch()).
		AddEnvAsPath(EnvPrefix, EnvAppCommit, a.GetCommit())

	if a.BranchForRelease {
		if ctx.Release == nil || ctx.Release.Transient {
			values["tag"] = a.Version
		} else {
			values["tag"] = fmt.Sprintf("%s-%s", a.Version, ctx.Release.Name)
		}
	} else {
		values["tag"] = "latest"
	}

	valuesConfig := a.GetValuesConfig(ctx)
	for _, f := range valuesConfig.Files {
		vf, err := ReadValuesFile(f)
		if err != nil {
			return nil, err
		}
		values.MergeInto(vf)
	}

	for k, v := range valuesConfig.Set {
		v.Resolve(ctx)
		values.AddPath(k, v.GetValue())
	}

	for k, v := range ctx.GetParams().ValueOverrides {
		values.AddPath(k, v)
	}

	return values, nil
}

func (a *App) getChartRef() string {
	if a.Chart != "" {
		return a.Chart
	}
	return a.ChartPath
}

func (a *App) getChartName() string {
	if a.Chart != "" {
		return a.Chart
	}
	name := filepath.Base(a.ChartPath)
	return fmt.Sprintf("helm.n5o.black/%s", name)
}

func (a *App) makeHelmArgs(ctx BosunContext) []string {

	var args []string

	args = append(args,
		"--set", fmt.Sprintf("domain=%s", ctx.Env.Domain))

	args = append(args, a.getHelmNamespaceArgs(ctx)...)

	values := a.GetValuesConfig(ctx)
	for k, v := range values.Set {
		v.Resolve(ctx)
		args = append(args, "--set", fmt.Sprintf("%s=%s", k, v.GetValue()))
	}

	for _, f := range values.Files {
		args = append(args, "-f", f)

	}

	if ctx.Env.IsLocal {
		args = append(args, "--set", "imagePullPolicy=IfNotPresent")
		if a.DesiredState.Routing == RoutingLocalhost {
			args = append(args, "--set", fmt.Sprintf("routeToHost=true"))
		} else {
			args = append(args, "--set", fmt.Sprintf("routeToHost=false"))
		}
	} else {
		args = append(args, "--set", "routeToHost=false")
	}

	args = append(args, a.getHelmDryRunArgs(ctx)...)

	return args
}

func (a *App) getHelmNamespaceArgs(ctx BosunContext) []string {
	if a.Namespace != "" && a.Namespace != "default" {
		return []string{"--namespace", a.Namespace}
	}
	return []string{}
}

func (a *App) getHelmDryRunArgs(ctx BosunContext) []string {
	if ctx.IsDryRun() {
		return []string{"--dry-run", "--debug"}
	}
	return []string{}
}

type HelmReleaseResult struct {
	Releases []*HelmRelease `yaml:"Releases"`
}
type HelmRelease struct {
	Name       string `yaml:"Name"`
	Revision   string `yaml:"Revision"`
	Updated    string `yaml:"Updated"`
	Status     string `yaml:"Status"`
	Chart      string `yaml:"Chart"`
	AppVersion string `yaml:"AppVersion"`
	Namespace  string `yaml:"Namespace"`
}

func (a *App) GetHelmRelease(name string) (*HelmRelease, error) {

	if a.HelmRelease == nil {
		releases, err := a.GetHelmList(fmt.Sprintf(`^%s$`, name))
		if err != nil {
			return nil, err
		}

		if len(releases) == 0 {
			return nil, nil
		}

		a.HelmRelease = releases[0]
	}

	return a.HelmRelease, nil
}

func (a *App) GetHelmList(filter ...string) ([]*HelmRelease, error) {

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

func (a *App) PublishChart(ctx BosunContext, force bool) error {
	if err := a.CheckRepoCloned(); err != nil {
		return err
	}

	branch := a.GetBranch()
	if branch != "master" && !strings.HasPrefix(branch, "release/") {
		ctx.Log.WithField("branch", branch).Warn("You should only publish the chart from the master or release branches.")
	}

	err := helm.PublishChart(a.ChartPath, force)
	return err
}

func (a *App) GetImageName(versionAndRelease ...string) string {
	project := "private"
	if a.HarborProject != "" {
		project = a.HarborProject
	}
	name := fmt.Sprintf("docker.n5o.black/%s/%s", project, a.Name)

	if len(versionAndRelease) > 0 {
		name = fmt.Sprintf("%s:%s", name, versionAndRelease[0])
	}
	if len(versionAndRelease) > 1 {
		name = fmt.Sprintf("%s-%s", name, versionAndRelease[1])
	}

	return name
}

func (a *App) PublishImage(ctx BosunContext) error {

	tags := []string{"latest", a.Version}

	branch := a.GetBranch()
	if branch != "master" && !strings.HasPrefix(branch, "release/") {
		ctx.Log.WithField("branch", branch).Warn("You should only push images from the master or release branches.")
	}

	release := a.GetReleaseFromBranch()
	if release != "" {
		tags = append(tags, fmt.Sprintf("%s-%s", a.Version, release))
	}

	name := a.GetImageName()

	for _, tag := range tags {
		err := pkg.NewCommand("docker", "tag", name, a.GetImageName(tag)).RunE()
		if err != nil {
			return err
		}
		err = pkg.NewCommand("docker", "push", a.GetImageName(tag)).RunE()
		if err != nil {
			return err
		}
	}
	return nil
}

func GetDependenciesInTopologicalOrder(apps map[string]*App, roots ...string) (DependenciesSortedByTopology, error) {

	const target = "__TARGET__"

	repos := map[string]string{}

	graph := topsort.NewGraph()

	graph.AddNode(target)

	for _, root := range roots {
		graph.AddNode(root)
		graph.AddEdge(target, root)
	}

	// add our root node to the graph

	for _, app := range apps {
		graph.AddNode(app.Name)
		for _, dep := range app.DependsOn {
			if repos[dep.Name] == "" || dep.Repo != "" {
				repos[dep.Name] = dep.Repo
			}

			// make sure dep is in the graph
			graph.AddNode(dep.Name)
			graph.AddEdge(app.Name, dep.Name)
		}
	}

	sortedNames, err := graph.TopSort(target)
	if err != nil {
		return nil, err
	}

	var result DependenciesSortedByTopology
	for _, name := range sortedNames {
		if name == target {
			continue
		}

		// exclude non-existent apps
		repo := repos[name]
		dep := Dependency{
			Name: name,
			Repo: repo,
		}
		if dep.Repo == "" {
			dep.Repo = "unknown"
		}
		result = append(result, dep)
	}

	return result, nil
}

func (a *App) MakeAppRelease(release *Release) (*AppRelease, error) {

	if !release.Transient && a.BranchForRelease {

		g, err := git.NewGitWrapper(a.FromPath)
		if err != nil {
			return nil, err
		}

		branchName := fmt.Sprintf("release/%s", release.Name)

		branches, err := g.Exec("branch", "-a")
		if err != nil {
			return nil, err
		}
		if strings.Contains(branches, branchName) {
			_, err := g.Exec("checkout", branchName)
			if err != nil {
				return nil, err
			}
			_, err = g.Exec("pull")
			if err != nil {
				return nil, err
			}
		} else {
			_, err = g.Exec("branch", branchName, "origin/master")
			if err != nil {
				return nil, err
			}
			_, err = g.Exec("checkout", branchName)
			if err != nil {
				return nil, err
			}
			_, err = g.Exec("push", "-u", "origin", branchName)
			if err != nil {
				return nil, err
			}
		}
	}

	r := &AppRelease{
		Name:      a.Name,
		BosunPath: a.FromPath,
		Version:   a.Version,
		Repo:      a.Repo,
		RepoPath:  a.RepoPath,
		App:       a,
		Branch:    a.GetBranch(),
		ChartName: a.getChartName(),
	}

	return r, nil

}

// BumpVersion bumps the version (including saving the source fragment
// and updating the chart. The `bump` parameter may be one of
// major|minor|patch|major.minor.patch. If major.minor.patch is provided,
// the version is set to that value.
func (a *App) BumpVersion(bump string) error {
	version, err := semver.NewVersion(bump)
	if err == nil {
		a.Version = version.String()
	}

	if err != nil {
		version, err = semver.NewVersion(a.Version)
		if err != nil {
			return errors.Errorf("app has invalid version %q: %s", a.Version, err)
		}
		var v2 semver.Version

		switch bump {
		case "major":
			v2 = version.IncMajor()
		case "minor":
			v2 = version.IncMinor()
		case "patch":
			v2 = version.IncPatch()
		default:
			return errors.Errorf("invalid version component %q (want major, minor, or patch)", bump)
		}
		a.Version = v2.String()
	}

	m, err := a.getChartAsMap()
	if err != nil {
		return err
	}

	m["version"] = a.Version
	err = a.saveChart(m)
	if err != nil {
		return err
	}

	return a.Fragment.Save()
}

func (a *App) getChartAsMap() (map[string]interface{}, error) {
	err := a.CheckRepoCloned()
	if err != nil {
		return nil, err
	}

	if a.ChartPath == "" {
		return nil, errors.New("chartPath not set")
	}

	path := filepath.Join(a.ChartPath, "Chart.yaml")
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var out map[string]interface{}
	err = yaml.Unmarshal(b, &out)
	return out, err
}

func (a *App) saveChart(m map[string]interface{}) error {
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	path := filepath.Join(a.ChartPath, "Chart.yaml")
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(path, b, stat.Mode())
	return err
}

func omitStrings(from []string, toOmit ...string) []string {
	var out []string
	for _, s := range from {
		matched := false
		for _, o := range toOmit {
			if o == s {
				matched = true
				continue
			}
		}
		if !matched {
			out = append(out, s)
		}
	}
	return out
}
