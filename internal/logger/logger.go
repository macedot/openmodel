package logger

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

var (
	logger *slog.Logger
	mu     sync.RWMutex
)

type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

type Format string

const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, errors.New("invalid log level")
	}
}

func parseFormat(s string) Format {
	switch strings.ToLower(s) {
	case "json":
		return FormatJSON
	default:
		return FormatText
	}
}

func getWriter() io.Writer {
	return os.Stderr
}

func Init(level string, format string) error {
	mu.Lock()
	defer mu.Unlock()

	lvl, err := parseLevel(level)
	if err != nil {
		return err
	}

	logFormat := parseFormat(format)

	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: lvl,
	}

	switch logFormat {
	case FormatJSON:
		handler = slog.NewJSONHandler(getWriter(), opts)
	default:
		handler = slog.NewTextHandler(getWriter(), opts)
	}

	logger = slog.New(handler)
	slog.SetDefault(logger)

	return nil
}

func Get() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	if logger == nil {
		return slog.Default()
	}
	return logger
}

func Debug(msg string, args ...any) {
	Get().Debug(msg, args...)
}

func DebugContext(ctx context.Context, msg string, args ...any) {
	Get().DebugContext(ctx, msg, args...)
}

func Info(msg string, args ...any) {
	Get().Info(msg, args...)
}

func InfoContext(ctx context.Context, msg string, args ...any) {
	Get().InfoContext(ctx, msg, args...)
}

func Warn(msg string, args ...any) {
	Get().Warn(msg, args...)
}

func WarnContext(ctx context.Context, msg string, args ...any) {
	Get().WarnContext(ctx, msg, args...)
}

func Error(msg string, args ...any) {
	Get().Error(msg, args...)
}

func ErrorContext(ctx context.Context, msg string, args ...any) {
	Get().ErrorContext(ctx, msg, args...)
}
