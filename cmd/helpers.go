package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/filter"
	"github.com/naveego/bosun/pkg/util"
	"github.com/olekukonko/tablewriter"
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

func mustGetBosun(optionalParams ...bosun.Parameters) *bosun.Bosun {
	b, err := getBosun(optionalParams...)
	if err != nil {
		log.Fatal(err)
	}

	envFromEnv := os.Getenv(bosun.EnvEnvironment)
	envFromConfig := b.GetCurrentEnvironment().Name
	if envFromConfig != envFromEnv {
		_, _ = colorError.Printf("Bosun config indicates environment should be %[1]q, but the environment var %[2]s is %[3]q. You may want to run $(bosun env %[1]s)",
			envFromConfig,
			bosun.EnvEnvironment,
			envFromEnv)
	}

	return b
}

func mustGetCurrentRelease(b *bosun.Bosun) *bosun.ReleaseManifest {
	r, err := b.GetCurrentReleaseManifest(true)
	if err != nil {
		log.Fatal(err)
	}

	return r

	// r, err := b.GetCurrentRelease()
	// if err != nil {
	// 	log.Fatal(err)
	// }
	//
	// whitelist := viper.GetStringSlice(ArgFilteringInclude)
	// if len(whitelist) > 0 {
	// 	toReleaseSet := map[string]bool{}
	// 	for _, r := range whitelist {
	// 		toReleaseSet[r] = true
	// 	}
	// 	for k, app := range r.AppReleases {
	// 		if !toReleaseSet[k] {
	// 			pkg.Log.Warnf("Skipping %q because it was not listed in the --%s flag.", k, ArgFilteringInclude)
	// 			app.DesiredState.Status = bosun.StatusUnchanged
	// 			app.Excluded = true
	// 		}
	// 	}
	// }
	//
	// blacklist := viper.GetStringSlice(ArgFilteringExclude)
	// for _, name := range blacklist {
	// 	pkg.Log.Warnf("Skipping %q because it was excluded by the --%s flag.", name, ArgFilteringExclude)
	// 	if app, ok := r.AppReleases[name]; ok {
	// 		app.DesiredState.Status = bosun.StatusUnchanged
	// 		app.Excluded = true
	// 	}
	// }
	//
	// return r
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
	params.ConfirmedEnv = viper.GetString(ArgGlobalConfirmedEnv)

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

func mustGetApp(b *bosun.Bosun, names []string) *bosun.App {
	f := getFilterParams(b, names).IncludeCurrent()
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
		ctx.Log.Debug("Creating transient release...")
		valueSetNames := util.ConcatStrings(ctx.Env.ValueSets, viper.GetStringSlice(ArgAppValueSet))
		valueSets, err := b.GetValueSetSlice(valueSetNames)
		if err != nil {
			return nil, err
		}

		includeDeps := viper.GetBool(ArgAppDeployDeps)
		deploySettings := bosun.DeploySettings{
			Environment:        ctx.Env,
			ValueSets:          valueSets,
			UseLocalContent:    true,
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

func (f FilterParams) Chain() filter.Chain {
	var include []filter.Filter
	var exclude []filter.Filter

	if viper.GetBool(ArgFilteringAll) {
		include = append(include, filter.FilterMatchAll())
	} else if len(f.Names) > 0 {
		for _, name := range f.Names {
			include = append(include, filter.MustParse(bosun.LabelName, "==", name))
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
	apps := f.b.GetAppsSortedByName()

	result, err := f.Chain().ToGetExactly(1).FromErr(apps)
	if err != nil {
		return nil, err
	}

	return result.([]*bosun.App)[0], nil
}

func (f FilterParams) GetApps() []*bosun.App {
	apps := f.b.GetAppsSortedByName()

	result := f.Chain().From(apps)

	return result.([]*bosun.App)
}

func (f FilterParams) GetAppsChain(chain filter.Chain) ([]*bosun.App, error) {
	apps := f.b.GetAppsSortedByName()

	result, err := chain.FromErr(apps)

	return result.([]*bosun.App), err
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
	var app *bosun.App

	wd, _ := os.Getwd()
	bosunFile, err = findFileInDirOrAncestors(wd, "bosun.yaml")
	if err != nil {
		return nil, err
	}

	_, err = b.GetOrAddAppForPath(bosunFile)
	if err != nil {
		return nil, err
	}

	bosunFileDir := filepath.Dir(bosunFile)

	var appsUnderDirNames []string
	var appsUnderDir []*bosun.App
	for _, app = range b.GetApps() {
		if app.IsRepoCloned() && strings.HasPrefix(app.FromPath, bosunFileDir) {
			appsUnderDirNames = append(appsUnderDirNames, app.Name)
			appsUnderDir = append(appsUnderDir, app)
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
	colorHeader = color.New(color.Bold, color.FgHiBlue)
	colorError  = color.New(color.FgRed)
	colorOK     = color.New(color.FgGreen, color.Bold)
)

func printOutput(out interface{}, columns ...string) error {

	format := viper.GetString(ArgGlobalOutput)

	if format == "" {
		format = "y"
	}
	formatKey := strings.ToLower(format[0:1])

	switch formatKey {
	case "j":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	case "y":
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(out)
	case "t":

		var header []string
		var rows [][]string

		switch t := out.(type) {
		case util.Tabler:
			header = t.Headers()
			rows = t.Rows()
		default:
			segs := strings.Split(format, "=")
			if len(segs) > 1 {
				columns = strings.Split(segs[1], ",")
			}
			j, err := json.Marshal(out)
			if err != nil {
				return err
			}
			var mapSlice []map[string]json.RawMessage
			err = json.Unmarshal(j, &mapSlice)
			if err != nil {
				return errors.Wrapf(err, "only slices of structs or maps can be rendered as a table, but got %T", out)
			}
			if len(mapSlice) == 0 {
				return nil
			}

			first := mapSlice[0]

			var keys []string
			if len(columns) > 0 {
				keys = columns
			} else {
				for k := range first {
					keys = append(keys, k)
				}
				sort.Strings(keys)
			}
			for _, k := range keys {
				header = append(header, k)
			}
			for _, m := range mapSlice {
				var values []string

				for _, k := range keys {
					if v, ok := m[k]; ok && len(v) > 0 {
						values = append(values, strings.Trim(string(v), `"`))
					} else {
						values = append(values, "")
					}
				}
				rows = append(rows, values)
			}
		}

		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader(header)
		table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
		table.SetCenterSeparator("|")

		for _, row := range rows {
			table.Append(row)
		}

		table.Render()

		return nil
	default:
		return errors.Errorf("Unrecognized format %q (valid formats are 'json', 'yaml', and 'table')", format)
	}

}

func getResolvedValuesFromApp(b *bosun.Bosun, app *bosun.App) (*bosun.PersistableValues, error) {
	ctx := b.NewContext().WithDir(app.FromPath)

	appManifest, err := app.GetManifest(ctx)
	if err != nil {
		return nil, err
	}
	return getResolvedValuesFromAppManifest(b, appManifest)
}

func getResolvedValuesFromAppManifest(b *bosun.Bosun, appManifest *bosun.AppManifest) (*bosun.PersistableValues, error) {

	ctx := b.NewContext()

	appDeploy, err := bosun.NewAppDeploy(ctx, bosun.DeploySettings{}, appManifest)
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
func getValueSetSlice(b *bosun.Bosun, env *bosun.EnvironmentConfig) ([]bosun.ValueSet, error) {
	valueSetNames := util.ConcatStrings(env.ValueSets, viper.GetStringSlice(ArgAppValueSet))
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
		overrideValueSet := bosun.ValueSet{
			Dynamic: map[string]*bosun.CommandValue{},
		}
		for k, v := range valueOverrides {
			overrideValueSet.Dynamic[k] = &bosun.CommandValue{Value: v}
		}
		valueSets = append(valueSets, overrideValueSet)
	}

	return valueSets, err
}
