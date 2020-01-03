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
