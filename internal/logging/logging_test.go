package logging

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestNew_LevelParsing(t *testing.T) {
	tests := []struct {
		input string
		level slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},       // default
		{"garbage", slog.LevelInfo}, // default
	}
	for _, tt := range tests {
		logger := New(tt.input, "test")
		if logger == nil {
			t.Fatalf("New(%q) returned nil", tt.input)
		}
		// Verify the logger is enabled at the expected level
		if !logger.Handler().Enabled(context.Background(), tt.level) {
			t.Errorf("New(%q): expected enabled at %v", tt.input, tt.level)
		}
	}
}

func TestWithContext_RoundTrip(t *testing.T) {
	logger := New("info", "test")
	ctx := WithContext(context.Background(), logger)
	got := FromContext(ctx)

	if got != logger {
		t.Error("FromContext did not return the same logger stored by WithContext")
	}
}

func TestFromContext_Default(t *testing.T) {
	got := FromContext(context.Background())
	if got == nil {
		t.Error("FromContext on plain context returned nil")
	}
}

func TestWithJob(t *testing.T) {
	logger := New("info", "test")
	jobLogger := WithJob(logger, "job-123", "My Download")
	if jobLogger == nil {
		t.Error("WithJob returned nil")
	}
}

func TestWithTraceID(t *testing.T) {
	logger := New("info", "test")
	traced := WithTraceID(logger, "trace-abc")
	if traced == nil {
		t.Error("WithTraceID returned nil")
	}
}

func TestShortenPath(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"/home/user/go/src/github.com/pkg/file.go", "pkg/file.go"},
		{"file.go", "file.go"},
		{"pkg/file.go", "pkg/file.go"},
		{"", ""},
		{"a/b/c/d.go", "c/d.go"},
	}
	for _, tt := range tests {
		got := shortenPath(tt.input)
		if got != tt.want {
			t.Errorf("shortenPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLogFunctions_DoNotPanic(t *testing.T) {
	logger := New("debug", "test")

	LogDownloadStart(logger, "job-1", 100, 50000)
	LogDownloadProgress(logger, "job-1", 50, 100, 1024*1024)
	LogDownloadComplete(logger, "job-1", 5*time.Second, 50000)
	LogSegmentError(logger, "job-1", "msg@id", 2, errors.New("timeout"))
	LogPostProcess(logger, "job-1", "par2", "running")
}
