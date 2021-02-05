package bosun

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg"
	actions "github.com/naveego/bosun/pkg/actions"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/helm"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"github.com/rs/xid"
	"github.com/stevenle/topsort"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
			core.LabelName:    a.Name,
			core.LabelPath:    a.FromPath,
			core.LabelVersion: a.Version.String(),
		})

		a.labels[core.LabelBranch] = filter.LabelFunc(func() string { return a.GetBranchName().String() })
		a.labels[core.LabelCommit] = filter.LabelFunc(a.GetCommit)

		if a.HasChart() {
			a.labels[core.LabelDeployable] = filter.LabelString("true")
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
			ConfigShared: core.ConfigShared{
				FromPath: dep.FromPath,
				Name:     dep.Name,
			},
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
		return core.ErrNotCloned
	}
	return nil
}

func (a *App) CheckOutBranch(name git.BranchName) error {
	if !a.IsRepoCloned() {
		return core.ErrNotCloned
	}
	local := a.Repo.LocalRepo
	if local.GetCurrentBranch() == name {
		return nil
	}
	if local.IsDirty() {
		return errors.Errorf("current branch %q is dirty", local.GetCurrentBranch())
	}

	err := local.CheckOut(name)
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

type BuildImageRequest struct {
	Pattern string
	Ctx BosunContext
}

func (a *App) BuildImages(req BuildImageRequest) error {

	ctx := req.Ctx

	var err error
	var re *regexp.Regexp

	if req.Pattern != "" {
		re, err = regexp.Compile(req.Pattern)
		if err != nil {
			return err
		}
	}


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

		fullName := image.GetFullName()

		if re != nil && !re.MatchString(fullName){
			ctx.Log().Infof("Skipping image %s because it did not match pattern %q", fullName, req.Pattern)
			continue
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
				"--tag", fullName,
				contextPath,
			}

			if ctx.GetParameters().Sudo {
				buildCommand = append([]string{"sudo"}, buildCommand...)
			}
		}

		for i := 0; i < len(image.BuildArgs); i++ {
			buildCommand = append(buildCommand, os.ExpandEnv(image.BuildArgs[i]))
		}

		ctx.Log().Infof("Building image %q from %q with context %q", image.ImageName, dockerfilePath, contextPath)
		_, err := pkg.NewShellExe(buildCommand[0], buildCommand[1:]...).
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

var featureBranchTagRE = regexp.MustCompile(`\W+`)

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

	originalVersion := a.Version

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
	// 	err = pkg.NewShellExe("npm", "--no-git-tag-version", "--allow-same-version", "version", bumpOrVersion).
	// 		WithDir(filepath.Dir(a.FromPath)).
	// 		RunE()
	// 	if err != nil {
	// 		return errors.Errorf("failed to update version in package.json: %s", err)
	// 	}
	// }

	if a.HasChart() {
		m, chartErr := a.getChartAsMap()
		if chartErr != nil {
			return chartErr
		}

		m["version"] = a.Version
		if a.BranchForRelease {
			m["appVersion"] = a.Version
		}
		chartErr = a.saveChart(m)
		if chartErr != nil {
			return chartErr
		}
	}

	err = a.FileSaver.Save()
	if err != nil {
		return errors.Wrap(err, "save parent file")
	}

	if wasDirty {
		log.Warn("Repo was dirty, will not commit bumped files.")
	} else {
		commitMsg := fmt.Sprintf("chore(version): bumped version from %s to %s", originalVersion, a.Version)
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

// ExportValues creates an ValueSetCollection instance with all the values
// for releasing this app, reified into their environments, including values from
// files and from the default values.yaml file for the chart.
func (a *App) ExportValues(ctx BosunContext) (values.ValueSetCollection, error) {
	ctx = ctx.WithApp(a)
	var defaultValues values.Values

	if a.HasChart() {
		chartRef := a.getAbsoluteChartPathOrChart(ctx)
		valuesYaml, err := pkg.NewShellExe(
			"helm", "inspect", "values",
			chartRef,
			"--version", a.Version.String(),
		).RunOut()
		if err != nil {
			return values.ValueSetCollection{}, errors.Errorf("load default values from %q: %s", chartRef, err)
		}
		defaultValues, err = values.ReadValues([]byte(valuesYaml))
		if err != nil {
			return values.ValueSetCollection{}, errors.Errorf("parse default values from %q: %s", chartRef, err)
		}
	} else {
		defaultValues = values.Values{}
	}

	out := a.Values.CanonicalizedCopy()

	out.DefaultValues = values.ValueSet{Static: defaultValues}.WithValues(out.DefaultValues)

	return out, nil
}

func (a *App) ExportActions(ctx BosunContext) ([]*actions.AppAction, error) {
	var err error
	var actionList []*actions.AppAction
	for _, action := range a.Actions {
		if len(action.When) == 1 && action.When[0] == actions.ActionManual {
			ctx.Log().Debugf("Skipping export of action %q because it is marked as manual.", action.Name)
		} else {
			err = action.MakeSelfContained(ctx)
			if err != nil {
				return nil, errors.Errorf("prepare action %q for release: %s", action.Name, err)
			}
			actionList = append(actionList, action)
		}
	}

	return actionList, nil
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

		appManifest = &AppManifest{
			AppConfig: appConfig,
			AppMetadata: &AppMetadata{
				Name:    appConfig.Name,
				Repo:    appConfig.RepoName,
				Version: a.Version,
				Branch:  a.GetBranchName().String(),
			},
		}

		err := appManifest.UpdateHashes()
		if a.Repo.CheckCloned() == nil {
			appManifest.Hashes.Commit = a.Repo.LocalRepo.GetCurrentCommit()
		}

		return err
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

func (a *App) GetManifestFromBranch(ctx BosunContext, branch string, makePortable bool) (*AppManifest, error) {

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
	if onBranch {
		if g.IsDirty() {
			ctx.Log().Warnf("Getting manifest from dirty branch %s, make sure you commit changes eventually.", branch)
		}
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

	if makePortable {
		err = manifest.MakePortable()
		if err != nil {
			return nil, err
		}
	}

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
