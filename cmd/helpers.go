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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
)

func getBosun() (*bosun.Bosun, error) {
	config, state, err := bosun.LoadConfig(viper.GetString(ArgBosunConfigFile))
	if err != nil {
		return nil, err
	}

	params := bosun.Parameters{
		Verbose: viper.GetBool(ArgGlobalVerbose),
		DryRun:  viper.GetBool(ArgGlobalDryRun),
	}

	return bosun.New(params, config, state), nil
}

// gets one or more microservices matching names.
// if names is empty, tries to find a microservice starting
// from the current directory
func getMicroservices(b *bosun.Bosun, names []string) ([]*bosun.App, error) {

	var services []*bosun.App
	var err error

	all := b.GetMicroservices()

	if viper.GetBool(ArgSvcAll) {
		return all, nil
	}

	labels := viper.GetStringSlice(ArgSvcLabels)
	if len(labels) > 0 {
		for _, label := range labels {
			for _, svc := range services {
				for _, svcLabel := range svc.Config.Labels {
					if svcLabel == label {
						services = append(services, svc)
						goto nextLabel
					}
				}
			}
		nextLabel:
		}

		return services, nil
	}

	var ms *bosun.App
	if len(names) > 0 {
		for _, svc := range all {
			for _, name := range names {
				if svc.Config.Name == name {
					services = append(services, svc)
					goto nextName
				}
			}
		nextName:
		}
		return services, nil
	}

	wd, _ := os.Getwd()
	bosunFile, err := findFileInDirOrAncestors(wd, "bosun.yaml")
	if err != nil {
		return nil, err
	}

	ms, err = b.GetOrAddMicroserviceForPath(bosunFile)
	if err != nil {
		return nil, err
	}
	services = append(services, ms)

	return services, nil
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
