package multierr

import (
	"github.com/pkg/errors"
	"strings"
)

func New(err ...error) *MultiError {
	return &MultiError{
		Errors: err,
	}
}

type MultiError struct {
	Errors []error
}

func (m *MultiError) Collect(err ...error) {
	if err != nil {
		m.Errors = append(m.Errors, err...)
	}
}

func (m MultiError) ToError() error {
	if len(m.Errors) == 0 {
		return nil
	}

	var errStrings []string
	for _, err := range m.Errors {
		errStrings = append(errStrings, err.Error())
	}
	return errors.New(strings.Join(errStrings, "\n"))
}
