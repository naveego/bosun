package util

import (
	"github.com/pkg/errors"
	"runtime/debug"
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
