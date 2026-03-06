package logger

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		wantLevel slog.Level
	}{
		{"debug lowercase", "debug", false, slog.LevelDebug},
		{"info lowercase", "info", false, slog.LevelInfo},
		{"warn lowercase", "warn", false, slog.LevelWarn},
		{"warning lowercase", "warning", false, slog.LevelWarn},
		{"error lowercase", "error", false, slog.LevelError},
		{"DEBUG uppercase", "DEBUG", false, slog.LevelDebug},
		{"INFO uppercase", "INFO", false, slog.LevelInfo},
		{"WARN uppercase", "WARN", false, slog.LevelWarn},
		{"ERROR uppercase", "ERROR", false, slog.LevelError},
		{"trace lowercase", "trace", false, slogLevelTrace},
		{"invalid level", "invalid", true, 0},
		{"empty string", "", true, 0},
		{"random string", "random", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLevel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseLevel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantLevel {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.input, got, tt.wantLevel)
			}
		})
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input string
		want  Format
	}{
		{"json", FormatJSON},
		{"JSON", FormatJSON},
		{"text", FormatText},
		{"TEXT", FormatText},
		{"color", FormatColor},
		{"COLOR", FormatColor},
		{"", FormatText},
		{"unknown", FormatText},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseFormat(tt.input)
			if got != tt.want {
				t.Errorf("parseFormat(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestInit(t *testing.T) {
	// Save original logger
	originalLogger := logger

	// Reset after test
	defer func() { logger = originalLogger }()

	t.Run("valid level and format", func(t *testing.T) {
		err := Init("debug", "json")
		if err != nil {
			t.Errorf("Init() error = %v", err)
		}
		if logger == nil {
			t.Error("Init() logger is nil")
		}
	})

	t.Run("invalid level", func(t *testing.T) {
		err := Init("invalid_level", "json")
		if err == nil {
			t.Error("Init() expected error for invalid level")
		}
	})

	t.Run("text format", func(t *testing.T) {
		err := Init("info", "text")
		if err != nil {
			t.Errorf("Init() error = %v", err)
		}
	})

	t.Run("color format", func(t *testing.T) {
		err := Init("info", "color")
		if err != nil {
			t.Errorf("Init() error = %v", err)
		}
	})

	t.Run("uppercase level", func(t *testing.T) {
		err := Init("ERROR", "json")
		if err != nil {
			t.Errorf("Init() error = %v", err)
		}
	})

	t.Run("warning alias", func(t *testing.T) {
		err := Init("warning", "json")
		if err != nil {
			t.Errorf("Init() error = %v", err)
		}
	})
}

func TestGet(t *testing.T) {
	// Save original logger
	originalLogger := logger

	// Reset after test
	defer func() { logger = originalLogger }()

	t.Run("logger not initialized returns default", func(t *testing.T) {
		logger = nil
		got := Get()
		if got == nil {
			t.Error("Get() returned nil")
		}
	})

	t.Run("logger initialized returns logger", func(t *testing.T) {
		_ = Init("debug", "json")
		got := Get()
		if got == nil {
			t.Error("Get() returned nil after Init")
		}
	})
}

func TestLoggingMethods(t *testing.T) {
	// Initialize logger with json format for easier output capture
	_ = Init("debug", "json")

	t.Run("Debug logs without panic", func(t *testing.T) {
		Debug("test debug message", "key", "value")
	})

	t.Run("Info logs without panic", func(t *testing.T) {
		Info("test info message", "key", "value")
	})

	t.Run("Warn logs without panic", func(t *testing.T) {
		Warn("test warn message", "key", "value")
	})

	t.Run("Error logs without panic", func(t *testing.T) {
		Error("test error message", "key", "value")
	})

	t.Run("Trace logs without panic", func(t *testing.T) {
		Trace("test trace message", "key", "value")
	})

	t.Run("DebugContext logs without panic", func(t *testing.T) {
		ctx := context.Background()
		DebugContext(ctx, "test debug context", "key", "value")
	})

	t.Run("InfoContext logs without panic", func(t *testing.T) {
		ctx := context.Background()
		InfoContext(ctx, "test info context", "key", "value")
	})

	t.Run("WarnContext logs without panic", func(t *testing.T) {
		ctx := context.Background()
		WarnContext(ctx, "test warn context", "key", "value")
	})

	t.Run("ErrorContext logs without panic", func(t *testing.T) {
		ctx := context.Background()
		ErrorContext(ctx, "test error context", "key", "value")
	})

	t.Run("TraceContext logs without panic", func(t *testing.T) {
		ctx := context.Background()
		TraceContext(ctx, "test trace context", "key", "value")
	})

	t.Run("multiple args", func(t *testing.T) {
		Info("multiple args", "a", 1, "b", 2, "c", 3)
	})

	t.Run("empty args", func(t *testing.T) {
		Info("no args")
	})
}

// mockWriter implements io.Writer for testing
type mockWriter struct {
	buf strings.Builder
}

func (m *mockWriter) Write(p []byte) (n int, err error) {
	return m.buf.Write(p)
}

func (m *mockWriter) String() string {
	return m.buf.String()
}

func TestGetWriter(t *testing.T) {
	// getWriter returns os.Stderr, just verify it doesn't panic
	// and returns a valid writer
	w := getWriter()
	if w == nil {
		t.Error("getWriter() returned nil")
	}
}

// BenchmarkParseLevel benchmarks parseLevel
func BenchmarkParseLevel(b *testing.B) {
	levels := []string{"debug", "info", "warn", "error", "warning"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, level := range levels {
			parseLevel(level)
		}
	}
}

// BenchmarkParseFormat benchmarks parseFormat
func BenchmarkParseFormat(b *testing.B) {
	formats := []string{"json", "text", "color", "JSON", "TEXT", "COLOR"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, format := range formats {
			parseFormat(format)
		}
	}
}

// Ensure io interface is satisfied
var _ io.Writer = (*mockWriter)(nil)
