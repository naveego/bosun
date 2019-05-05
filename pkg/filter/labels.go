package filter

type LabelFunc func() string

func (l LabelFunc) Value() string { return l() }

type LabelString string

func (l LabelString) Value() string { return string(l) }

type LabelValue interface {
	Value() string
}

type Labels map[string]LabelValue

func (l Labels) MarshalYAML() (interface{}, error) {
	if l == nil {
		return nil, nil
	}

	m := make(map[string]string)
	for k, v := range l {
		if s, ok := v.(LabelString); ok {
			m[k] = string(s)
		}
	}
	return m, nil
}

func (l *Labels) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var arr []string
	err := unmarshal(&arr)
	proxy := map[string]string{}
	if err == nil {
		for _, name := range arr {
			proxy[name] = "true"
		}
	} else {
		err = unmarshal(proxy)
	}
	out := Labels{}

	for k, v := range proxy {
		out[k] = LabelString(v)
	}
	*l = out
	return err
}

// Labels implements Labelled.
func (l Labels) GetLabels() Labels {
	return l
}

func LabelsFromMap(m map[string]string) Labels {
	out := Labels{}
	for k, v := range m {
		out[k] = LabelString(v)
	}
	return out
}
