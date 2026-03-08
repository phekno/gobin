package logging

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"
)

// contextKey is unexported to prevent collisions.
type contextKey struct{}

var loggerKey = contextKey{}

// Fields is a convenience alias for structured log attributes.
type Fields map[string]any

// New creates a production-ready structured logger.
// Output is always JSON for machine parseability (Loki, Fluentd, ELK, etc.)
func New(level string, component string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level:     lvl,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Normalize field names for cloud-native log aggregators
			switch a.Key {
			case slog.TimeKey:
				// RFC3339Nano for precise correlation
				a.Value = slog.StringValue(a.Value.Time().UTC().Format(time.RFC3339Nano))
			case slog.LevelKey:
				// Lowercase levels (info, warn, error) for consistency
				a.Value = slog.StringValue(strings.ToLower(a.Value.String()))
			case slog.SourceKey:
				// Shorten source paths
				if src, ok := a.Value.Any().(*slog.Source); ok {
					src.File = shortenPath(src.File)
				}
			}
			return a
		},
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)

	// Always include component name and hostname for k8s log correlation
	hostname, _ := os.Hostname()
	logger := slog.New(handler).With(
		slog.String("component", component),
		slog.String("hostname", hostname),
		slog.String("go_version", runtime.Version()),
	)

	return logger
}

// WithContext embeds a logger into a context for propagation.
func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext retrieves the logger from context, or returns the default.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

// WithJob returns a logger enriched with job-specific fields.
// Use this when processing a specific download job.
func WithJob(logger *slog.Logger, jobID, jobName string) *slog.Logger {
	return logger.With(
		slog.String("job_id", jobID),
		slog.String("job_name", jobName),
	)
}

// WithTraceID returns a logger enriched with a trace ID for request correlation.
// Integrates with OpenTelemetry trace context if available.
func WithTraceID(logger *slog.Logger, traceID string) *slog.Logger {
	return logger.With(slog.String("trace_id", traceID))
}

// shortenPath trims the file path to just package/file.go
func shortenPath(path string) string {
	// Find last two path segments
	count := 0
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			count++
			if count == 2 {
				return path[i+1:]
			}
		}
	}
	return path
}

// --- Convenience wrappers for common log patterns ---

// LogDownloadStart emits a structured log for download initiation.
func LogDownloadStart(logger *slog.Logger, jobID string, totalSegments int, totalBytes int64) {
	logger.Info("download started",
		slog.String("job_id", jobID),
		slog.Int("total_segments", totalSegments),
		slog.Int64("total_bytes", totalBytes),
		slog.String("event", "download.start"),
	)
}

// LogDownloadProgress emits periodic progress updates.
func LogDownloadProgress(logger *slog.Logger, jobID string, done, total int, bytesPerSec float64) {
	logger.Info("download progress",
		slog.String("job_id", jobID),
		slog.Int("segments_done", done),
		slog.Int("segments_total", total),
		slog.Float64("progress_pct", float64(done)/float64(total)*100),
		slog.Float64("speed_mbps", bytesPerSec/1024/1024),
		slog.String("event", "download.progress"),
	)
}

// LogDownloadComplete emits a structured log for successful completion.
func LogDownloadComplete(logger *slog.Logger, jobID string, duration time.Duration, totalBytes int64) {
	logger.Info("download complete",
		slog.String("job_id", jobID),
		slog.Duration("duration", duration),
		slog.Int64("total_bytes", totalBytes),
		slog.Float64("avg_speed_mbps", float64(totalBytes)/duration.Seconds()/1024/1024),
		slog.String("event", "download.complete"),
	)
}

// LogSegmentError emits a structured log for a failed segment fetch.
func LogSegmentError(logger *slog.Logger, jobID, messageID string, attempt int, err error) {
	logger.Warn("segment fetch failed",
		slog.String("job_id", jobID),
		slog.String("message_id", messageID),
		slog.Int("attempt", attempt),
		slog.String("error", err.Error()),
		slog.String("event", "segment.error"),
	)
}

// LogPostProcess emits structured logs for post-processing stages.
func LogPostProcess(logger *slog.Logger, jobID, stage, status string) {
	logger.Info("post-processing",
		slog.String("job_id", jobID),
		slog.String("stage", stage),
		slog.String("status", status),
		slog.String("event", "postprocess."+stage),
	)
}
