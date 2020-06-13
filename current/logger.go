package current

import (
	"context"

	"github.com/rs/zerolog"
)

var logger *zerolog.Logger
var disabledLogger *zerolog.Logger

func SetLogger(l *zerolog.Logger) {
	if logger != nil {
		panic("cannot call SetLogger twice")
	}

	logger = l
	disabledLogger = zerolog.Ctx(context.Background())
}

// Logger returns the logger associated with the ctx or the global logger assigned with SetLogger.
//
// zerolog.Ctx provides similar functionality, however this method returns our global logger instead of a disabled
// logger if ctx does not have a logger.
func Logger(ctx context.Context) *zerolog.Logger {
	l := zerolog.Ctx(ctx)
	if l != disabledLogger {
		return l
	}

	if logger == nil {
		panic("missing logger in ctx and logger not set")
	}

	return logger
}

func WithLogger(ctx context.Context, l *zerolog.Logger) context.Context {
	return l.WithContext(ctx)
}
