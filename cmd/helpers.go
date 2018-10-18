package cmd

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/naveegoinc/devops/bosun/internal"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh/terminal"
	"os"
	"os/exec"
	"regexp"
	"runtime"
)

func checkExecutableDependency(exe string) {
	path, err := exec.LookPath(exe)
	check(err, "Could not find executable for %q", exe)
	internal.Log.WithFields(logrus.Fields{"exe": exe, "path": path}).Debug("Found dependency.")
}

func confirm(msg string, args ... string) bool {

	label := fmt.Sprintf(msg, args)

	if !terminal.IsTerminal(int(os.Stdout.Fd())) {
		internal.Log.WithField("label", label).Warn("No terminal attached, skipping confirmation.")
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
		marketingRelease, err = internal.NewCommand("git", "rev-parse", "--abbrev-ref", "HEAD").RunOut()
		if err != nil {
			return "", errors.WithMessage(err, "could not get current branch")
		}
	}

	for !marketingReleaseFormat.MatchString(marketingRelease) {
		marketingRelease = internal.RequestStringFromUser("%q is not a marketing release. Provide a release number like 2018.2.1", marketingRelease)
	}

	return marketingRelease, nil
}
