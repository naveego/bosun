package cli

import (
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/manifoldco/promptui"
	"golang.org/x/crypto/ssh/terminal"
	"os"
	"strings"
)

func RequestConfirmFromUser(label string, args ...interface{}) bool {

	if !IsInteractive() {
		return false
	}

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

	return false
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
