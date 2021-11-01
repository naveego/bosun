package bosun

import (
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/brns"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environment"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/mirror"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type DeploymentPlan struct {
	core.ConfigShared `yaml:",inline"`
	ReleaseVersion    *semver.Version `yaml:"releaseVersion"`
	// Hash of the release this plan was based on, if any - used to detect that the plan is out of date
	BasedOnHash              string                   `yaml:"basedOnHash"`
	DirectoryPath            string                   `yaml:"-"`
	ProviderPriority         []string                 `yaml:"providerPriority"`
	SkipDependencyValidation bool                     `yaml:"skipDependencyValidation"`
	ValueOverrides           values.ValueSet          `yaml:"valueOverrides"`
	AppDeploymentProgress    []*AppDeploymentProgress `yaml:"deployedApps"`
	Apps                     []*AppDeploymentPlan     `yaml:"apps"`
	BundleInfo               *BundleInfo              `yaml:"bundleInfo,omitempty"`
	DeployApps               map[string]bool          `yaml:"deployApps"`
}

type AppDeploymentProgress struct {
	AppName   string    `yaml:"appName"`
	Stack     string    `yaml:"stack"`
	Hash      string    `yaml:"hash"`
	Timestamp time.Time `yaml:"timestamp"`
	Error     string    `yaml:"error,omitempty"`
}

type AppDeploymentProgressReport struct {
	Progress  AppDeploymentProgress
	Plan      AppDeploymentPlan
	OutOfSync bool
	Status    string
}

type BundleInfo struct {
	Environments map[string]*environment.Config
}

func LoadDeploymentPlanFromFile(path string) (*DeploymentPlan, error) {
	var out DeploymentPlan
	err := yaml.LoadYaml(path, &out)
	if err != nil {
		return &out, err
	}

	out.SetFromPath(path)

	for _, appPlan := range out.Apps {
		manifestPath := out.ResolveRelative(appPlan.ManifestPath)

		appPlan.Manifest, err = LoadAppManifestFromPathAndName(manifestPath, appPlan.Name)
		if err != nil {
			return nil, err
		}
	}

	return &out, nil
}

func (d *DeploymentPlan) Save() error {
	var err error

	if d.DirectoryPath == "" {
		if d.FromPath != "" {
			d.DirectoryPath = filepath.Dir(d.FromPath)
		} else {
			return errors.New("directoryPath and fromPath were both empty")
		}
	}

	_ = os.RemoveAll(d.DirectoryPath)
	if err = os.MkdirAll(d.DirectoryPath, 0700); err != nil {
		return err
	}

	for _, app := range d.Apps {
		savePath, saveErr := app.Manifest.Save(d.DirectoryPath)
		if saveErr != nil {
			return errors.Wrapf(err, "saving portable manifest for app %q from providers %+v", app.Name, d.ProviderPriority)
		}
		app.ManifestPath, _ = filepath.Rel(d.DirectoryPath, savePath)
	}

	return d.SavePlanFileOnly()
}

func (d *DeploymentPlan) SavePlanFileOnly() error {

	mirror.Sort(d.Apps, func(a, b *AppDeploymentPlan) bool {
		return strings.Compare(a.Name, b.Name) < 0
	})

	planPath := d.FromPath
	if planPath == "" {
		if d.DirectoryPath == "" {
			return errors.New("fromPath and directoryPath were both empty")
		}
		planPath = filepath.Join(d.DirectoryPath, "plan.yaml")
		d.SetFromPath(planPath)
	}

	err := yaml.SaveYaml(planPath, d)

	return err
}

type AddDeploymentProgressReports []AppDeploymentProgressReport

func (a AddDeploymentProgressReports) Headers() []string {
	return []string{
		"Name",
		"Last Deployed At",
		"In Sync with Cluster",
		"Status",
	}
}

func (a AddDeploymentProgressReports) Rows() [][]string {

	var rows [][]string

	for _, r := range a {

		timestamp := ""

		if !r.Progress.Timestamp.IsZero() {
			timestamp = r.Progress.Timestamp.String()
		}

		inSync := ""
		if r.OutOfSync {
			inSync = color.RedString("NO")
		} else {
			inSync = color.GreenString("YES")
		}

		rows = append(rows, []string{
			r.Plan.Name,
			timestamp,
			inSync,
			r.Status,
		})
	}

	return rows
}

func (d *DeploymentPlan) GetDeploymentProgressReportForStack(env *environment.Environment, stack *kube.Stack) AddDeploymentProgressReports {

	var reports []AppDeploymentProgressReport

	for _, plan := range d.Apps {

		report := AppDeploymentProgressReport{
			Plan: *plan,
		}

		for _, progress := range d.AppDeploymentProgress {

			if progress.AppName != plan.Name {
				continue
			}

			if progress.Stack != stack.Brn.String() {
				continue
			}

			report.Progress = *progress
		}

		if env.IsAppDisabled(plan.Name) {
			report.Status = "Disabled (by environment)"
			report.OutOfSync = true
		} else if stack.IsAppDisabled(plan.Name) {
			report.Status = "Disabled (by stack)"
			report.OutOfSync = true
		} else {

			if report.Progress.Hash == plan.Manifest.Hashes.Summarize() {

				report.Status = "Deployed"

			} else {
				report.OutOfSync = true

				if report.Progress.Timestamp.IsZero() {
					report.Status = "Never deployed"
				} else {
					report.Status = "Changed since deployed"
				}
			}
		}

		reports = append(reports, report)
	}

	return reports
}

func (d *DeploymentPlan) RecordProgress(app *AppDeploy, stack brns.StackBrn, e error) {

	errMessage := ""
	if e != nil {
		errMessage = e.Error()
	}

	for _, progress := range d.AppDeploymentProgress {
		if progress.AppName == app.Name && progress.Stack == stack.String() {
			progress.Timestamp = time.Now()
			progress.Hash = app.AppManifest.Hashes.Summarize()
			progress.Error = errMessage
			return
		}
	}

	d.AppDeploymentProgress = append(d.AppDeploymentProgress, &AppDeploymentProgress{
		AppName:   app.Name,
		Stack:     stack.String(),
		Hash:      app.AppManifest.Hashes.Summarize(),
		Timestamp: time.Now(),
		Error:     errMessage,
	})
}

func (d *DeploymentPlan) FindDeploymentPlanProgress(app *AppManifest, stack brns.StackBrn, ) *AppDeploymentProgress {
	for _, progress := range d.AppDeploymentProgress {
		if progress.AppName == app.Name &&
			progress.Stack == stack.String() &&
			progress.Hash == app.Hashes.Summarize() {
			return progress
		}
	}

	return nil
}

type AppDeploymentPlan struct {
	Name           string          `yaml:"name"`
	Tag            string          `yaml:"tag"`
	ValueOverrides values.ValueSet `yaml:"valueOverrides"`
	ManifestPath   string          `yaml:"manifestPath"`
	Manifest       *AppManifest    `yaml:"-"`
}
