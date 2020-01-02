package util

import (
	"context"
	"github.com/sirupsen/logrus"
)

type Logger interface {
	Log() *logrus.Entry
}

// Ctxer contains a context.Context.
type Ctxer interface {
	Ctx() context.Context
}
