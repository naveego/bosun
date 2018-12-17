package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type App struct {
	bosun *Bosun
	AppConfig
	HelmRelease  *HelmRelease
	DesiredState AppState
	ActualState  AppState
}

func (m *App) LoadActualState(diff bool) error {

	m.ActualState = AppState{}

	log := pkg.Log.WithField("name", m.Name)

	if !m.bosun.IsClusterAvailable() {
		log.Debug("Cluster not available.")

		m.ActualState.Status = "unknown"
		m.ActualState.Routing = "unknown"
		m.ActualState.Version = "unknown"

		return nil
	}

	log.Debug("Getting actual state...")

	release, err := m.GetHelmRelease(m.Name)

	if err != nil || release == nil {
		if release == nil || strings.Contains(err.Error(), "not found") {
			m.ActualState.Status = StatusNotFound
			m.ActualState.Routing = RoutingNA
			m.ActualState.Version = ""
		} else {
			m.ActualState.Error = err
		}
		return nil
	}

	m.ActualState.Status = release.Status

	releaseData, _ := pkg.NewCommand("helm", "get", m.Name).RunOut()

	if strings.Contains(releaseData, "routeToHost: true") {
		m.ActualState.Routing = RoutingLocalhost
	} else {
		m.ActualState.Routing = RoutingCluster
	}

	if diff {
		if m.ActualState.Status == StatusDeployed {
			m.ActualState.Diff, m.ActualState.Error = m.diff()
		}
	}

	return nil
}

func (m *App) Dir() string {
	return filepath.Dir(m.FromPath)
}

func (m *App) GetRunCommand() (*exec.Cmd, error) {

	if m.RunCommand == nil || len(m.RunCommand) == 0 {
		return nil, errors.Errorf("no runCommand in %q", m.FromPath)
	}

	c := exec.Command(m.RunCommand[0], m.RunCommand[1:]...)
	c.Dir = m.Dir()
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	return c, nil
}

type Plan []PlanStep

type PlanStep struct {
	Description string
	Action      func() error
}

func (p Plan) Execute() error {
	for _, step := range p {
		err := step.Action()
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *App) PlanReconciliation() (Plan, error) {

	if !m.bosun.IsClusterAvailable() {
		return nil, errors.New("cluster not available")
	}

	var steps []PlanStep

	actual, desired := m.ActualState, m.DesiredState

	if actual.Status == desired.Status &&
		actual.Routing == desired.Routing &&
		actual.Version == desired.Version &&
		!desired.Force {
		return nil, nil
	}

	var (
		needsDelete  bool
		needsInstall bool
		needsUpgrade bool
	)

	if desired.Status == StatusNotFound {
		needsDelete = true
	} else {
		switch actual.Status {
		case StatusDeployed, StatusNotFound:
			needsDelete = false
		default:
			needsDelete = true
		}
	}

	if desired.Status == StatusDeployed {
		if needsDelete || actual.Status == StatusNotFound {
			needsInstall = true
		} else {
			if actual.Routing != desired.Routing ||
				actual.Version != desired.Version ||
				actual.Diff != "" ||
				desired.Force {
				needsUpgrade = true
			}
		}
	}

	if needsDelete {
		steps = append(steps, PlanStep{
			Description: "Delete",
			Action:      m.Delete,
		})
	}

	if needsInstall {
		steps = append(steps, PlanStep{
			Description: "Install",
			Action:      m.Install,
		})
	}

	if needsUpgrade {
		steps = append(steps, PlanStep{
			Description: "Upgrade",
			Action:      m.Upgrade,
		})
	}

	return steps, nil

}

func (m *App) diff() (string, error) {
	msg, err := pkg.NewCommand("helm", "diff", "upgrade", m.Name, m.getChartRef(), "--version", strconv.Quote(m.Version)).
		WithArgs(m.makeHelmArgs()...).
		RunOut()

	if err != nil {
		return "", err
	} else {
		pkg.Log.Debug("Diff result:")
		pkg.Log.Debug(msg)
	}

	return msg, nil
}

func (m *App) Delete() error {
	return pkg.NewCommand("helm", "delete", "--purge", m.Name).RunE()
}

func (m *App) Install() error {
	args := append([]string{"install", "--name", m.Name, m.getChartRef()}, m.makeHelmArgs()...)
	return pkg.NewCommand("helm", args...).RunE()
}

func (m *App) Upgrade() error {
	args := append([]string{"upgrade", m.Name, m.getChartRef()}, m.makeHelmArgs()...)
	return pkg.NewCommand("helm", args...).RunE()
}

func (m *App) GetStatus() (string, error) {
	release, err := m.GetHelmRelease(m.Name)
	if err != nil {
		return "", err
	}
	if release == nil {
		return "NOTFOUND", nil
	}

	return release.Status, nil
}

func (m *App) getChartRef() string {
	if m.Chart != "" {
		return m.Chart
	}
	return m.ChartPath
}

func (m *App) makeHelmArgs() []string {

	env, err := m.bosun.GetCurrentEnvironment()
	if err != nil {
		panic(err)
	}

	envs := []string{
		"all",
		m.bosun.rootConfig.CurrentEnvironment,
	}

	var args []string

	for _, envName := range envs {

		values, ok := m.Values[envName]
		if !ok {
			continue
		}

		for k, v := range values.Set {
			args = append(args, "--set", fmt.Sprintf("%s=%s", k, v))
		}

		for _, f := range values.Files {
			args = append(args, "-f", f)
		}
	}

	if env.Name == "red" {
		args = append(args, "--set", "imagePullPolicy=IfNotPresent")
		if m.DesiredState.Routing == RoutingLocalhost {
			args = append(args, "--set", fmt.Sprintf("routeToHost=true"))
		} else {
			args = append(args, "--set", fmt.Sprintf("routeToHost=false"))
		}
	} else {
		args = append(args, "--set", "routeToHost=false")
	}

	args = append(args, "--set", fmt.Sprintf("domain=%s", env.Domain))

	if m.bosun.params.DryRun {
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

func (m *App) GetHelmRelease(name string) (*HelmRelease, error) {

	if m.HelmRelease == nil {
		releases, err := m.GetHelmList(name)
		if err != nil {
			return nil, err
		}

		if len(releases) == 0 {
			return nil, nil
		}

		m.HelmRelease = releases[0]
	}

	return m.HelmRelease, nil
}

func (m *App) GetHelmList(filter ...string) ([]*HelmRelease, error) {

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
