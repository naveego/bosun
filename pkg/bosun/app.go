package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"github.com/stevenle/topsort"
	"gopkg.in/yaml.v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type App struct {
	bosun *Bosun
	AppConfig
	HelmRelease  *HelmRelease
	DesiredState AppState
	ActualState  AppState
}

type AppsSortedByName []*App
type DependenciesSortedByTopology []Dependency

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

func (a AppsSortedByName) Len() int {
	return len(a)
}

func (a AppsSortedByName) Less(i, j int) bool {
	return strings.Compare(a[i].Name, a[j].Name) < 0
}

func (a AppsSortedByName) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a *App) HasChart() bool {
	return a.ChartPath != "" || a.Chart != ""
}

func (a *App) GetValuesForEnvironment(name string) AppValuesConfig {
	if a.Values == nil {
		a.Values = map[string]AppValuesConfig{}
	}
	av, ok := a.Values[name]
	if !ok {
		av = NewAppValues()
		a.Values[name] = av
	}
	return av
}

func (a *App) LoadActualState(diff bool) error {

	ctx := a.bosun.NewContext(a.FromPath)

	a.ActualState = AppState{}

	log := pkg.Log.WithField("name", a.Name)

	if !a.HasChart() {
		return nil
	}

	if !a.bosun.IsClusterAvailable() {
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
	Name string
	Description string
	Action      func(ctx BosunContext) error
}

func (a *App) PlanReconciliation(ctx BosunContext) (Plan, error) {

	ctx = ctx.ForDir(a.FromPath)

	if !a.bosun.IsClusterAvailable() {
		return nil, errors.New("cluster not available")
	}

	var steps []PlanStep

	actual, desired := a.ActualState, a.DesiredState

	log := pkg.Log.WithField("name", a.Name)

	log.WithField("state", desired.String()).Debug("Desired state.")
	log.WithField("state", actual.String()).Debug("Actual state.")

	var (
		needsDelete  bool
		needsInstall bool
		needsUpgrade bool
	)

	if desired.Status == StatusNotFound || desired.Status == StatusDeleted {
		needsDelete = actual.Status != StatusDeleted && actual.Status != StatusNotFound
	} else {
		needsDelete = actual.Status == StatusFailed
		needsDelete = needsDelete || actual.Status == StatusPendingUpgrade
	}

	if desired.Status == StatusDeployed {
		if actual.Status == StatusNotFound {
			needsInstall = true
		} else {
			needsUpgrade = actual.Status != StatusDeployed
			needsUpgrade = needsUpgrade || actual.Routing != desired.Routing
			needsUpgrade = needsUpgrade || actual.Version != desired.Version
			needsUpgrade = needsUpgrade || actual.Diff != ""
			needsUpgrade = needsUpgrade || desired.Force
		}
	}

	if needsDelete {
		steps = append(steps, PlanStep{
			Name: "Delete",
			Description: "Delete release from kubernetes.",
			Action:      a.Delete,
		})
	}

	values, err := a.GetValuesMap(ctx)
	if err != nil {
		return nil, err
	}

	if needsInstall || needsUpgrade {
		for i := range a.Actions {
			action := a.Actions[i]
			if strings.Contains(string(action.Schedule), ActionBeforeDeploy) {
				steps = append(steps, PlanStep{
					Name:action.Name,
					Description:action.Description,
					Action: func(ctx BosunContext) error {
						return action.Execute(ctx, values)
					},
				})
			}
		}
	}

	if needsInstall {
		steps = append(steps, PlanStep{
			Name: "Install",
			Description: "Install chart to kubernetes.",
			Action:      a.Install,
		})
	}

	if needsUpgrade {
		steps = append(steps, PlanStep{
			Name: "Upgrade",
			Description: "Upgrade existing chart in kubernetes.",
			Action:      a.Upgrade,
		})
	}

	if needsInstall || needsUpgrade {
		for i := range a.Actions {
			action := a.Actions[i]
			if strings.Contains(string(action.Schedule), ActionAfterDeploy) {
				steps = append(steps, PlanStep{
					Name:action.Name,
					Description:action.Description,
					Action: func(ctx BosunContext) error {
						return action.Execute(ctx, values)
					},
				})
			}
		}
	}

	return steps, nil

}

func (a *App) diff(ctx BosunContext) (string, error) {

	args := omitStrings(a.makeHelmArgs(ctx), "--dry-run", "--debug")

	msg, err := pkg.NewCommand("helm", "diff", "upgrade", a.Name, a.getChartRef(), "--version", a.Version).
		WithArgs(args...).
		RunOut()

	if err != nil {
		return "", err
	} else {
		pkg.Log.Debug("Diff result:")
		pkg.Log.Debug(msg)
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

	values := Values{}

	valuesConfig, ok := a.Values[ctx.Env.Name]
	if ok {
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
	}

	return values, nil
}

func (a *App) getChartRef() string {
	if a.Chart != "" {
		return a.Chart
	}
	return a.ChartPath
}

func (a *App) makeHelmArgs(ctx BosunContext) []string {

	var args []string

	args = append(args,
		"--set", fmt.Sprintf("domain=%s", ctx.Env.Domain))

	if a.Namespace != "" && a.Namespace != "default" {
		args = append(args,
			"--namespace", a.Namespace)
	}

	values, ok := a.Values[ctx.Env.Name]
	if ok {
		for k, v := range values.Set {
			v.Resolve(ctx)
			args = append(args, "--set", fmt.Sprintf("%s=%s", k, v.GetValue()))
		}

		for _, f := range values.Files {
			args = append(args, "-f", f)
		}
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

	if a.bosun.params.DryRun {
		args = append(args, "--dry-run", "--debug")
	}

	return args
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
