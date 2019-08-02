package bosun

import "github.com/sirupsen/logrus"

// Services is version 2 of the BosunContext.
// It's intended to implement contracts defined in contracts.go,
// to make it easier for various components to declare their actual
// dependencies rather than taking a dependency on the entire
// concrete BosunContext.
type Services struct {
	ctx BosunContext
}

func (c BosunContext) Services() Services {
	return Services{ctx: c}
}

func (s Services) Log() *logrus.Entry {
	return s.ctx.GetLog()
}

type Logger interface {
	Log() *logrus.Entry
}
