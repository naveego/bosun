package multierr

import (
	"github.com/pkg/errors"
	"strings"
	"sync"
)

func New(err ...error) Collector {
	return &multiError{
		mu:     sync.Mutex{},
		Errors: err,
	}
}

type Collector interface {
	Collect(err ...error)
	ToError() error
}

type multiError struct {
	mu     sync.Mutex
	Errors []error
}

func (m *multiError) Collect(err ...error) {
	m.mu.Lock()
	if err != nil {
		m.Errors = append(m.Errors, err...)
	}
	m.mu.Unlock()
}

func (m *multiError) ToError() error {
	if len(m.Errors) == 0 {
		return nil
	}

	var errStrings []string
	for _, err := range m.Errors {
		errStrings = append(errStrings, err.Error())
	}
	return errors.New(strings.Join(errStrings, "\n"))
}
