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
	"time"
)

type AppRepo struct {
	*AppRepoConfig
	HelmRelease *HelmRelease
	branch      string
	commit      string
	gitTag      string
	isCloned    bool
}

type ReposSortedByName []*AppRepo
type DependenciesSortedByTopology []string

func NewApp(appConfig *AppRepoConfig) *AppRepo {
	return &AppRepo{
		AppRepoConfig: appConfig,
		isCloned:      true,
	}
}

func NewRepoFromDependency(dep *Dependency) *AppRepo {
	return &AppRepo{
		AppRepoConfig: &AppRepoConfig{
			Name:    dep.Name,
			Version: dep.Version,
			Repo:    dep.Repo,
		},
		isCloned: false,
	}
}

func (a ReposSortedByName) Len() int {
	return len(a)
}

func (a ReposSortedByName) Less(i, j int) bool {
	return strings.Compare(a[i].Name, a[j].Name) < 0
}

func (a ReposSortedByName) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a *AppRepo) CheckRepoCloned() error {
	if !a.IsRepoCloned() {
		return ErrNotCloned
	}
	return nil
}

func (a *AppRepo) CloneRepo(ctx BosunContext, githubDir string) error {
	if a.IsRepoCloned() {
		return nil
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

func (a *AppRepo) PullRepo(ctx BosunContext) error {
	err := a.CheckRepoCloned()
	if err != nil {
		return err
	}

	g, _ := git.NewGitWrapper(a.FromPath)
	err = g.Pull()

	return err
}

func (a *AppRepo) IsRepoCloned() bool {

	if a.FromPath == "" {
		return false
	}

	if _, err := os.Stat(a.FromPath); os.IsNotExist(err) {
		return false
	}

	return true
}

func (a *AppRepo) GetRepo() string {
	if a.Repo == "" {
		repoPath, _ := git.GetRepoPath(a.FromPath)
		org, repo := git.GetOrgAndRepoFromPath(repoPath)
		a.Repo = fmt.Sprintf("%s/%s", org, repo)
	}

	return a.Repo
}

func (a *AppRepo) GetBranch() string {
	if a.IsRepoCloned() {
		if a.branch == "" {
			g, _ := git.NewGitWrapper(a.FromPath)
			a.branch = g.Branch()
		}
	}
	return a.branch
}

func (a *AppRepo) GetReleaseFromBranch() string {
	b := a.GetBranch()
	if b != "" && strings.HasPrefix(b, "release/") {
		return strings.Replace(b, "release/", "", 1)
	}
	return ""
}

func (a *AppRepo) GetCommit() string {
	if a.IsRepoCloned() && a.commit == "" {
		g, _ := git.NewGitWrapper(a.FromPath)
		a.commit = strings.Trim(g.Commit(), "'")
	}
	return a.commit
}

func (a *AppRepo) HasChart() bool {
	return a.ChartPath != "" || a.Chart != ""
}

func (a *AppRepo) Dir() string {
	return filepath.Dir(a.FromPath)
}

func (a *AppRepo) GetRunCommand() (*exec.Cmd, error) {

	if a.RunCommand == nil || len(a.RunCommand) == 0 {
		return nil, errors.Errorf("no runCommand in %q", a.FromPath)
	}

	c := exec.Command(a.RunCommand[0], a.RunCommand[1:]...)
	c.Dir = a.Dir()
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	return c, nil
}

func (a *AppRepo) GetAbsolutePathToChart() string {
	return resolvePath(a.FromPath, a.ChartPath)
}

func (a *AppRepo) getAbsoluteChartPathOrChart(ctx BosunContext) string {
	if a.ChartPath != "" {
		return ctx.ResolvePath(a.ChartPath)
	}
	return a.Chart
}

func (a *AppRepo) getChartName() string {
	if a.Chart != "" {
		return a.Chart
	}
	name := filepath.Base(a.ChartPath)
	return fmt.Sprintf("helm.n5o.black/%s", name)
}

func (a *AppRepo) PublishChart(ctx BosunContext, force bool) error {
	if err := a.CheckRepoCloned(); err != nil {
		return err
	}

	branch := a.GetBranch()
	if branch != "master" && !strings.HasPrefix(branch, "release/") {
		if ctx.GetParams().Force {
			ctx.Log.WithField("branch", branch).Warn("You should only publish the chart from the master or release branches (overridden by --force).")
		} else {
			ctx.Log.WithField("branch", branch).Warn("You can only push charts from the master or release branches (override by setting the --force flag).")
			return nil
		}
	}

	err := helm.PublishChart(a.GetAbsolutePathToChart(), force)
	return err
}

// GetImageName returns the image name. If no arguments are provided,
// it will be tagged "latest"; if one arg is provided it will be used as the tag;
// if 2 args are provided it will be tagged "arg[0]-arg[1]".
func (a *AppRepo) GetImageName(versionAndRelease ...string) string {
	project := "private"
	if a.HarborProject != "" {
		project = a.HarborProject
	}
	name := fmt.Sprintf("docker.n5o.black/%s/%s", project, a.Name)

	switch len(versionAndRelease) {
	case 0:
		name = fmt.Sprintf("%s:latest", name)
	case 1:
		name = fmt.Sprintf("%s:%s", name, versionAndRelease[0])
	case 2:
		name = fmt.Sprintf("%s:%s-%s", name, versionAndRelease[0], versionAndRelease[1])
	}

	return name
}

func (a *AppRepo) PublishImage(ctx BosunContext) error {

	tags := []string{"latest", a.Version}

	branch := a.GetBranch()
	if branch != "master" && !strings.HasPrefix(branch, "release/") {
		if ctx.GetParams().Force {
			ctx.Log.WithField("branch", branch).Warn("You should only push images from the master or release branches (overridden by --force).")
		} else {
			ctx.Log.WithField("branch", branch).Warn("You can only push images from the master or release branches (override by setting the --force flag).")
			return nil
		}
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

func GetDependenciesInTopologicalOrder(apps map[string][]string, roots ...string) (DependenciesSortedByTopology, error) {

	const target = "__TARGET__"

	graph := topsort.NewGraph()

	graph.AddNode(target)

	for _, root := range roots {
		graph.AddNode(root)
		graph.AddEdge(target, root)
	}

	// add our root node to the graph

	for name, deps := range apps {
		graph.AddNode(name)
		for _, dep := range deps {
			// make sure dep is in the graph
			graph.AddNode(dep)
			graph.AddEdge(name, dep)
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

		result = append(result, name)
	}

	return result, nil
}

func (a *AppRepo) GetAppReleaseConfig(ctx BosunContext) (*AppReleaseConfig, error) {
	var err error
	ctx = ctx.WithAppRepo(a)

	isTransient := ctx.Release == nil || ctx.Release.Transient

	r := &AppReleaseConfig{
		Name:             a.Name,
		Namespace:        a.Namespace,
		Version:          a.Version,
		ReportDeployment: a.ReportDeployment,
		SyncedAt:         time.Now(),
	}

	if !isTransient && a.BranchForRelease {

		g, err := git.NewGitWrapper(a.FromPath)
		if err != nil {
			return nil, err
		}

		branchName := fmt.Sprintf("release/%s", ctx.Release.Name)

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

		r.Branch = a.GetBranch()
		r.Repo = a.GetRepo()
		r.Commit = a.GetCommit()

	}

	if isTransient {
		r.Chart = ctx.ResolvePath(a.ChartPath)
	} else {
		r.Chart = a.getChartName()
	}

	if a.BranchForRelease {
		r.Image = strings.Split(a.GetImageName(), ":")[0]
		if isTransient || ctx.Release == nil {
			r.ImageTag = "latest"
		} else {
			r.ImageTag = fmt.Sprintf("%s-%s", r.Version, ctx.Release.Name)
		}
	}

	r.Values, err = a.ExportValues(ctx)
	if err != nil {
		return nil, errors.Errorf("export values for release: %s", err)
	}

	r.Actions, err = a.ExportActions(ctx)
	if err != nil {
		return nil, errors.Errorf("export actions for release: %s", err)
	}

	for _, dep := range a.DependsOn {
		r.DependsOn = append(r.DependsOn, dep.Name)
	}

	return r, nil
}

// BumpVersion bumps the version (including saving the source fragment
// and updating the chart. The `bump` parameter may be one of
// major|minor|patch|major.minor.patch. If major.minor.patch is provided,
// the version is set to that value.
func (a *AppRepo) BumpVersion(ctx BosunContext, bump string) error {
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

	packageJSONPath := filepath.Join(filepath.Dir(a.FromPath), "package.json")
	if _, err = os.Stat(packageJSONPath); err == nil {
		ctx.Log.Info("package.json detected, its version will be updated.")
		err = pkg.NewCommand("npm", "--no-git-tag-version", "--allow-same-version", "version", bump).
			WithDir(filepath.Dir(a.FromPath)).
			RunE()
		if err != nil {
			return errors.Errorf("failed to update version in package.json: %s", err)
		}
	}

	m, err := a.getChartAsMap()
	if err != nil {
		return err
	}

	m["version"] = a.Version
	if a.BranchForRelease {
		m["appVersion"] = a.Version
	}
	err = a.saveChart(m)
	if err != nil {
		return err
	}

	return a.Fragment.Save()
}

func (a *AppRepo) getChartAsMap() (map[string]interface{}, error) {
	err := a.CheckRepoCloned()
	if err != nil {
		return nil, err
	}

	if a.ChartPath == "" {
		return nil, errors.New("chartPath not set")
	}

	path := filepath.Join(a.GetAbsolutePathToChart(), "Chart.yaml")
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var out map[string]interface{}
	err = yaml.Unmarshal(b, &out)
	return out, err
}

func (a *AppRepo) saveChart(m map[string]interface{}) error {
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	path := filepath.Join(a.GetAbsolutePathToChart(), "Chart.yaml")
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