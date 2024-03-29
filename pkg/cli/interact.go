package cli

import (
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/manifoldco/promptui"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh/terminal"
	"os"
	"strings"
)

func RequestConfirmFromUser(label string, args ...interface{}) bool {

	if !IsInteractive() {
		return false
	}

	for {

		prompt := promptui.Prompt{
			Label: fmt.Sprintf(label, args...) + " [y/N]",
		}

		value, err := prompt.Run()
		if err == promptui.ErrInterrupt || err == promptui.ErrAbort {
			fmt.Println("User quit.")
			os.Exit(0)
		}

		if strings.HasPrefix(strings.ToLower(value), "y") {
			return true
		}

		if strings.HasPrefix(value, "?") || strings.HasPrefix(value, "h") {
			e := errors.New("Stack")
			_, _ = fmt.Fprintf(os.Stderr, "%+v\n", e)
			continue
		}

		return false
	}
}

func RequestStringFromUser(text string, args ...interface{}) string {

	if !IsInteractive() {
		panic(fmt.Sprintf("Requested string from user but no terminal is attached: %q, %v", text, args))
	}

	prompt := promptui.Prompt{
		Label: fmt.Sprintf(text, args...),
	}

	value, err := prompt.Run()
	if err == promptui.ErrInterrupt || err == promptui.ErrAbort {
		fmt.Println("User quit.")
		os.Exit(0)
	}

	return value
}

func RequestSecretFromUser(text string, args ...interface{}) string {
	prompt := promptui.Prompt{
		Label: fmt.Sprintf(text, args...),
		Mask:  '*',
	}

	value, err := prompt.Run()
	if err == promptui.ErrInterrupt || err == promptui.ErrAbort {
		fmt.Println("User quit.")
		os.Exit(0)
	}

	return value
}

func IsInteractive() bool {
	return terminal.IsTerminal(int(os.Stdout.Fd()))
}

// RequestChoiceIfEmpty returns defaultValue if it is not empty, otherwise requests a choice from the user.
func RequestChoiceIfEmpty(defaultValue string, message string, options ...string) string {
	if defaultValue != "" {
		return defaultValue
	}
	return RequestChoice(message, options...)
}

func RequestChoice(message string, options ...string) string {

	if !IsInteractive() {
		panic(fmt.Sprintf("Requested choice from user but no terminal is attached: %q, %v", message, options))
	}
	var response string
	prompt := &survey.Select{
		Message: message,
		Options: options,
	}
	err := survey.AskOne(prompt, &response)

	if err != nil {
		fmt.Printf("User quit %s\n", err)
		os.Exit(0)
	}
	return response

}

func RequestBump(message string, currentVersion semver.Version) (semver.Bump, *semver.Version) {

	if !IsInteractive() {
		panic(fmt.Sprintf("Requested bump from user but no terminal is attached: %q, %v", message))
	}

	var bump semver.Bump
	var version *semver.Version
		var tempVersion semver.Version
	var response string

	message = fmt.Sprintf("%s (current version is %s)", message, currentVersion)

	prompt := &survey.Select{
		Message: message,
		Options: []string {
			string(semver.BumpMajor),
			string(semver.BumpMinor),
			string(semver.BumpPatch),
			string(semver.BumpNone),
			"custom",
		},
	}
	err := survey.AskOne(prompt, &response)

	if err != nil {
		fmt.Printf("User quit %s\n", err)
		os.Exit(0)
	}
	switch response {
	case "custom":
		versionPrompt := &survey.Input{
		Message: "Enter the version number you want",
		}
		err = survey.AskOne(versionPrompt, &response)
		if err != nil {
			fmt.Printf("User quit %s\n", err)
			os.Exit(0)
		}
		tempVersion, err = semver.Parse(response)
		if err != nil {
			fmt.Printf("Invalid version %s: %s\n", response, err)
			os.Exit(1)
		}
		version = &tempVersion
		bump = semver.Unknown
	default:
		tempVersion, err = currentVersion.Bump(response)
		if err != nil {
				fmt.Printf("Invalid bump %s: %s\n", response, err)
				os.Exit(1)
		}
		version = &tempVersion
		bump = semver.Bump(response)
	}

	return bump, version
}

func RequestMultiChoiceIfEmpty(defaultValue []string, message string, options []string) []string {
	if len(defaultValue) > 0 {
		return defaultValue
	}
	return RequestMultiChoice(message, options)
}

func RequestMultiChoice(message string, options []string) []string {

	if !IsInteractive() {
		panic(fmt.Sprintf("Requested choices from user but no terminal is attached: %q, %v", message, options))
	}
	var response []string
	prompt := &survey.MultiSelect{
		Message: message,
		Options: options,
	}
	err := survey.AskOne(prompt, &response)

	if err != nil {
		fmt.Printf("User quit %s\n", err)
		os.Exit(0)
	}
	return response

}
