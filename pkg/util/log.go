package util

import (
	"github.com/sirupsen/logrus"
)

type Logger interface {
	Log() *logrus.Entry
}

type WithLogFielder interface {
	Logger
	WithLogField(name string, value interface{}) WithLogFielder
}
