package util

import (
	"github.com/pkg/errors"
	"runtime/debug"
	"strings"
)

type multiError struct {
	errs []error
}

func (m multiError) Error() string {
	w := new(strings.Builder)
	first := true
	for _, err := range m.errs {
		if !first {
			w.WriteString("\n")
		}
		w.WriteString(err.Error())
		first = false
	}
	return w.String()
}

func MultiErr(errs ...error) error {
	var acc []error
	for _, err := range errs {
		if err != nil {
			acc = append(acc, err)
		}
	}

	if len(acc) == 0 {
		return nil
	}

	return multiError{errs: acc}

}

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
