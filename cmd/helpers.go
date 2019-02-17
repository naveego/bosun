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

func mustGetBosun() *bosun.Bosun {
	b, err := getBosun()
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

	whitelist := viper.GetStringSlice(ArgReleaseIncludeApps)
	if len(whitelist) > 0 {
		toReleaseSet := map[string]bool{}
		for _, r := range whitelist {
			toReleaseSet[r] = true
		}
		for k, app := range r.AppReleases {
			if !toReleaseSet[k] {
				pkg.Log.Warnf("Skipping %q because it was not listed in the --%s flag.", k, ArgReleaseIncludeApps)
				app.DesiredState.Status = bosun.StatusUnchanged
				app.Excluded = true
			}
		}
	}

	blacklist := viper.GetStringSlice(ArgReleaseExcludeApps)
	for _, name := range blacklist {
		pkg.Log.Warnf("Skipping %q because it was excluded by the --%s flag.", name, ArgReleaseExcludeApps)
		if app, ok := r.AppReleases[name]; ok {
			app.DesiredState.Status = bosun.StatusUnchanged
			app.Excluded = true
		}
	}

	return r
}

func getBosun() (*bosun.Bosun, error) {
	config, err := bosun.LoadWorkspace(viper.GetString(ArgBosunConfigFile))
	if err != nil {
		return nil, err
	}

	params := bosun.Parameters{
		Verbose:  viper.GetBool(ArgGlobalVerbose),
		DryRun:   viper.GetBool(ArgGlobalDryRun),
		NoReport: viper.GetBool(ArgGlobalNoReport),
		Force:    viper.GetBool(ArgGlobalForce),
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


// gets one or more apps matching names, or if names
// are valid file paths, imports the file at that path.
// if names is empty, tries to find a apps starting
// from the current directory
func getAppRepos(b *bosun.Bosun, names []string) ([]*bosun.AppRepo, error) {

	var apps []*bosun.AppRepo
	var err error

	all := b.GetAppsSortedByName()

	if viper.GetBool(ArgAppAll) {
		return all, nil
	}

	labels := viper.GetStringSlice(ArgAppLabels)
	if len(labels) > 0 {
		for _, label := range labels {
			for _, svc := range all {
				for _, svcLabel := range svc.Labels {
					if svcLabel == label {
						apps = append(apps, svc)
						break
					}
				}
			}
		}

		return apps, nil
	}

	var app *bosun.AppRepo
	if len(names) > 0 {
		for _, name := range names {
			maybePath, _ := filepath.Abs(name)
			for _, svc := range all {
				if svc.Name == name || svc.FromPath == maybePath {
					apps = append(apps, svc)
					continue
				}
			}
			if stat, err := os.Stat(maybePath); err == nil && !stat.IsDir() {
				app, err = b.GetOrAddAppForPath(maybePath)
				if err == nil {
					apps = append(apps, app)
				}
			}
		}
		return apps, nil
	}

	var bosunFile string

	wd, _ := os.Getwd()
	bosunFile, err = findFileInDirOrAncestors(wd, "bosun.yaml")
	if err != nil {
		return nil, err
	}

	app, err = b.GetOrAddAppForPath(bosunFile)
	if err != nil {
		return nil, err
	}
	apps = append(apps, app)

	return apps, nil
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
	return handledError{msg:w.String()}
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
