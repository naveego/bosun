package cmd

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environment"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

const (
	OutputTable = "table"
	OutputYaml  = "yaml"
)

func MustGetBosun(optionalParams ...cli.Parameters) *bosun.Bosun {
	b, err := getBosun(optionalParams...)
	if err != nil {
		log.Fatal(err)
	}

	return b
}

func MustGetPlatform(optionalParams ...cli.Parameters) (*bosun.Bosun, *bosun.Platform) {
	b := MustGetBosun()
	p, err := b.GetCurrentPlatform()
	if err != nil {
		log.Fatal(err)
	}
	return b, p
}

func mustGetActiveRelease(b *bosun.Bosun) *bosun.ReleaseManifest {
	r, err := getActiveRelease(b)
	if err != nil {
		log.Fatal(err)
	}
	return r
}
func getActiveRelease(b *bosun.Bosun) (*bosun.ReleaseManifest, error) {

	p, err := b.GetCurrentPlatform()
	if err != nil {
		return nil, err
	}

	r, err := p.GetReleaseManifestBySlot(bosun.SlotStable)

	return r, err
}

func mustGetRelease(p *bosun.Platform, requestedSlot string, allowedSlots ...string) *bosun.ReleaseManifest {

	r, err := getRelease(p, requestedSlot, allowedSlots...)
	if err != nil {
		log.Fatal(err)
	}
	return r
}

func getRelease(p *bosun.Platform, requestedSlot string, allowedSlots ...string) (*bosun.ReleaseManifest, error) {
	if len(allowedSlots) == 0 {
		allowedSlots = []string{bosun.SlotUnstable, bosun.SlotStable}
	}
	allowed := false
	for _, slot := range allowedSlots {
		if requestedSlot == slot {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, errors.Errorf("invalid slot %q (allowed slots are %+v)", requestedSlot, allowedSlots)
	}

	r, err := p.GetReleaseManifestBySlot(requestedSlot)
	if err != nil {
		return nil, err
	}

	return r, nil
}

func getBosun(optionalParams ...cli.Parameters) (*bosun.Bosun, error) {
	config, err := bosun.LoadWorkspace(viper.GetString(ArgBosunConfigFile))
	if err != nil {
		return nil, err
	}

	var params cli.Parameters
	if len(optionalParams) > 0 {
		params = optionalParams[0]
	}

	params.Sudo = params.Sudo || viper.GetBool(ArgGlobalSudo)
	params.Verbose = params.Verbose || viper.GetBool(ArgGlobalVerbose)
	params.DryRun = params.DryRun || viper.GetBool(ArgGlobalDryRun)
	params.NoReport = params.NoReport || viper.GetBool(ArgGlobalNoReport)
	params.Force = params.Force || viper.GetBool(ArgGlobalForce)
	params.ConfirmedEnv = viper.GetString(ArgGlobalConfirmedEnv)
	params.ProviderPriority = viper.GetStringSlice(ArgAppProviderPriority)

	if params.ValueOverrides == nil {
		params.ValueOverrides = map[string]string{}
	}

	for _, kv := range viper.GetStringSlice(ArgGlobalValues) {
		segs := strings.Split(kv, "=")
		if len(segs) != 2 {
			color.Red("invalid values flag value: %q (should be key=value)\n", kv)
		}
		params.ValueOverrides[segs[0]] = segs[1]
	}

	b, err := bosun.New(params, config)
	if err != nil {
		return nil, err
	}
	if !params.NoEnvironment {

		envFromEnv := os.Getenv(core.EnvEnvironment)
		envFromConfig := b.GetCurrentEnvironment().Name
		if envFromConfig != envFromEnv {
			_, _ = colorError.Fprintf(os.Stderr, "Bosun config indicates environment should be %[1]q, but the environment var %[2]s is %[3]q. You may want to run $(bosun env use %[1]s)\n\n",
				envFromConfig,
				core.EnvEnvironment,
				envFromEnv)
		}
	}

	return b, err
}

func mustGetApp(b *bosun.Bosun, names []string) *bosun.App {
	f := getFilterParams(b, names).PreferCurrent()
	return f.MustGetApp()
}

func MustYaml(i interface{}) string {
	b, err := yaml.Marshal(i)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func getAppDeploysFromApps(b *bosun.Bosun, repos []*bosun.App) ([]*bosun.AppDeploy, error) {
	var appReleases []*bosun.AppDeploy

	for _, app := range repos {
		if !app.HasChart() {
			continue
		}
		ctx := b.NewContext()
		ctx.Log().Debug("Creating transient release...")
		valueSetNames := util.ConcatStrings(b.GetCurrentEnvironment().ValueSetNames, viper.GetStringSlice(ArgAppValueSet))
		valueSets, err := b.GetValueSetSlice(valueSetNames)
		if err != nil {
			return nil, err
		}

		includeDeps := viper.GetBool(argDeployPlanAutoDeps)
		deploySettings := bosun.DeploySettings{
			SharedDeploySettings: bosun.SharedDeploySettings{
				Environment:     ctx.Environment(),
				UseLocalContent: true,
			},
			ValueSets:          valueSets,
			IgnoreDependencies: !includeDeps,
			Apps:               map[string]*bosun.App{},
		}

		manifest, err := app.GetManifest(ctx)
		if err != nil {
			return nil, err
		}

		appRelease, err := bosun.NewAppDeploy(ctx, deploySettings, manifest)
		if err != nil {
			return nil, errors.Errorf("error creating release for repo %q: %s", app.Name, err)
		}
		appReleases = append(appReleases, appRelease)
	}

	return appReleases, nil
}

type FilterParams struct {
	b       *bosun.Bosun
	Names   []string
	All     bool
	Include []string
	Exclude []string
	Labels  []string
}

func (f FilterParams) IsEmpty() bool {
	return len(f.Names) == 0 && len(f.Include) == 0 && len(f.Exclude) == 0 && len(f.Labels) == 0
}

func getFilterParams(b *bosun.Bosun, names []string) FilterParams {
	p := FilterParams{
		b:     b,
		Names: names,
		All:   viper.GetBool(ArgFilteringAll),
	}
	p.Labels = viper.GetStringSlice(ArgFilteringLabels)
	p.Include = viper.GetStringSlice(ArgFilteringInclude)
	p.Exclude = viper.GetStringSlice(ArgFilteringExclude)

	p.All = len(p.Labels) == 0 &&
		len(p.Include) == 0 &&
		len(p.Exclude) == 0 &&
		len(p.Names) == 0

	return p
}

// ApplyToDeploySettings will set a filter on the deploy settings if
// the filter is not empty.
func (f FilterParams) ApplyToDeploySettings(d *bosun.DeploySettings) {
	if !f.IsEmpty() {
		chain := f.Chain()
		d.Filter = &chain
	}
}

func (f FilterParams) IncludeCurrent() FilterParams {
	if f.IsEmpty() {
		app, err := getCurrentApp(f.b)
		if err == nil && app != nil {
			f.Names = []string{app.Name}
		}
	}
	return f
}

func (f FilterParams) PreferCurrent() FilterParams {
	if f.IsEmpty() {
		f.All = false
		app, err := getCurrentApp(f.b)
		if err == nil && app != nil {
			f.Names = []string{app.Name}
		}
	}
	return f
}

func (f FilterParams) Chain() filter.Chain {
	var include []filter.Filter
	var exclude []filter.Filter

	if f.All {
		include = append(include, filter.FilterMatchAll())
	} else if len(f.Names) > 0 {
		for _, name := range f.Names {
			include = append(include, filter.MustParse(core.LabelName, "==", name))
		}
	} else {
		labels := append(viper.GetStringSlice(ArgFilteringLabels), viper.GetStringSlice(ArgFilteringInclude)...)
		for _, label := range labels {
			include = append(include, filter.MustParse(label))
		}
	}

	if len(f.Exclude) > 0 {
		for _, label := range f.Exclude {
			exclude = append(exclude, filter.MustParse(label))
		}
	}

	chain := filter.Try().Including(include...).Excluding(exclude...)
	return chain
}

func (f FilterParams) MustGetApp() *bosun.App {
	app, err := f.GetApp()
	if err != nil {
		panic(err)
	}
	return app
}

func (f FilterParams) GetApp() (*bosun.App, error) {
	apps := f.b.GetAllApps().ToList()

	result, err := f.Chain().ToGetExactly(1).FromErr(apps)
	if err != nil {
		return nil, err
	}

	return result.(bosun.AppList)[0], nil
}

func (f FilterParams) GetApps() bosun.AppList {
	apps := f.b.GetAllApps().ToList()

	result := f.Chain().From(apps).(bosun.AppList).SortByName()

	return result
}

func (f FilterParams) GetAppsChain(chain filter.Chain) ([]*bosun.App, error) {
	apps := f.b.GetAllApps().ToList()

	result, err := chain.FromErr(apps)

	return result.(bosun.AppList), err
}

func mustGetAppsIncludeCurrent(b *bosun.Bosun, names []string) []*bosun.App {
	repos, err := getAppsIncludeCurrent(b, names)
	if err != nil {
		log.Fatal(err)
	}
	if len(repos) == 0 {
		color.Red("No apps found (provided names: %v).", names)
	}
	return repos
}

func (f FilterParams) GetAppDeploys() ([]*bosun.AppDeploy, error) {
	apps := f.GetApps()

	releases, err := getAppDeploysFromApps(f.b, apps)
	return releases, err
}

func (f FilterParams) MustGetAppDeploys() []*bosun.AppDeploy {
	appReleases, err := f.GetAppDeploys()
	if err != nil {
		log.Fatal(err)
	}
	return appReleases
}

func getCurrentApp(b *bosun.Bosun) (*bosun.App, error) {
	var bosunFile string
	var err error
	var matchedApp *bosun.App

	wd, _ := os.Getwd()
	bosunFile, err = util.FindFileInDirOrAncestors(wd, "bosun.yaml")
	if err != nil {
		return nil, err
	}

	ctx := b.NewContext()
	matchedApp, err = b.GetOrAddAppForPath(bosunFile)
	if err == nil {
		return matchedApp, nil
	}

	ctx.Log().Errorf("Could not get an app from bosun file at %s: %s", bosunFile, err)

	bosunFileDir := filepath.Dir(bosunFile)

	var appsUnderDirNames []string
	var appsUnderDir []*bosun.App
	for _, matchedApp = range b.GetAllApps() {
		if matchedApp.IsRepoCloned() && strings.HasPrefix(matchedApp.FromPath, bosunFileDir) {
			appsUnderDirNames = append(appsUnderDirNames, matchedApp.Name)
			appsUnderDir = append(appsUnderDir, matchedApp)
		}
	}
	sort.Strings(appsUnderDirNames)

	if len(appsUnderDir) == 0 {
		return nil, errors.Errorf("no apps found under current path %s", wd)
	}

	if len(appsUnderDir) == 1 {
		return appsUnderDir[0], nil
	}

	p := &promptui.Select{
		Label: "Multiple apps found in this repo, please choose one.",
		Items: appsUnderDirNames,
	}

	i, _, err := p.Run()
	if err != nil {
		return nil, err
	}

	return appsUnderDir[i], nil

}

// gets one or more apps matching names, or if names
// are valid file paths, imports the file at that path.
// if names is empty, tries to find a apps starting
// from the current directory
func getAppsIncludeCurrent(b *bosun.Bosun, names []string) ([]*bosun.App, error) {
	f := getFilterParams(b, names).IncludeCurrent()
	return f.GetApps(), nil
}

// gets all known apps, without attempting to discover new ones
func mustGetKnownApps(b *bosun.Bosun, names []string) []*bosun.App {
	apps, err := getKnownApps(b, names)
	if err != nil {
		log.Fatal(err)
	}
	return apps
}

// gets all known apps
func getKnownApps(b *bosun.Bosun, names []string) ([]*bosun.App, error) {
	f := getFilterParams(b, names)
	return f.GetApps(), nil
}

func checkExecutableDependency(exe string) {
	path, err := exec.LookPath(exe)
	check(err, "Could not find executable for %q", exe)
	pkg.Log.WithFields(logrus.Fields{"exe": exe, "path": path}).Debug("Found dependency.")
}

func confirm(msg string, args ...interface{}) bool {

	label := fmt.Sprintf(msg, args...)

	if !pkg.IsInteractive() {
		pkg.Log.WithField("label", label).Warn("No terminal attached, skipping confirmation.")
		return true
	}

	prompt := promptui.Prompt{
		Label:     label,
		IsConfirm: true,
	}

	_, err := prompt.Run()

	if err == promptui.ErrAbort {
		color.Red("User quit.")
		os.Exit(0)
	}

	return true
}

type handledError struct {
	msg string
}

func (h handledError) Error() string {
	return h.msg
}

func checkHandleMsg(msg string, err error) error {
	return checkHandle(err, msg)
}

func checkHandle(err error, msgAndArgs ...string) error {
	if err == nil {
		return nil
	}
	var msg string
	switch len(msgAndArgs) {
	case 0:
		msg = "Fatal error."
	case 1:
		msg = msgAndArgs[0]
	default:
		msg = fmt.Sprintf(msgAndArgs[0], msgAndArgs[1:])
	}

	w := new(strings.Builder)

	fmt.Fprintln(w, color.RedString(msg))
	fmt.Fprintln(w, color.YellowString(err.Error()))

	_, file, line, ok := runtime.Caller(1)
	if ok {
		fmt.Fprintln(w, color.BlueString("@ %s : line %d", file, line))
	}
	return handledError{msg: w.String()}
}

func check(err error, msgAndArgs ...string) {
	if checkHandle(err, msgAndArgs...) != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func checkMsg(msg string, err error) {
	check(err, msg)
}

var marketingReleaseFormat = regexp.MustCompile(`\d\d\d\d\.\d+\.\d+`)


type globalParameters struct {
	vaultToken string
	vaultAddr  string
	domain     string
}

func (p *globalParameters) init() error {

	if p.domain == "" {
		p.domain = viper.GetString(ArgGlobalDomain)
	}
	if p.domain == "" {
		return errors.New("domain not set")
	}
	if p.vaultToken == "" {
		p.vaultToken = viper.GetString(ArgVaultToken)
	}
	if p.vaultToken == "" {
		p.vaultToken = os.Getenv("VAULT_TOKEN")
	}
	if p.vaultToken == "" {
		return errors.New("vault token not set (try setting VAULT_TOKEN)")
	}

	if p.vaultAddr == "" {
		p.vaultAddr = viper.GetString(ArgVaultAddr)
	}

	if p.vaultAddr == "" {
		p.vaultAddr = os.Getenv("VAULT_ADDR")
	}

	if p.vaultAddr == "" {
		switch p.domain {
		case "n5o.red":
			p.vaultAddr = "http://vault.n5o.red"
		default:
			p.vaultAddr = fmt.Sprintf("https://vault.%s", p.domain)
		}
	}

	return nil
}

var (
	colorHeader = color.New(color.Bold, color.FgHiBlue)
	colorError  = color.New(color.FgRed)
	colorOK     = color.New(color.FgGreen, color.Bold)
)

func getResolvedValuesFromApp(b *bosun.Bosun, app *bosun.App) (*values.PersistableValues, error) {
	ctx := b.NewContext().WithDir(app.FromPath)

	appManifest, err := app.GetManifest(ctx)
	if err != nil {
		return nil, err
	}
	return getResolvedValuesFromAppManifest(b, appManifest)
}

func getResolvedValuesFromAppManifest(b *bosun.Bosun, appManifest *bosun.AppManifest) (*values.PersistableValues, error) {

	ctx := b.NewContext().WithDir(appManifest.AppConfig.FromPath)

	valueSets, err := getValueSetSlice(b, ctx.Environment())
	if err != nil {
		return nil, err
	}

	appDeploy, err := bosun.NewAppDeploy(ctx, bosun.DeploySettings{
		SharedDeploySettings: bosun.SharedDeploySettings{
			Environment: ctx.Environment(),
		},
		ValueSets: valueSets,
	}, appManifest)
	if err != nil {
		return nil, err
	}

	values, err := appDeploy.GetResolvedValues(ctx)
	if err != nil {
		return nil, err
	}

	return values, nil
}

// getValueSetSlice gets the value sets for the provided environment
// and for any additional value sets specified using --value-sets,
// and creates an additional valueSet from any --set parameters.
func getValueSetSlice(b *bosun.Bosun, env *environment.Environment) ([]values.ValueSet, error) {
	valueSetNames := util.ConcatStrings(env.ValueSetNames, viper.GetStringSlice(ArgAppValueSet))
	valueSets, err := b.GetValueSetSlice(valueSetNames)
	if err != nil {
		return nil, err
	}
	valueOverrides := map[string]string{}
	for _, set := range viper.GetStringSlice(ArgAppSet) {
		segs := strings.Split(set, "=")
		if len(segs) != 2 {
			return nil, errors.Errorf("invalid set (should be key=value): %q", set)
		}
		valueOverrides[segs[0]] = segs[1]
	}
	if len(valueOverrides) > 0 {
		overrideValueSet := values.ValueSet{
			Dynamic: map[string]*command.CommandValue{},
		}
		for k, v := range valueOverrides {
			overrideValueSet.Dynamic[k] = &command.CommandValue{Value: v}
		}
		valueSets = append(valueSets, overrideValueSet)
	}

	return valueSets, err
}

func getOrAddGitRoot(b *bosun.Bosun, dir string) (string, error) {
	roots := b.GetGitRoots()
	var err error
	if dir == "" {
		if len(roots) == 0 {
			p := promptui.Prompt{
				Label: "Provide git root (apps will be cloned to ./org/repo in the dir you specify)",
			}
			dir, err = p.Run()
			if err != nil {
				return "", err
			}
		} else {
			dir = roots[0]
		}
	}
	rootExists := false
	for _, root := range roots {
		if root == dir {
			rootExists = true
			break
		}
	}
	if !rootExists {
		b.AddGitRoot(dir)
		err = b.SaveAndReload()
		if err != nil {
			return "", err
		}
	}

	return dir, nil
}
