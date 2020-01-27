package cli

import (
	"fmt"
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
