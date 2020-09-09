package wfstores

import "github.com/naveego/bosun/pkg/wf/wfcontracts"

type StateStore interface {
	LoadState(name string) (wfcontracts.State, error)
	SaveState(state wfcontracts.State) error
}

type ConfigStore interface {
	LoadConfigs() ([]wfcontracts.Config, error)
	SaveConfigs(configs []wfcontracts.Config) error
}

