package cmd

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/bosun"
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
)

func mustGetBosun() *bosun.Bosun {
	b, err := getBosun()
	if err != nil {
		log.Fatal(err)
	}
	return b
}

func mustGetCurrentRelease(b *bosun.Bosun) *bosun.Release {
	r, err := b.GetCurrentRelease()
	if err != nil {
		log.Fatal(err)
	}
	return r
}

func getBosun() (*bosun.Bosun, error) {
	config, err := bosun.LoadConfig(viper.GetString(ArgBosunConfigFile))
	if err != nil {
		return nil, err
	}

	params := bosun.Parameters{
		Verbose: viper.GetBool(ArgGlobalVerbose),
		DryRun:  viper.GetBool(ArgGlobalDryRun),
		CIMode: viper.GetBool(ArgGlobalCIMode),
		Force: viper.GetBool(ArgGlobalForce),
	}

	return bosun.New(params, config)
}

func mustGetApp(b *bosun.Bosun, names []string) *bosun.App {
	apps, err := getApps(b, names)
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

func mustYaml(i interface{}) string {
	b, err := yaml.Marshal(i)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// gets one or more apps matching names, or if names
// are valid file paths, imports the file at that path.
// if names is empty, tries to find a apps starting
// from the current directory
func getApps(b *bosun.Bosun, names []string) ([]*bosun.App, error) {

	var apps []*bosun.App
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

	var app *bosun.App
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
				if err ==  nil {
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

func confirm(msg string, args ... string) bool {

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

func check(err error, msgAndArgs ... string) {
	if err == nil {
		return
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

	color.Red(msg)
	color.Yellow(err.Error())

	_, file, line, ok := runtime.Caller(1)
	if ok {
		color.Blue("@ %s : line %d", file, line)
	}

	os.Exit(1)
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
