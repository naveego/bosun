package pkg

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/sirupsen/logrus"
	"os"
	"runtime"
)

var Log *logrus.Entry

func RequestStringFromUser(text string, args... interface{}) (string) {
	prompt := promptui.Prompt{
		Label:fmt.Sprintf(text, args...),
	}

	value, err := prompt.Run()
	if err == promptui.ErrInterrupt || err == promptui.ErrAbort {
		fmt.Println("User quit.")
		os.Exit(0)
	}

	return value
}

func RequestSecretFromUser(text string, args... interface{}) (string) {
	prompt := promptui.Prompt{
		Label:fmt.Sprintf(text, args...),
		Mask:'*',
	}

	value, err := prompt.Run()
	if err == promptui.ErrInterrupt || err == promptui.ErrAbort {
		fmt.Println("User quit.")
		os.Exit(0)
	}

	return value
}

func Must(err error, msgAndArgs... string) {
	if err == nil {
		return
	}
	var msg string
	switch len(msgAndArgs){
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