package values

import (
	"github.com/pkg/errors"
)

// ValueMappings is a map of dotted value path to dotted value path.
// When Apply is called, it takes the values found at the key path
// and writes them to the value path.
type ValueMappings map[string]string

func (v ValueMappings) Apply(target Values) error {

	for from, to := range v {
		fromValue, err := target.GetAtPath(from)
		if err != nil {
			return errors.Wrapf(err, "applying mapping from %q to %q", from, to)
		}
		err = target.SetAtPath(to, fromValue)
		if err != nil {
			return errors.Wrapf(err, "applying mapping from %q to %q", from, to)
		}
		commentPath := to + "/comment"
		_ = target.SetAtPath(commentPath, "Mapped from "+from)
	}

	return nil
}
