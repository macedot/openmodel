// Package logger provides structured logging for openmodel.
// It wraps slog with a simple API and supports text, colored text, and JSON formats.
package logger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"golang.org/x/term"
)

var (
	logger *slog.Logger
	mu     sync.RWMutex
	level  slog.Level
)

// Level represents the logging level.
type Level string

const (
	LevelTrace Level = "trace"
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// Format represents the output format for logs.
type Format string

const (
	FormatJSON  Format = "json"
	FormatText  Format = "text"
	FormatColor Format = "color"
)

// Custom slog level for trace (lower than debug)
const slogLevelTrace = slog.Level(-8) // slog.LevelDebug is -4

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorGreen  = "\033[32m"
	colorGray   = "\033[90m"
)

// getColorForLevel returns ANSI color code for a log level
func getColorForLevel(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return colorRed
	case level >= slog.LevelWarn:
		return colorYellow
	case level >= slog.LevelInfo:
		return colorBlue
	case level >= slog.LevelDebug:
		return colorGreen
	default:
		return colorGray
	}
}

// isTerminal returns true if stdout is a terminal
func isTerminal() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// coloredTextHandler is a custom handler that outputs colored text logs
type coloredTextHandler struct {
	opts *slog.HandlerOptions
	out  io.Writer
}

func (h *coloredTextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if h.opts.Level != nil {
		return level >= h.opts.Level.Level()
	}
	return true
}

func (h *coloredTextHandler) Handle(ctx context.Context, r slog.Record) error {
	state := make([]string, 0, 4)

	// Time
	state = append(state, r.Time.Format("2006-01-02T15:04:05.000-07:00"))

	// Level (colored)
	levelColor := getColorForLevel(r.Level)
	state = append(state, fmt.Sprintf("%s%-5s%s", levelColor, r.Level.String(), colorReset))

	// Message
	state = append(state, r.Message)

	// Attributes
	r.Attrs(func(a slog.Attr) bool {
		state = append(state, fmt.Sprintf("%s=%s", a.Key, a.Value.String()))
		return true
	})

	// Write output
	_, err := fmt.Fprintf(h.out, "%s\n", strings.Join(state, " "))
	return err
}

func (h *coloredTextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *coloredTextHandler) WithGroup(name string) slog.Handler {
	return h
}

// newColoredTextHandler creates a new colored text handler
func newColoredTextHandler(out io.Writer, opts *slog.HandlerOptions) slog.Handler {
	return &coloredTextHandler{
		opts: opts,
		out:  out,
	}
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "trace":
		return slogLevelTrace, nil
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
	case "color":
		return FormatColor
	default:
		return FormatText
	}
}

func getWriter() io.Writer {
	return os.Stderr
}

// Init initializes the logger with the given level and format.
// Level can be: trace, debug, info, warn, error
// Format can be: text, color, json
func Init(levelStr string, format string) error {
	mu.Lock()
	defer mu.Unlock()

	lvl, err := parseLevel(levelStr)
	if err != nil {
		return err
	}

	level = lvl

	logFormat := parseFormat(format)

	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: lvl,
	}

	switch logFormat {
	case FormatJSON:
		handler = slog.NewJSONHandler(getWriter(), opts)
	case FormatColor:
		handler = newColoredTextHandler(getWriter(), opts)
	default:
		handler = slog.NewTextHandler(getWriter(), opts)
	}

	logger = slog.New(handler)
	slog.SetDefault(logger)

	return nil
}

// Get returns the current logger instance.
func Get() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	if logger == nil {
		return slog.Default()
	}
	return logger
}

// IsTraceEnabled returns true if trace level logging is enabled.
func IsTraceEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return level <= slogLevelTrace
}

// Trace logs a message at trace level.
func Trace(msg string, args ...any) {
	Get().Log(context.Background(), slogLevelTrace, msg, args...)
}

// TraceContext logs a message at trace level with context.
func TraceContext(ctx context.Context, msg string, args ...any) {
	Get().Log(ctx, slogLevelTrace, msg, args...)
}

// Debug logs a message at debug level.
func Debug(msg string, args ...any) {
	Get().Debug(msg, args...)
}

// DebugContext logs a message at debug level with context.
func DebugContext(ctx context.Context, msg string, args ...any) {
	Get().DebugContext(ctx, msg, args...)
}

// Info logs a message at info level.
func Info(msg string, args ...any) {
	Get().Info(msg, args...)
}

// InfoContext logs a message at info level with context.
func InfoContext(ctx context.Context, msg string, args ...any) {
	Get().InfoContext(ctx, msg, args...)
}

// Warn logs a message at warn level.
func Warn(msg string, args ...any) {
	Get().Warn(msg, args...)
}

// WarnContext logs a message at warn level with context.
func WarnContext(ctx context.Context, msg string, args ...any) {
	Get().WarnContext(ctx, msg, args...)
}

// Error logs a message at error level.
func Error(msg string, args ...any) {
	Get().Error(msg, args...)
}

// ErrorContext logs a message at error level with context.
func ErrorContext(ctx context.Context, msg string, args ...any) {
	Get().ErrorContext(ctx, msg, args...)
}

// TraceFile writes a trace file with the given name suffix.
// The file is named trace-<suffix>.json and contains JSON-encoded data.
// This is used for debugging and is only created when trace level is enabled.
func TraceFile(suffix string, data any) error {
	if !IsTraceEnabled() {
		return nil
	}

	filename := fmt.Sprintf("trace-%s.json", suffix)
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal trace data: %w", err)
	}

	if err := os.WriteFile(filename, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write trace file: %w", err)
	}

	Trace("trace_file_written", "filename", filename)
	return nil
}
