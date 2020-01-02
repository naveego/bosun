package bosun

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/helm"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"github.com/rs/xid"
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
	Repo        *Repo // a pointer to the repo if bosun is aware of it
	HelmRelease *HelmRelease
	branch      string
	commit      string
	gitTag      string
	isCloned    bool
	labels      filter.Labels
	Provider    AppProvider
	AppManifest *AppManifest
}

func (a *App) ProviderName() string {
	if a.Provider == nil {
		return "unknown"
	}
	return a.Provider.String()
}

func (a *App) GetLabels() filter.Labels {
	if a.labels == nil {
		a.labels = filter.LabelsFromMap(map[string]string{
			LabelName:    a.Name,
			LabelPath:    a.FromPath,
			LabelVersion: a.Version.String(),
		})

		a.labels[LabelBranch] = filter.LabelFunc(func() string { return a.GetBranchName().String() })
		a.labels[LabelCommit] = filter.LabelFunc(a.GetCommit)

		if a.HasChart() {
			a.labels[LabelDeployable] = filter.LabelString("true")
		}

		for k, v := range a.Labels {
			a.labels[k] = v
		}
	}
	return a.labels
}

type DependenciesSortedByTopology []string

func NewApp(appConfig *AppConfig) *App {
	return &App{
		AppConfig: appConfig,
		isCloned:  true,
	}
}

func NewAppFromDependency(dep *Dependency) *App {
	return &App{
		AppConfig: &AppConfig{
			FromPath: dep.FromPath,
			Name:     dep.Name,
			Version:  dep.Version,
			RepoName: dep.Repo,
			IsRef:    true,
		},
		isCloned: false,
	}
}

type AppsSortedByName []*App

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

func (a *App) CheckOutBranch(name string) error {
	if !a.IsRepoCloned() {
		return ErrNotCloned
	}
	local := a.Repo.LocalRepo
	if local.GetCurrentBranch() == name {
		return nil
	}
	if local.IsDirty() {
		return errors.Errorf("current branch %q is dirty", local.GetCurrentBranch())
	}

	_, err := local.git().Exec("checkout", name)
	return err
}

func (a *App) GetLocalRepoPath() (string, error) {
	if !a.IsRepoCloned() {
		return "", errors.New("repo is not cloned")
	}
	return git.GetRepoPath(a.FromPath)
}

func (a *App) IsRepoCloned() bool {
	if a.Repo == nil {
		return false
	}
	return a.Repo.CheckCloned() == nil
}

func (a *App) GetRepoPath() string {
	if a.Repo == nil || a.Repo.LocalRepo == nil {
		return ""
	}

	return a.Repo.LocalRepo.Path
}

func (a *App) GetBranchName() git.BranchName {
	if a.IsRepoCloned() {
		return a.Repo.GetLocalBranchName()
	}
	return ""
}

func (a *App) GetCommit() string {
	if a.IsRepoCloned() && a.commit == "" {
		g, _ := git.NewGitWrapper(a.FromPath)
		a.commit = strings.Trim(g.GetCurrentCommit(), "'")
	}
	return a.commit
}

func (a *App) HasChart() bool {
	return a.ChartPath != "" || a.Chart != ""
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

func (a *App) GetAbsolutePathToChart() string {
	return resolvePath(a.FromPath, a.ChartPath)
}

func (a *App) getAbsoluteChartPathOrChart(ctx BosunContext) string {
	if a.IsFromManifest {
		return a.Chart
	}

	if a.ChartPath != "" {
		return ctx.ResolvePath(a.ChartPath)
	}
	return a.Chart
}

func (a *App) getChartName() string {
	if a.Chart != "" {
		return a.Chart
	}
	name := filepath.Base(a.ChartPath)
	// TODO: Configure chart repo at WS or File level.
	return fmt.Sprintf("helm.n5o.black/%s", name)
}

func (a *App) PublishChart(ctx BosunContext, force bool) error {
	if err := a.CheckRepoCloned(); err != nil {
		return err
	}

	branch := a.GetBranchName()
	if a.Branching.IsFeature(branch) || a.Branching.IsDevelop(branch) {
		if ctx.GetParameters().Force {
			ctx.Log().WithField("branch", branch).Warn("You should only publish the chart from the master or release branches (overridden by --force).")
		} else {
			ctx.Log().WithField("branch", branch).Warn("You can only push charts from the master or release branches (override by setting the --force flag).")
			return nil
		}
	}

	err := helm.PublishChart(a.getChartName(), a.GetAbsolutePathToChart(), force)
	return err
}

func (a *AppConfig) GetImages() []AppImageConfig {
	images := a.Images
	defaultProjectName := "private"
	if a.HarborProject != "" {
		defaultProjectName = a.HarborProject
	}
	if len(images) == 0 {
		images = []AppImageConfig{{ImageName: a.Name}}
	}

	var formattedImages []AppImageConfig
	for _, i := range images {
		if i.ProjectName == "" {
			i.ProjectName = defaultProjectName
		}

		formattedImages = append(formattedImages, i)
	}

	return formattedImages
}

// GetPrefixedImageNames returns the untagged names of the images for this repo.
func (a *App) GetPrefixedImageNames() []string {
	var prefixedNames []string
	for _, image := range a.GetImages() {
		prefixedNames = append(prefixedNames, image.GetFullName())
	}
	return prefixedNames
}

// GetImageName returns the image name with the tags appended. If no arguments are provided,
// it will be tagged "latest"; if one arg is provided it will be used as the tag;
// if 2 args are provided it will be tagged "arg[0]-arg[1]".
func (a *App) GetTaggedImageNames(versionAndRelease ...string) []string {
	var taggedNames []string
	names := a.GetPrefixedImageNames()
	for _, name := range names {
		taggedName := name
		switch len(versionAndRelease) {
		case 0:
			taggedName = fmt.Sprintf("%s:latest", taggedName)
		case 1:
			taggedName = fmt.Sprintf("%s:%s", taggedName, versionAndRelease[0])
		case 2:
			taggedName = fmt.Sprintf("%s:%s-%s", taggedName, versionAndRelease[0], versionAndRelease[1])
		}
		taggedNames = append(taggedNames, taggedName)
	}

	return taggedNames
}

func (a *App) BuildImages(ctx BosunContext) error {

	var report []string
	for _, image := range a.GetImages() {
		if image.ImageName == "" {
			return errors.New("imageName not set in image config (did you accidentally set `name` instead?)")
		}
		dockerfilePath := image.Dockerfile
		if dockerfilePath == "" {
			dockerfilePath = ctx.ResolvePath("Dockerfile")
		} else {
			dockerfilePath = ctx.ResolvePath(dockerfilePath)
		}
		contextPath := image.ContextPath
		if contextPath == "" {
			contextPath = filepath.Dir(dockerfilePath)
		} else {
			contextPath = ctx.ResolvePath(contextPath)
		}

		var buildCommand []string
		if len(image.BuildCommand) > 0 {
			buildCommand = image.BuildCommand
		} else {
			buildCommand = []string{
				"docker",
				"build",
				"-f", dockerfilePath,
				"--build-arg", fmt.Sprintf("VERSION_NUMBER=%s", a.Version),
				"--build-arg", fmt.Sprintf("COMMIT=%s", a.GetCommit()),
				"--build-arg", fmt.Sprintf("BUILD_NUMBER=%s", os.Getenv("BUILD_NUMBER")),
				"--tag", image.GetFullName(),
				contextPath,
			}

			if ctx.GetParameters().Sudo {
				buildCommand = append([]string{"sudo"}, buildCommand...)
			}
		}

		ctx.Log().Infof("Building image %q from %q with context %q", image.ImageName, dockerfilePath, contextPath)
		_, err := pkg.NewCommand(buildCommand[0], buildCommand[1:]...).
			WithEnvValue("VERSION_NUMBER", a.Version.String()).
			WithEnvValue("COMMIT", a.GetCommit()).
			WithEnvValue("BUILD_NUMBER", os.Getenv("BUILD_NUMBER")).
			RunOutLog()

		if err != nil {
			return errors.Wrapf(err, "build image %q from %q with context %q", image.ImageName, dockerfilePath, contextPath)
		}

		report = append(report, fmt.Sprintf("Built image from %q with context %q: %s", dockerfilePath, contextPath, image.GetFullName()))
	}

	fmt.Println()
	for _, line := range report {
		color.Green("%s\n", line)
	}

	return nil
}

func (a *App) PublishImages(ctx BosunContext) error {

	var report []string

	tags := []string{"latest", a.Version.String()}

	branch := a.GetBranchName()

	if a.Branching.IsFeature(branch) {
		if ctx.GetParameters().Force {
			ctx.Log().WithField("branch", branch).Warn("You should not push images from a feature branch (overridden by --force).")
		} else {
			ctx.Log().WithField("branch", branch).Warn("You cannot push images from a feature branch (override by setting the --force flag).")
			return nil
		}
	}

	if a.Branching.IsDevelop(branch) {
		tags = append(tags, "develop")
	}

	if a.Branching.IsMaster(branch) {
		tags = append(tags, "master")
	}

	if a.Branching.IsRelease(branch) {
		_, releaseVersion, err := a.Branching.GetReleaseNameAndVersion(branch)
		if err == nil {
			tags = append(tags, fmt.Sprintf("%s-%s", a.Version, releaseVersion))
		}
	}

	for _, tag := range tags {
		for _, taggedName := range a.GetTaggedImageNames(tag) {
			ctx.Log().Infof("Tagging and pushing %q", taggedName)
			untaggedName := strings.Split(taggedName, ":")[0]
			_, err := pkg.NewCommand("docker", "tag", untaggedName, taggedName).Sudo(ctx.GetParameters().Sudo).RunOutLog()
			if err != nil {
				return err
			}
			_, err = pkg.NewCommand("docker", "push", taggedName).Sudo(ctx.GetParameters().Sudo).RunOutLog()
			if err != nil {
				return err
			}
			report = append(report, fmt.Sprintf("Tagged and pushed %s", taggedName))
		}
	}

	fmt.Println()
	for _, line := range report {
		color.Green("%s\n", line)
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

//
// func (a *App) GetAppReleaseConfig(ctx BosunContext) (*AppReleaseConfig, error) {
// 	var err error
// 	ctx = ctx.WithApp(a)
//
// 	isTransient := ctx.Deploy == nil || ctx.Deploy.Transient
//
// 	r := &AppReleaseConfig{
// 		Name:             a.Name,
// 		Namespace:        a.Namespace,
// 		Version:          a.Version,
// 		ReportDeployment: a.ReportDeployment,
// 		SyncedAt:         time.Now(),
// 	}
//
// 	ctx.Log().Debug("Getting app release config.")
//
// 	if !isTransient && a.BranchForRelease {
//
// 		g, err := git.NewGitWrapper(a.FromPath)
// 		if err != nil {
// 			return nil, err
// 		}
//
// 		branchName := fmt.Sprintf("release/%s", ctx.Deploy.Name)
//
// 		branches, err := g.Exec("branch", "-a")
// 		if err != nil {
// 			return nil, err
// 		}
// 		if strings.Contains(branches, branchName) {
// 			ctx.Log().Info("Checking out release branch...")
// 			_, err := g.Exec("checkout", branchName)
// 			if err != nil {
// 				return nil, err
// 			}
// 			_, err = g.Exec("pull")
// 			if err != nil {
// 				return nil, err
// 			}
// 		} else {
//
// 			if ctx.Deploy.IsPatch {
// 				return nil, errors.New("patch release not implemented yet, you must create the release branch manually")
// 			}
//
// 			ctx.Log().Info("Creating release branch...")
// 			_, err = g.Exec("checkout", "master")
// 			if err != nil {
// 				return nil, errors.Wrap(err, "checkout master")
// 			}
// 			_, err = g.Exec("pull")
// 			if err != nil {
// 				return nil, errors.Wrap(err, "pull master")
// 			}
//
// 			_, err = g.Exec("branch", branchName, "origin/master")
// 			if err != nil {
// 				return nil, err
// 			}
// 			_, err = g.Exec("checkout", branchName)
// 			if err != nil {
// 				return nil, err
// 			}
// 			_, err = g.Exec("push", "-u", "origin", branchName)
// 			if err != nil {
// 				return nil, err
// 			}
// 		}
//
// 		r.Branch = a.GetBranchName()
// 		r.Repo = a.GetRepoPath()
// 		r.GetCurrentCommit = a.GetCommit()
//
// 	}
//
// 	if isTransient {
// 		r.Chart = ctx.ResolvePath(a.ChartPath)
// 	} else {
// 		r.Chart = a.getChartName()
// 	}
//
// 	if a.BranchForRelease {
// 		r.ImageNames = a.GetPrefixedImageNames()
// 		if isTransient || ctx.Deploy == nil {
// 			r.ImageTag = "latest"
// 		} else {
// 			r.ImageTag = fmt.Sprintf("%s-%s", r.Version, ctx.Deploy.Name)
// 		}
// 	}
//
// 	r.Values, err = a.ExportValues(ctx)
// 	if err != nil {
// 		return nil, errors.Errorf("export values for release: %s", err)
// 	}
//
// 	r.Actions, err = a.ExportActions(ctx)
// 	if err != nil {
// 		return nil, errors.Errorf("export actions for release: %s", err)
// 	}
//
// 	for _, dep := range a.DependsOn {
// 		r.DependsOn = append(r.DependsOn, dep.Name)
// 	}
//
// 	return r, nil
// }

// BumpVersion bumps the version (including saving the source fragment
// and updating the chart. The `bump` parameter may be one of
// major|minor|patch|major.minor.patch. If major.minor.patch is provided,
// the version is set to that value.
func (a *App) BumpVersion(ctx BosunContext, bumpOrVersion string) error {

	log := ctx.WithApp(a).Log()
	wasDirty := a.Repo.LocalRepo.IsDirty()

	version, err := NewVersion(bumpOrVersion)
	if err == nil {
		a.Version = version
	} else {
		bump := semver.Bump(bumpOrVersion)
		switch bump {
		case semver.BumpMajor:
			a.Version = a.Version.BumpMajor()
		case semver.BumpMinor:
			a.Version = a.Version.BumpMinor()
		case semver.BumpPatch:
			a.Version = a.Version.BumpPatch()
		case semver.BumpNone:
			return nil
		default:
			return errors.Errorf("invalid version component %q (want major, minor, or patch)", bumpOrVersion)
		}
	}

	// this package.json update is annoying and just makes builds slower right now so I'm disabling it:

	// packageJSONPath := filepath.Join(filepath.Dir(a.FromPath), "package.json")
	// if _, err = os.Stat(packageJSONPath); err == nil {
	// 	log.Info("package.json detected, its version will be updated.")
	// 	err = pkg.NewCommand("npm", "--no-git-tag-version", "--allow-same-version", "version", bumpOrVersion).
	// 		WithDir(filepath.Dir(a.FromPath)).
	// 		RunE()
	// 	if err != nil {
	// 		return errors.Errorf("failed to update version in package.json: %s", err)
	// 	}
	// }

	if a.HasChart() {
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
	}

	err = a.Parent.Save()
	if err != nil {
		return errors.Wrap(err, "save parent file")
	}

	if wasDirty {
		log.Warn("Repo was dirty, will not commit bumped files.")
	} else {
		commitMsg := fmt.Sprintf("chore(version): %s bump to %s", bumpOrVersion, a.Version)
		err = a.Repo.LocalRepo.Commit(commitMsg, ".")
		if err != nil {
			return errors.Wrap(err, "commit bumped files")
		}
	}

	return nil
}

func (a *App) getChartAsMap() (map[string]interface{}, error) {
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

func (a *App) saveChart(m map[string]interface{}) error {
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

// ExportValues creates an ValueSetMap instance with all the values
// for releasing this app, reified into their environments, including values from
// files and from the default values.yaml file for the chart.
func (a *App) ExportValues(ctx BosunContext) (values.ValueSetMap, error) {
	ctx = ctx.WithApp(a)
	var err error
	envs := map[string]*EnvironmentConfig{}
	for envNames := range a.Values {
		for _, envName := range strings.Split(envNames, ",") {
			if envName == values.ValueSetAll {
				continue
			}
			if _, ok := envs[envName]; !ok {
				env, err := ctx.Bosun.GetEnvironment(envName)
				if err != nil {
					ctx.Log().Warnf("App values include key for environment %q, but no such environment has been defined.", envName)
					continue
				}
				envs[envName] = env
			}
		}
	}
	var defaultValues values.Values

	if a.HasChart() {
		chartRef := a.getAbsoluteChartPathOrChart(ctx)
		valuesYaml, err := pkg.NewCommand(
			"helm", "inspect", "values",
			chartRef,
			"--version", a.Version.String(),
		).RunOut()
		if err != nil {
			return nil, errors.Errorf("load default values from %q: %s", chartRef, err)
		}
		defaultValues, err = values.ReadValues([]byte(valuesYaml))
		if err != nil {
			return nil, errors.Errorf("parse default values from %q: %s", chartRef, err)
		}
	} else {
		defaultValues = values.Values{}
	}

	valueCopy := a.Values.CanonicalizedCopy()

	for name, values := range valueCopy {

		if env, ok := envs[name]; ok {
			ctx = ctx.WithEnv(env)
		}

		values, err = values.WithFilesLoaded(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "loading files for value set %q", name)
		}
		// make sure values from bosun app overwrite defaults from helm chart
		static := defaultValues.Clone()
		static.Merge(values.Static)
		values.Static = static
		values.Files = nil
		valueCopy[name] = values
	}

	return valueCopy, nil
}

func (a *App) ExportActions(ctx BosunContext) ([]*AppAction, error) {
	var err error
	var actions []*AppAction
	for _, action := range a.Actions {
		if action.When == ActionManual {
			ctx.Log().Debugf("Skipping export of action %q because it is marked as manual.", action.Name)
		} else {
			err = action.MakeSelfContained(ctx)
			if err != nil {
				return nil, errors.Errorf("prepare action %q for release: %s", action.Name, err)
			}
			actions = append(actions, action)
		}
	}

	return actions, nil
}

func (a *App) GetManifest(ctx BosunContext) (*AppManifest, error) {

	if a.manifest != nil {
		// App already has a manifest, probably because it was created
		// from an AppConfig that was obtained from an AppManifest.
		return a.manifest, nil
	}

	var appManifest *AppManifest

	err := util.TryCatch(a.Name, func() error {

		appConfig := a.AppConfig
		var err error

		appConfig.Values, err = a.ExportValues(ctx)
		if err != nil {
			return errors.Errorf("export values for manifest: %s", err)
		}

		appConfig.Actions, err = a.ExportActions(ctx)
		if err != nil {
			return errors.Errorf("export actions for manifest: %s", err)
		}

		hashes := AppHashes{}

		if a.Repo.CheckCloned() == nil {
			hashes.Commit = a.Repo.LocalRepo.GetCurrentCommit()
		}

		hashes.AppConfig, err = util.HashToStringViaYaml(appConfig)

		appManifest = &AppManifest{
			AppConfig: appConfig,
			AppMetadata: &AppMetadata{
				Name:    appConfig.Name,
				Repo:    appConfig.RepoName,
				Version: a.Version,
				Branch:  a.GetBranchName().String(),
				Hashes:  hashes,
			},
		}

		return nil
	})

	return appManifest, err
}

func (a *App) GetMostRecentCommitFromBranch(ctx BosunContext, branch string) (string, error) {
	err := a.Repo.CheckCloned()
	if err != nil {
		return "", err
	}

	g, err := a.Repo.LocalRepo.Git()
	if err != nil {
		return "", err
	}

	err = g.Fetch()
	if err != nil {
		return "", err
	}

	hash, err := g.Exec("rev-parse", fmt.Sprintf("origin/%s", branch))
	if err != nil {
		return "", errors.Wrapf(err, "get tip commit hash for origin/%s", branch)
	}

	return hash, nil

}

func (a *App) GetManifestFromBranch(ctx BosunContext, branch string) (*AppManifest, error) {

	wsApp, err := ctx.Bosun.GetAppFromWorkspace(a.Name)
	if err != nil {
		return nil, err
	}

	g, err := wsApp.Repo.LocalRepo.Git()
	if err != nil {
		return nil, err
	}

	currentBranch := g.Branch()

	useWorktreeCheckout := false
	forced := ctx.GetParameters().Force
	onBranch := currentBranch == branch
	if forced && onBranch {
		ctx.Log().Warn("Skipping worktree checkout because --force parameter was provided.")
		useWorktreeCheckout = false
	} else if forced {
		return nil, errors.Errorf("--force provided but branch %q is not checked out (current branch is %s)", branch, currentBranch)
	} else {
		useWorktreeCheckout = true
	}

	bosunFile := wsApp.FromPath

	if useWorktreeCheckout {

		err = g.Fetch()
		if err != nil {
			return nil, err
		}
		repoDirName := filepath.Base(wsApp.Repo.LocalRepo.Path)
		worktreePath := fmt.Sprintf("/tmp/%s-worktree", repoDirName)
		tmpBranchName := fmt.Sprintf("worktree-%s", xid.New())
		_, err = g.Exec("branch", "--track", tmpBranchName, fmt.Sprintf("origin/%s", branch))
		if err != nil {
			return nil, errors.Wrapf(err, "checking out tmp branch tracking origin/%q", branch)
		}
		defer func() {
			ctx.Log().Debugf("Deleting worktree %s", worktreePath)
			_, _ = g.Exec("checkout", currentBranch)
			_, _ = g.Exec("worktree", "remove", worktreePath)
			_, _ = g.Exec("branch", "-D", tmpBranchName)
		}()

		ctx.Log().Debugf("Creating worktree for %s at %s", tmpBranchName, worktreePath)
		_, _ = g.Exec("worktree", "remove", worktreePath)
		_, err = g.Exec("worktree", "add", worktreePath, tmpBranchName)
		if err != nil {
			return nil, errors.Wrapf(err, "create work tree for checking out branch %q", tmpBranchName)
		}

		// bosunFile should now be pulled from the worktree
		bosunFile = strings.Replace(wsApp.FromPath, wsApp.Repo.LocalRepo.Path, "", 1)
		bosunFile = filepath.Join(worktreePath, bosunFile)
	}

	provider := NewFilePathAppProvider(ctx.Log())

	app, err := provider.GetAppByPathAndName(bosunFile, wsApp.Name)
	if err != nil {
		return nil, err
	}

	ctx.Log().Infof("Creating manifest from branch %q...", branch)
	manifest, err := app.GetManifest(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "create manifest from branch %q", branch)
	}

	// set branch to requested branch, not the temp branch
	manifest.Branch = branch

	return manifest, nil
}

func (a *App) GetMostRecentReleaseVersion() *semver.Version {
	for _, entry := range a.ReleaseHistory {
		version, err := semver.NewVersion(entry.ReleaseVersion)
		if err != nil {
			return &version
		}
	}
	return nil
}
