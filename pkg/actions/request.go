package actions

import (
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/ioc"
)

type ActionContext interface {
	core.PathResolver
	core.StringKeyValuer
	core.InterfaceKeyValuer
	ioc.Provider

	command.ExecutionContext
}
