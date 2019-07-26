package bosun

import (
	"github.com/pkg/errors"
)

type errAppNotFound string

func (e errAppNotFound) Error() string { return string(e) }

func IsErrAppNotFound(err error) bool {
	_, ok := err.(errAppNotFound)
	return ok
}

func ErrAppNotFound(name string) error {
	return errAppNotFound(errors.Errorf("app %q not found", name).Error())
}
