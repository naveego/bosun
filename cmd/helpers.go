package cmd

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const (
	OutputTable = "table"
	OutputYaml  = "yaml"
)

func addCommand(parent *cobra.Command, child *cobra.Command, flags ...func(cmd *cobra.Command)) *cobra.Command {
	for _, fn := range flags {
		fn(child)
	}
	parent.AddCommand(child)

	return child
}

func mustGetBosun(optionalParams ...bosun.Parameters) *bosun.Bosun {
	b, err := getBosun(optionalParams...)
	if err != nil {
		log.Fatal(err)
	}

	envFromEnv := os.Getenv(bosun.EnvEnvironment)
	envFromConfig := b.GetCurrentEnvironment().Name
	if envFromConfig != envFromEnv {
		colorError.Printf("Bosun config indicates environment should be %[1]q, but the environment var %[2]s is %[3]q. You may want to run $(bosun env %[1]s)",
			envFromConfig,
			bosun.EnvEnvironment,
			envFromEnv)
	}

	return b
}

func mustGetCurrentRelease(b *bosun.Bosun) *bosun.Release {
	r, err := b.GetCurrentRelease()
	if err != nil {
		log.Fatal(err)
	}

	whitelist := viper.GetStringSlice(ArgInclude)
	if len(whitelist) > 0 {
		toReleaseSet := map[string]bool{}
		for _, r := range whitelist {
			toReleaseSet[r] = true
		}
		for k, app := range r.AppReleases {
			if !toReleaseSet[k] {
				pkg.Log.Warnf("Skipping %q because it was not listed in the --%s flag.", k, ArgInclude)
				app.DesiredState.Status = bosun.StatusUnchanged
				app.Excluded = true
			}
		}
	}

	blacklist := viper.GetStringSlice(ArgExclude)
	for _, name := range blacklist {
		pkg.Log.Warnf("Skipping %q because it was excluded by the --%s flag.", name, ArgExclude)
		if app, ok := r.AppReleases[name]; ok {
			app.DesiredState.Status = bosun.StatusUnchanged
			app.Excluded = true
		}
	}

	return r
}

func getBosun(optionalParams ...bosun.Parameters) (*bosun.Bosun, error) {
	config, err := bosun.LoadWorkspace(viper.GetString(ArgBosunConfigFile))
	if err != nil {
		return nil, err
	}

	var params bosun.Parameters
	if len(optionalParams) > 0 {
		params = optionalParams[0]
	}

	params.Verbose = viper.GetBool(ArgGlobalVerbose)
	params.DryRun = viper.GetBool(ArgGlobalDryRun)
	params.NoReport = viper.GetBool(ArgGlobalNoReport)
	params.Force = viper.GetBool(ArgGlobalForce)

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

	return bosun.New(params, config)
}

func mustGetApp(b *bosun.Bosun, names []string) *bosun.AppRepo {
	apps, err := getAppRepos(b, names)
	if err != nil {
		log.Fatal(err)
	}
	if len(apps) == 0 {
		log.Fatalf("no apps matched %v", names)
	}
	if len(apps) > 1 {
		log.Fatalf("%d apps match %v", len(apps), names)
	}
	return apps[0]
}

func MustYaml(i interface{}) string {
	b, err := yaml.Marshal(i)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func getAppReleasesFromApps(b *bosun.Bosun, repos []*bosun.AppRepo) ([]*bosun.AppRelease, error) {
	var appReleases []*bosun.AppRelease

	for _, appRepo := range repos {
		if !appRepo.HasChart() {
			continue
		}
		ctx := b.NewContext()
		appRelease, err := bosun.NewAppReleaseFromRepo(ctx, appRepo)
		if err != nil {
			return nil, errors.Errorf("error creating release for repo %q: %s", appRepo.Name, err)
		}
		appReleases = append(appReleases, appRelease)
	}

	return appReleases, nil
}

func mustGetAppRepos(b *bosun.Bosun, names []string) []*bosun.AppRepo {
	repos, err := getAppRepos(b, names)
	if err != nil {
		log.Fatal(err)
	}
	if len(repos) == 0 {
		color.Red("No apps found (provided names: %v).", names)
	}
	return repos
}

func mustGetAppReleases(b *bosun.Bosun, names []string) []*bosun.AppRelease {
	repos, err := getAppRepos(b, names)
	if err != nil {
		log.Fatal(err)
	}
	releases, err := getAppReleasesFromApps(b, repos)
	if err != nil {
		log.Fatal(err)
	}
	return releases
}

type getAppReposOptions struct {
	ifNoFiltersGetAll   bool
	ifNoMatchGetAll     bool
	ifNoMatchGetCurrent bool
}

// gets one or more apps matching names, or if names
// are valid file paths, imports the file at that path.
// if names is empty, tries to find a apps starting
// from the current directory
func getAppReposOpt(b *bosun.Bosun, names []string, opt getAppReposOptions) ([]*bosun.AppRepo, error) {

	apps := b.GetAppsSortedByName()

	includeFilters := getIncludeFilters(names)
	excludeFilters := getExcludeFilters()

	if opt.ifNoFiltersGetAll && len(includeFilters) == 0 && len(excludeFilters) == 0 {
		return apps, nil
	}

	filtered := bosun.ApplyFilter(apps, true, includeFilters).(bosun.ReposSortedByName)
	filtered = bosun.ApplyFilter(filtered, false, excludeFilters).(bosun.ReposSortedByName)

	if len(filtered) > 0 {
		return filtered, nil
	}

	if opt.ifNoMatchGetAll {
		return apps, nil
	}

	var err error

	if opt.ifNoMatchGetCurrent {
		var bosunFile string

		wd, _ := os.Getwd()
		bosunFile, err = findFileInDirOrAncestors(wd, "bosun.yaml")
		if err != nil {
			return nil, err
		}

		app, err := b.GetOrAddAppForPath(bosunFile)
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	return apps, err
}

// gets one or more apps matching names, or if names
// are valid file paths, imports the file at that path.
// if names is empty, tries to find a apps starting
// from the current directory
func getAppRepos(b *bosun.Bosun, names []string) ([]*bosun.AppRepo, error) {
	return getAppReposOpt(b, names, getAppReposOptions{ifNoMatchGetCurrent: true})
}

func getIncludeFilters(names []string) []bosun.Filter {
	if viper.GetBool(ArgAppAll) {
		return bosun.FilterMatchAll()
	}

	out := bosun.FiltersFromNames(names...)

	labels := viper.GetStringSlice(ArgAppLabels)
	if len(labels) > 0 {
		out = append(out, bosun.FiltersFromAppLabels(labels...)...)
	}

	conditions := viper.GetStringSlice(ArgInclude)
	if len(conditions) > 0 {
		out = append(out, bosun.FiltersFromArgs(conditions...)...)
	}

	return out
}

func getExcludeFilters() []bosun.Filter {
	conditions := viper.GetStringSlice(ArgExclude)
	return bosun.FiltersFromArgs(conditions...)
}

func checkExecutableDependency(exe string) {
	path, err := exec.LookPath(exe)
	check(err, "Could not find executable for %q", exe)
	pkg.Log.WithFields(logrus.Fields{"exe": exe, "path": path}).Debug("Found dependency.")
}

func confirm(msg string, args ...string) bool {

	label := fmt.Sprintf(msg, args)

	if pkg.IsInteractive() {
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

func getMarketingRelease() (string, error) {
	var err error
	marketingRelease := viper.GetString(ArgHelmsmanMarketingRelease)
	if marketingRelease == "" {
		marketingRelease, err = pkg.NewCommand("git", "rev-parse", "--abbrev-ref", "HEAD").RunOut()
		if err != nil {
			return "", errors.WithMessage(err, "could not get current branch")
		}
	}

	for !marketingReleaseFormat.MatchString(marketingRelease) {
		marketingRelease = pkg.RequestStringFromUser("%q is not a marketing release. Provide a release number like 2018.2.1", marketingRelease)
	}

	return marketingRelease, nil
}

type globalParameters struct {
	vaultToken string
	vaultAddr  string
	cluster    string
	domain     string
}

func (p *globalParameters) init() error {

	if p.domain == "" {
		p.domain = viper.GetString(ArgGlobalDomain)
	}
	if p.domain == "" {
		return errors.New("domain not set")
	}

	if p.cluster == "" {
		p.cluster = viper.GetString(ArgGlobalCluster)
	}
	if p.cluster == "" {
		return errors.New("cluster not set")
	}

	if p.vaultToken == "" {
		p.vaultToken = viper.GetString(ArgVaultToken)
	}
	if p.vaultToken == "" && p.cluster != "blue" {
		p.vaultToken = "root"
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
		switch p.domain {
		case "n5o.red":
			p.vaultAddr = "http://vault.n5o.red"
		default:
			p.vaultAddr = fmt.Sprintf("https://vault.%s", p.domain)
		}
	}

	return nil
}

func findFileInDirOrAncestors(dir string, filename string) (string, error) {
	startedAt := dir

	for {
		if dir == "" || dir == filepath.Dir(dir) {
			return "", errors.Errorf("file %q not found in %q or any parent", filename, startedAt)
		}
		curr := filepath.Join(dir, filename)

		_, err := os.Stat(curr)
		if err == nil {
			return curr, nil
		}
		dir = filepath.Dir(dir)
	}
}

var (
	colorHeader = color.New(color.Bold)
	colorError  = color.New(color.FgRed)
	colorOK     = color.New(color.FgGreen, color.Bold)
)

func filterApps(apps []*bosun.AppRepo) []*bosun.AppRepo {
	var out []*bosun.AppRepo
	for _, app := range apps {
		if passesConditions(app) {
			out = append(out, app)
		}
	}
	if len(apps) > 0 && len(out) == 0 && len(viper.GetStringSlice(ArgInclude)) > 0 {
		color.Yellow("All apps excluded by conditions.")
		os.Exit(0)
	}
	return out
}

func passesConditions(app *bosun.AppRepo) bool {
	conditions := viper.GetStringSlice(ArgInclude)
	if len(conditions) == 0 {
		return true
	}

	for _, cs := range conditions {

		segs := strings.Split(cs, "=")
		if len(segs) != 2 {
			check(errors.Errorf("invalid condition %q (should be x=y)", cs))
		}
		kind, arg := segs[0], segs[1]

		switch kind {
		case "branch":
			re, err := regexp.Compile(arg)
			check(errors.Wrapf(err, "branch must be regex (was %q)", arg))
			branch := app.GetBranch()
			if !re.MatchString(branch) {
				color.Yellow("Skipping command for app %s because it did not match condition %q", app.Name, cs)
				return false
			}
			return true
		default:
			check(errors.Errorf("invalid condition %q (should be branch=y)", cs))
		}
	}

	return true
}
