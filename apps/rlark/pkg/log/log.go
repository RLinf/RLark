package log

import (
	"context"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type loggerKey struct{}

var globalLogger logr.Logger

func init() {
	globalLogger = newLogger(os.Getenv("LOG_LEVEL"))
}

func newLogger(levelStr string) logr.Logger {
	level := zapcore.InfoLevel
	switch strings.ToLower(levelStr) {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	}

	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(level)
	config.Encoding = "json"
	config.EncoderConfig.TimeKey = "time"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	zapLogger := zap.Must(config.Build())
	return zapr.NewLogger(zapLogger)
}

// GetLogger returns the logger.
func GetLogger() logr.Logger {
	return globalLogger
}

// FromContext extracts a value from the context.
func FromContext(ctx context.Context) logr.Logger {
	if l, ok := ctx.Value(loggerKey{}).(logr.Logger); ok {
		return l
	}
	return globalLogger
}

// WithLogger returns a modified value with the given logger.
func WithLogger(ctx context.Context, l logr.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// InitLogger initializes the logger.
func InitLogger(level string) {
	if level == "" {
		return
	}
	globalLogger = newLogger(level)
}
