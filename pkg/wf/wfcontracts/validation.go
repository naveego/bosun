package wfcontracts

import "github.com/pkg/errors"

func ValidateConfig(config Config) error {
	if config.Name == "" {
		return errors.New("config.Name was unset")
	}
	if config.Type == "" {
		return errors.New("config.Type was unset")
	}
	if config.Values == nil {
		return errors.New("config.Values was unset")
	}
	return nil
}


func ValidateState(state State) error {
	if state.Name == "" {
		return errors.New("state.Name was unset")
	}
	if state.Current == "" {
		return errors.New("state.Current was unset")
	}
	if state.Values == nil {
		return errors.New("state.Values was unset")
	}
	return nil
}