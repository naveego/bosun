package actions

import "strings"

type ActionSchedule string

type ActionSchedules []ActionSchedule

func (f ActionSchedules) MarshalYAML() (interface{}, error) {
	if f == nil {
		return nil, nil
	}
	var out []string
	for _, e := range f {
		out = append(out, string(e))
	}
	return out, nil
}

func (f *ActionSchedules) UnmarshalYAML(unmarshal func(interface{}) error) error {

	var p string
	var out []ActionSchedule
	err := unmarshal(&p)

	if err == nil {
		segs := strings.Split(p, ",")
		for _, s := range segs {
			out = append(out, ActionSchedule(s))
		}

		*f = out
		return nil
	}

	err = unmarshal(&out)
	*f = out

	return err
}

func (f ActionSchedules) Contains(schedule ActionSchedule) bool {
	for _, r := range f {
		if r == schedule {
			return true
		}
	}
	return false
}
