package util

import (
	"github.com/golang/glog"
	"github.com/naveego/bosun/pkg/util/multierr"
	"time"
)

type RetriableError struct {
	Err error
}

func (r RetriableError) Error() string { return "Temporary Error: " + r.Err.Error() }

func Retry(attempts int, callback func() error) (err error) {
	return RetryAfter(attempts, callback, 0)
}

func RetryAfter(attempts int, callback func() error, d time.Duration) (err error) {
	m := multierr.New()
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

// DefaultString returns the first string that is not empty
func DefaultString(strs ...string) string {
	for _, s := range strs {
		if s != "" {
			return s
		}
	}
	return ""
}
