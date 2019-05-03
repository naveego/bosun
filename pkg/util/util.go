package util

import (
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"strings"
	"time"
)

type RetriableError struct {
	Err error
}

func (r RetriableError) Error() string { return "Temporary Error: " + r.Err.Error() }

type MultiError struct {
	Errors []error
}

func (m *MultiError) Collect(err error) {
	if err != nil {
		m.Errors = append(m.Errors, err)
	}
}

func (m MultiError) ToError() error {
	if len(m.Errors) == 0 {
		return nil
	}

	errStrings := []string{}
	for _, err := range m.Errors {
		errStrings = append(errStrings, err.Error())
	}
	return errors.New(strings.Join(errStrings, "\n"))
}

func Retry(attempts int, callback func() error) (err error) {
	return RetryAfter(attempts, callback, 0)
}

func RetryAfter(attempts int, callback func() error, d time.Duration) (err error) {
	m := MultiError{}
	for i := 0; i < attempts; i++ {
		if i > 0 {
			glog.V(1).Infof("retry loop %d", i)
		}
		err = callback()
		if err == nil {
			return nil
		}
		m.Collect(err)
		if _, ok := err.(*RetriableError); !ok {
			glog.Infof("non-retriable error: %v", err)
			return m.ToError()
		}
		glog.V(2).Infof("sleeping %s", d)
		time.Sleep(d)
	}
	return m.ToError()
}

func DistinctStrings(strs []string) []string {
	var out []string
	m := map[string]struct{}{}
	for _, s := range strs {
		m[s] = struct{}{}
	}
	for k := range m {
		out = append(out, k)
	}
	return out
}
