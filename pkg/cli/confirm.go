package cli

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/naveego/bosun/pkg"
	"os"
)

func Confirm(msg string, args ...interface{}) bool {

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
		return false
	}

	return true
}

func ConfirmOrExit(msg string, args ...interface{}) {

	if Confirm(msg, args...) {
		return
	}
	color.Red("User quit.")
	os.Exit(0)
}
