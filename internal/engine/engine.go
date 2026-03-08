// Package engine orchestrates the download pipeline:
// queue → NZB parse → NNTP fetch → yEnc decode → file assembly.
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/phekno/gobin/internal/assembler"
	"github.com/phekno/gobin/internal/config"
	"github.com/phekno/gobin/internal/decoder"
	"github.com/phekno/gobin/internal/logging"
	"github.com/phekno/gobin/internal/metrics"
	"github.com/phekno/gobin/internal/nntp"
	"github.com/phekno/gobin/internal/nzb"
	"github.com/phekno/gobin/internal/queue"
)

// Engine watches the queue and downloads NZBs.
type Engine struct {
	queue  *queue.Manager
	cfgMgr *config.Manager
}

// New creates a download engine.
func New(q *queue.Manager, cfgMgr *config.Manager) *Engine {
	return &Engine{queue: q, cfgMgr: cfgMgr}
}

// Run starts the engine loop. It watches for queued jobs and processes them.
// Blocks until the context is cancelled.
func (e *Engine) Run(ctx context.Context) {
	slog.Info("download engine started")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("download engine stopping")
			return
		case <-ticker.C:
			job := e.queue.Next()
			if job == nil {
				continue
			}
			e.processJob(ctx, job)
		}
	}
}

func (e *Engine) processJob(ctx context.Context, job *queue.Job) {
	logger := logging.WithJob(slog.Default(), job.ID, job.Name)

	// Mark as downloading
	job.SetStatus(queue.StatusDownloading)
	job.StartedAt = time.Now()

	logger.Info("starting download", "nzb_path", job.NZBPath, "segments", job.TotalSegments)
	logging.LogDownloadStart(logger, job.ID, job.TotalSegments, job.TotalBytes)

	cfg := e.cfgMgr.Get()

	// Parse the NZB file
	parsed, err := nzb.ParseFile(job.NZBPath)
	if err != nil {
		logger.Error("failed to parse NZB", "error", err)
		job.Error = fmt.Sprintf("NZB parse error: %v", err)
		job.SetStatus(queue.StatusFailed)
		job.DoneAt = time.Now()
		return
	}

	// Create assembler for this job
	outputDir := cfg.General.CompleteDir
	if job.Category != "" {
		// Check if category has a custom dir
		for _, cat := range cfg.Categories {
			if strings.EqualFold(cat.Name, job.Category) && cat.Dir != "" {
				outputDir = outputDir + "/" + cat.Dir
				break
			}
		}
	}

	asm, err := assembler.New(cfg.General.DownloadDir+"/"+job.Name, outputDir)
	if err != nil {
		logger.Error("failed to create assembler", "error", err)
		job.Error = fmt.Sprintf("assembler error: %v", err)
		job.SetStatus(queue.StatusFailed)
		job.DoneAt = time.Now()
		return
	}

	// Build NNTP connection pools (sorted by priority)
	pools, err := e.createPools(cfg)
	if err != nil {
		logger.Error("failed to create NNTP pools", "error", err)
		job.Error = fmt.Sprintf("NNTP connection error: %v", err)
		job.SetStatus(queue.StatusFailed)
		job.DoneAt = time.Now()
		return
	}
	defer e.closePools(pools)

	metrics.NNTPConnectionsActive.Set(int64(len(pools)))

	// Download each file in the NZB
	for _, file := range parsed.Files {
		if ctx.Err() != nil {
			logger.Info("download cancelled")
			job.SetStatus(queue.StatusFailed)
			job.Error = "cancelled"
			job.DoneAt = time.Now()
			return
		}

		// Check if job was paused
		if job.GetStatus() == queue.StatusPaused {
			logger.Info("job paused, stopping")
			return
		}

		filename := file.Filename()
		logger.Info("downloading file", "filename", filename, "segments", len(file.Segments))

		err := e.downloadFile(ctx, job, logger, pools, asm, file, cfg.Downloads.MaxRetries)
		if err != nil {
			logger.Error("file download failed", "filename", filename, "error", err)
			// Continue with next file — partial downloads are still useful
			continue
		}

		// Move completed file to output
		if err := asm.Finalize(filename); err != nil {
			logger.Error("failed to finalize file", "filename", filename, "error", err)
		}
	}

	metrics.NNTPConnectionsActive.Set(0)

	// Mark complete
	job.DoneAt = time.Now()
	duration := job.DoneAt.Sub(job.StartedAt)
	job.SetStatus(queue.StatusCompleted)

	logging.LogDownloadComplete(logger, job.ID, duration, job.DownloadedBytes.Load())
	logger.Info("download complete",
		"duration", duration,
		"segments_ok", job.DoneSegments.Load(),
		"segments_failed", job.FailedSegments.Load(),
	)
}

// downloadFile fetches all segments for a single file and writes them to disk.
func (e *Engine) downloadFile(
	ctx context.Context,
	job *queue.Job,
	logger *slog.Logger,
	pools []*serverPool,
	asm *assembler.Assembler,
	file nzb.File,
	maxRetries int,
) error {
	filename := file.Filename()

	f, err := asm.CreateFile(filename)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Segments are already sorted by number (parser does this).
	// Fetch them sequentially to write in order.
	// Use a worker pool for concurrent fetching with ordered output.
	type result struct {
		index int
		data  []byte
		err   error
	}

	concurrency := pools[0].pool.MaxConns()
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > len(file.Segments) {
		concurrency = len(file.Segments)
	}

	segCh := make(chan int, len(file.Segments))
	for i := range file.Segments {
		segCh <- i
	}
	close(segCh)

	resultCh := make(chan result, concurrency)

	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range segCh {
				if ctx.Err() != nil {
					return
				}
				seg := file.Segments[idx]
				data, fetchErr := e.fetchSegment(ctx, pools, seg.MessageID, maxRetries)
				resultCh <- result{index: idx, data: data, err: fetchErr}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results and write in order
	results := make([]result, len(file.Segments))
	received := 0

	for r := range resultCh {
		results[r.index] = r
		received++
	}

	// Write segments in order
	for i, r := range results {
		if r.err != nil {
			job.FailedSegments.Add(1)
			metrics.DownloadSegmentsFailed.Inc()
			logging.LogSegmentError(logger, job.ID, file.Segments[i].MessageID, 0, r.err)
			continue
		}

		// Decode yEnc
		decoded, decErr := decoder.DecodeYEnc(r.data)
		if decErr != nil {
			// CRC mismatch returns both result and error — use the data anyway
			if decoded == nil {
				job.FailedSegments.Add(1)
				metrics.DownloadSegmentsFailed.Inc()
				metrics.YEncCRCErrors.Inc()
				logging.LogSegmentError(logger, job.ID, file.Segments[i].MessageID, 0, decErr)
				continue
			}
			metrics.YEncCRCErrors.Inc()
			logger.Warn("yenc CRC mismatch, using data anyway",
				"segment", file.Segments[i].Number,
				"error", decErr,
			)
		}

		if err := asm.WriteSegment(f, decoded.Data); err != nil {
			return fmt.Errorf("writing segment %d: %w", file.Segments[i].Number, err)
		}

		metrics.YEncDecodedBytesTotal.Add(int64(len(decoded.Data)))
		job.DoneSegments.Add(1)
		job.DownloadedBytes.Add(int64(len(decoded.Data)))
		metrics.DownloadBytesTotal.Add(int64(len(r.data)))
		metrics.DownloadSegmentsOK.Inc()
	}

	return nil
}

// serverPool wraps an NNTP pool with its priority and groups.
type serverPool struct {
	pool     *nntp.Pool
	priority int
	name     string
}

// fetchSegment fetches a single segment, trying servers by priority with retries.
func (e *Engine) fetchSegment(ctx context.Context, pools []*serverPool, messageID string, maxRetries int) ([]byte, error) {
	var lastErr error

	for _, sp := range pools {
		for attempt := range maxRetries {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			client, err := sp.pool.Get(ctx)
			if err != nil {
				lastErr = fmt.Errorf("pool %s: get connection: %w", sp.name, err)
				continue
			}

			data, err := client.Body(messageID)
			sp.pool.Put(client)

			if err == nil {
				return data, nil
			}

			lastErr = err

			// Article not found — try next server (don't retry same server)
			if strings.Contains(err.Error(), "not found") {
				break
			}

			// Other errors — retry on same server
			slog.Debug("segment fetch retry",
				"server", sp.name,
				"message_id", messageID,
				"attempt", attempt+1,
				"error", err,
			)
		}
	}

	return nil, fmt.Errorf("all servers exhausted for %s: %w", messageID, lastErr)
}

func (e *Engine) createPools(cfg *config.Config) ([]*serverPool, error) {
	if len(cfg.Servers) == 0 {
		return nil, fmt.Errorf("no servers configured")
	}

	pools := make([]*serverPool, 0, len(cfg.Servers))

	for _, srv := range cfg.Servers {
		if srv.Host == "" {
			continue
		}
		nntpCfg := nntp.ServerConfig{
			Host:     srv.Host,
			Port:     srv.Port,
			TLS:      srv.TLS,
			Username: srv.Username,
			Password: srv.Password,
		}
		conns := srv.Connections
		if conns < 1 {
			conns = 1
		}
		pool := nntp.NewPool(nntpCfg, conns)
		pools = append(pools, &serverPool{
			pool:     pool,
			priority: srv.Priority,
			name:     srv.Name,
		})
	}

	if len(pools) == 0 {
		return nil, fmt.Errorf("no valid servers configured")
	}

	// Sort by priority (0 = primary, higher = backup)
	for i := 1; i < len(pools); i++ {
		for j := i; j > 0 && pools[j].priority < pools[j-1].priority; j-- {
			pools[j], pools[j-1] = pools[j-1], pools[j]
		}
	}

	slog.Info("NNTP pools created",
		"servers", len(pools),
		"primary", pools[0].name,
	)

	return pools, nil
}

func (e *Engine) closePools(pools []*serverPool) {
	for _, sp := range pools {
		sp.pool.Close()
	}
}
