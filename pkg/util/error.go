package util

import "strings"

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
