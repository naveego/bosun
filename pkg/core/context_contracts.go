package core

import (
	"context"
	"time"
)

type Ctxer interface {
	Ctx() context.Context
	WithTimeout(timeout time.Duration) Ctxer
}

type PathResolver interface {
	ResolvePath(path string, expansions ...string) string
}

type WorkspaceContext struct {
	EnvironmentName string
	ClusterName     string
	ReleaseSlot     string
}
