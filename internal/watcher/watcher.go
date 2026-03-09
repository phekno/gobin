// Package watcher monitors a directory for NZB files and auto-adds them to the queue.
package watcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/phekno/gobin/internal/nzb"
	"github.com/phekno/gobin/internal/queue"
)

// Watcher polls a directory for new .nzb files.
type Watcher struct {
	dir      string
	queue    *queue.Manager
	interval time.Duration
	idGen    func() string
}

// New creates a directory watcher.
func New(dir string, q *queue.Manager, interval time.Duration, idGen func() string) *Watcher {
	return &Watcher{dir: dir, queue: q, interval: interval, idGen: idGen}
}

// Run starts watching. Blocks until context is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	if w.dir == "" {
		slog.Info("watch directory not configured, skipping")
		return
	}

	if err := os.MkdirAll(w.dir, 0755); err != nil {
		slog.Error("failed to create watch directory", "dir", w.dir, "error", err)
		return
	}

	slog.Info("watching for NZB files", "dir", w.dir, "interval", w.interval)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.scan()
		}
	}
}

func (w *Watcher) scan() {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		slog.Error("failed to read watch directory", "error", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".nzb") {
			continue
		}

		path := filepath.Join(w.dir, name)
		w.processFile(path, name)
	}
}

func (w *Watcher) processFile(path, filename string) {
	parsed, err := nzb.ParseFile(path)
	if err != nil {
		slog.Error("failed to parse NZB from watch dir", "file", filename, "error", err)
		// Move to a .failed extension so we don't retry
		_ = os.Rename(path, path+".failed")
		return
	}

	name := strings.TrimSuffix(filename, ".nzb")
	if title, ok := parsed.Meta["title"]; ok && title != "" {
		name = title
	}

	// Detect category from NZB metadata or parent directory name
	category := ""
	if cat, ok := parsed.Meta["category"]; ok {
		category = cat
	}

	job := &queue.Job{
		ID:            w.idGen(),
		Name:          name,
		NZBPath:       path,
		Category:      category,
		TotalSegments: parsed.TotalSegments(),
		TotalBytes:    parsed.TotalBytes(),
	}

	if err := w.queue.Add(job); err != nil {
		slog.Warn("failed to add NZB from watch dir", "file", filename, "error", err)
		return
	}

	slog.Info("added NZB from watch directory", "file", filename, "name", name, "segments", job.TotalSegments)
}
