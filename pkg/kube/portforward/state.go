package portforward

import "fmt"

type DaemonState struct {
	OK    bool                        `yaml:"ok"`
	Error string                      `yaml:"error,omitempty"`
	Ports map[string]PortForwardState `yaml:"ports"`
}

func (d DaemonState) Headers() []string {
	return []string{"Name", "Active", "State", "Config", "Error"}
}

func (d DaemonState) Rows() [][]string {

	var out [][]string
	for name, state := range d.Ports{
		out = append(out, []string{name, fmt.Sprint(state.Config.Active), state.State, state.Config.String(), state.Error})
	}

	return out
}

type PortForwardState struct {
	State  string            `yaml:"state,omitempty"`
	Error  string            `yaml:"error,omitempty"`
	Config *PortForwardConfig `yaml:"config"`
	PID    int               `yaml:"pid,omitempty"`
}