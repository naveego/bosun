package core

import (
	"context"
	"time"
)

type Ctxer interface {
	Ctx() context.Context
	WithTimeout(timeout time.Duration) Ctxer
}
