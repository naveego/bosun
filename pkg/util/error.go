package util

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
)

func TryCatch(label string, fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			err, ok = r.(error)
			if ok {
				err = errors.Errorf("%s: panicked with error: %s\n%s", label, err, debug.Stack())
			} else {
				err = errors.Errorf("%s: panicked: %v\n%s", label, r, debug.Stack())
			}
		}
	}()

	return fn()
}

func CheckHandleMsg(msg string, err error) error {
	return CheckHandle(err, msg)
}

func CheckHandle(err error, msgAndArgs ...string) error {
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

type handledError struct {
	msg string
}

func (h handledError) Error() string {
	return h.msg
}

func Check(err error, msgAndArgs ...string) {
	if CheckHandle(err, msgAndArgs...) != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func checkMsg(msg string, err error) {
	Check(err, msg)
}
