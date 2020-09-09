package wf

import (
	"github.com/naveego/bosun/pkg/ioc"
	"github.com/naveego/bosun/pkg/wf/wfregistry"
	"github.com/naveego/bosun/pkg/wf/wfstores"
	"github.com/sirupsen/logrus"
)



type Engine struct {
	Registry    *wfregistry.Registry
	Provider    ioc.Provider
	Log         *logrus.Entry
	ConfigStore wfstores.ConfigStore
	StateStore wfstores.StateStore
}

func (e *Engine) Run(name string) error {

}
