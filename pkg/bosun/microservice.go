package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Microservice struct {
	bosun *Bosun
	Config *MicroserviceConfig
	HelmRelease *HelmRelease
	DesiredState MicroserviceState
	ActualState *MicroserviceState
}

func (m *Microservice) LoadActualState() error {

	m.ActualState = new(MicroserviceState)

	log := pkg.Log.WithField("name", m.Config.Name)

	if !m.bosun.IsClusterAvailable() {
		log.Debug("Cluster not available.")
		return nil
	}

	log.Debug("Getting actual state...")

	release, err := pkg.NewCommand("helm", "get", m.Config.Name).RunOut()

	if err != nil {
		if strings.Contains(err.Error(), "not found"){
			return nil
		}
	}

	m.ActualState.Deployed = true

	if strings.Contains(release, "routeToHost: true") {
		m.ActualState.RouteToHost = true
	}

	return nil
}

func (m *Microservice) Dir() string {
	return filepath.Dir(m.Config.FromPath)
}

func (m *Microservice) GetRunCommand() (*exec.Cmd, error) {


	if m.Config.RunCommand == nil || len(m.Config.RunCommand) == 0{
		return nil, errors.Errorf("no runCommand in %q", m.Config.FromPath)
	}

	c := exec.Command(m.Config.RunCommand[0], m.Config.RunCommand[1:]...)
	c.Dir = m.Dir()
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	return c, nil
}

func (m *Microservice) Deploy() error {

	if !m.bosun.IsClusterAvailable() {
		return errors.New("cluster not available")
	}

	m.DesiredState.Deployed = true
	m.ActualState = new(MicroserviceState)

	pkg.Log.Info("Deploying...")
	status, err := m.GetStatus()
	if err != nil {
		return err
	}

	var actions []func() error

	switch status {
	case "NOTFOUND":
		actions = []func() error { m.Install}
	case "DEPLOYED":
		actions = []func() error { m.Upgrade}
	case "DELETED", "FAILED", "PENDING_UPGRADE":
		actions = []func() error { m.Delete, m.Install}
	default:
		return errors.Errorf("unrecognized status %q", status)
	}

	for _, action := range actions {
		err = action()
		if err != nil {
			return err
		}
	}

	m.ActualState.Deployed = true
	m.ActualState.RouteToHost = m.DesiredState.RouteToHost

	return nil
}

func (m *Microservice) Delete() error {
	return pkg.NewCommand("helm", "delete", "--purge", m.Config.Name).RunE()
}

func (m *Microservice) Install() error {
	args := append([]string{"install", "--name", m.Config.Name, m.Config.ChartPath}, m.makeHelmArgs()...)
	return pkg.NewCommand("helm", args...).RunE()
}

func (m *Microservice) Upgrade() error {
	args := append([]string{"upgrade", m.Config.Name, m.Config.ChartPath}, m.makeHelmArgs()...)
	args = append(args, "--set", fmt.Sprintf("routeToHost=%t", m.DesiredState.RouteToHost))

	return pkg.NewCommand("helm", args...).RunE()
}

func (m *Microservice) GetStatus() (string, error) {
	release, err := m.GetHelmRelease(m.Config.Name)
	if err != nil {
		return "", err
	}
	if release == nil {
		return "NOTFOUND", nil
	}

	return release.Status, nil
}

func (m *Microservice) makeHelmArgs() []string {

	env, err := m.bosun.GetCurrentEnvironment()
	if err != nil {
		panic(err)
	}

	envs := []string {
		"all",
		m.bosun.config.CurrentEnvironment,
	}

	var args []string

	for _, envName := range envs {

		values, ok := m.Config.Values[envName]
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
		args = append(args, "--set", fmt.Sprintf("routeToHost=%t", m.DesiredState.RouteToHost))
	} else {
		args = append(args, "--set", "routeToHost=false")
	}

	args = append(args, "--set", fmt.Sprintf("domain=%s", env.Domain))

	return args
}

type HelmReleaseResult struct {
	Releases []*HelmRelease `yaml:"Releases"`
}
type HelmRelease struct {
	Name string `yaml:"Name"`
	Revision string `yaml:"Revision"`
	Updated string `yaml:"Updated"`
	Status string `yaml:"Status"`
	Chart string `yaml:"Chart"`
	AppVersion string `yaml:"AppVersion"`
	Namespace string `yaml:"Namespace"`
}

func (m *Microservice) GetHelmRelease(name string) (*HelmRelease, error) {

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

func (m *Microservice) GetHelmList(filter ...string) ([]*HelmRelease, error) {

	args := append([]string{"list", "--all", "--output", "yaml"}, filter...)
	data, err := pkg.NewCommand("helm",  args...).RunOut()
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