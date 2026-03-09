// Package engine orchestrates the download pipeline:
// queue → NZB parse → NNTP fetch → yEnc decode → file assembly → post-processing.
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
	"github.com/phekno/gobin/internal/notify"
	"github.com/phekno/gobin/internal/nzb"
	"github.com/phekno/gobin/internal/postprocess"
	"github.com/phekno/gobin/internal/queue"
	"github.com/phekno/gobin/internal/storage"
)

// Engine watches the queue and downloads NZBs.
type Engine struct {
	queue    *queue.Manager
	cfgMgr   *config.Manager
	store    *storage.Store
	notifier *notify.Notifier
	Speed    *queue.SpeedTracker
}

// New creates a download engine.
func New(q *queue.Manager, cfgMgr *config.Manager, store *storage.Store, notifier *notify.Notifier) *Engine {
	return &Engine{
		queue:    q,
		cfgMgr:  cfgMgr,
		store:   store,
		notifier: notifier,
		Speed:   &queue.SpeedTracker{},
	}
}

// Run starts the engine loop. It watches for queued jobs and processes them.
// Blocks until the context is cancelled.
func (e *Engine) Run(ctx context.Context) {
	slog.Info("download engine started")

	// Restore queued jobs from storage on startup
	e.restoreJobs()

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

// restoreJobs reloads persisted jobs from storage into the queue.
func (e *Engine) restoreJobs() {
	records, err := e.store.ListJobs()
	if err != nil {
		slog.Error("failed to restore jobs from storage", "error", err)
		return
	}
	for _, rec := range records {
		job := recordToJob(rec)
		// Only restore jobs that weren't completed/failed
		if job.GetStatus() == queue.StatusQueued ||
			job.GetStatus() == queue.StatusDownloading ||
			job.GetStatus() == queue.StatusPaused {
			// Reset downloading jobs to queued (they'll restart)
			if job.GetStatus() == queue.StatusDownloading {
				job.SetStatus(queue.StatusQueued)
			}
			if err := e.queue.Add(job); err != nil {
				slog.Warn("skipping duplicate restored job", "id", rec.ID, "error", err)
				continue
			}
			slog.Info("restored job from storage", "id", rec.ID, "name", rec.Name, "status", rec.Status)
		}
	}
}

// persistJob saves the current job state to storage.
func (e *Engine) persistJob(job *queue.Job) {
	rec := jobToRecord(job)
	if err := e.store.SaveJob(rec); err != nil {
		slog.Error("failed to persist job", "id", job.ID, "error", err)
	}
}

// moveToHistory saves a completed/failed job to history and removes from active jobs.
func (e *Engine) moveToHistory(job *queue.Job) {
	entry := &storage.HistoryEntry{
		ID:              job.ID,
		Name:            job.Name,
		Category:        job.Category,
		Status:          job.GetStatus().String(),
		Error:           job.Error,
		TotalBytes:      job.TotalBytes,
		DownloadedBytes: job.DownloadedBytes.Load(),
		TotalSegments:   job.TotalSegments,
		FailedSegments:  job.FailedSegments.Load(),
		AddedAt:         job.AddedAt,
		StartedAt:       job.StartedAt,
		CompletedAt:     job.DoneAt,
	}
	if !job.StartedAt.IsZero() && !job.DoneAt.IsZero() {
		entry.Duration = job.DoneAt.Sub(job.StartedAt).Round(time.Second).String()
	}

	if err := e.store.SaveHistory(entry); err != nil {
		slog.Error("failed to save history", "id", job.ID, "error", err)
	}
	_ = e.store.DeleteJob(job.ID)
}

func (e *Engine) processJob(ctx context.Context, job *queue.Job) {
	logger := logging.WithJob(slog.Default(), job.ID, job.Name)

	// Mark as downloading
	job.SetStatus(queue.StatusDownloading)
	job.StartedAt = time.Now()
	e.persistJob(job)

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
		e.moveToHistory(job)
		return
	}

	// Determine output directory
	outputDir := cfg.General.CompleteDir
	if job.Category != "" {
		for _, cat := range cfg.Categories {
			if strings.EqualFold(cat.Name, job.Category) && cat.Dir != "" {
				outputDir = outputDir + "/" + cat.Dir
				break
			}
		}
	}

	workDir := cfg.General.DownloadDir + "/" + job.Name

	asm, err := assembler.New(workDir, outputDir)
	if err != nil {
		logger.Error("failed to create assembler", "error", err)
		job.Error = fmt.Sprintf("assembler error: %v", err)
		job.SetStatus(queue.StatusFailed)
		job.DoneAt = time.Now()
		e.moveToHistory(job)
		return
	}

	// Build NNTP connection pools
	pools, err := e.createPools(cfg)
	if err != nil {
		logger.Error("failed to create NNTP pools", "error", err)
		job.Error = fmt.Sprintf("NNTP connection error: %v", err)
		job.SetStatus(queue.StatusFailed)
		job.DoneAt = time.Now()
		e.moveToHistory(job)
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
			e.moveToHistory(job)
			return
		}

		if job.GetStatus() == queue.StatusPaused {
			logger.Info("job paused, stopping")
			e.persistJob(job)
			return
		}

		filename := file.Filename()
		logger.Info("downloading file", "filename", filename, "segments", len(file.Segments))

		err := e.downloadFile(ctx, job, logger, pools, asm, file, cfg.Downloads.MaxRetries)
		if err != nil {
			logger.Error("file download failed", "filename", filename, "error", err)
			continue
		}

		// Move completed file to output
		if err := asm.Finalize(filename); err != nil {
			logger.Error("failed to finalize file", "filename", filename, "error", err)
		}

		// Persist progress periodically
		e.persistJob(job)
	}

	metrics.NNTPConnectionsActive.Set(0)

	// Post-processing
	if cfg.PostProcess.Par2Enabled || cfg.PostProcess.UnpackEnabled {
		job.SetStatus(queue.StatusPostProcessing)
		e.persistJob(job)
		logging.LogPostProcess(logger, job.ID, "pipeline", "starting")

		pp := postprocess.New(cfg.PostProcess)
		result := pp.Run(logger, job.ID, outputDir)

		if result.Error != nil {
			logger.Error("post-processing failed", "error", result.Error)
			job.Error = fmt.Sprintf("post-processing: %v", result.Error)
		}
	}

	// Mark complete
	job.DoneAt = time.Now()
	duration := job.DoneAt.Sub(job.StartedAt)

	if job.Error != "" {
		job.SetStatus(queue.StatusFailed)
	} else {
		job.SetStatus(queue.StatusCompleted)
	}

	logging.LogDownloadComplete(logger, job.ID, duration, job.DownloadedBytes.Load())

	// Send notifications
	eventType := "complete"
	if job.Error != "" {
		eventType = "failed"
	}
	e.notifier.Notify(ctx, notify.Event{
		Type:     eventType,
		Name:     job.Name,
		Category: job.Category,
		Status:   job.GetStatus().String(),
		Size:     job.DownloadedBytes.Load(),
		Duration: duration,
		Error:    job.Error,
	})

	// Move to history
	e.moveToHistory(job)

	// Remove from queue
	_ = e.queue.Remove(job.ID)
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

	// Collect results
	results := make([]result, len(file.Segments))
	for r := range resultCh {
		results[r.index] = r
	}

	// Write segments in order
	for i, r := range results {
		if r.err != nil {
			job.FailedSegments.Add(1)
			metrics.DownloadSegmentsFailed.Inc()
			logging.LogSegmentError(logger, job.ID, file.Segments[i].MessageID, 0, r.err)
			continue
		}

		decoded, decErr := decoder.DecodeYEnc(r.data)
		if decErr != nil {
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

		dataLen := int64(len(decoded.Data))
		metrics.YEncDecodedBytesTotal.Add(dataLen)
		job.DoneSegments.Add(1)
		job.DownloadedBytes.Add(dataLen)
		metrics.DownloadBytesTotal.Add(int64(len(r.data)))
		metrics.DownloadSegmentsOK.Inc()
		e.Speed.Record(dataLen)
		metrics.DownloadSpeedBps.Set(int64(e.Speed.BytesPerSecond()))
	}

	return nil
}

// serverPool wraps an NNTP pool with its priority.
type serverPool struct {
	pool     *nntp.Pool
	priority int
	name     string
}

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

			if strings.Contains(err.Error(), "not found") {
				break
			}

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
		if srv.Host == "" || srv.Host == "news.example.com" {
			continue // Skip placeholder servers
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
		return nil, fmt.Errorf("no valid servers configured (update your config with real Usenet server details)")
	}

	// Sort by priority (0 = primary, higher = backup)
	for i := 1; i < len(pools); i++ {
		for j := i; j > 0 && pools[j].priority < pools[j-1].priority; j-- {
			pools[j], pools[j-1] = pools[j-1], pools[j]
		}
	}

	slog.Info("NNTP pools created", "servers", len(pools), "primary", pools[0].name)
	return pools, nil
}

func (e *Engine) closePools(pools []*serverPool) {
	for _, sp := range pools {
		sp.pool.Close()
	}
}

// --- Conversion helpers ---

func jobToRecord(j *queue.Job) *storage.JobRecord {
	return &storage.JobRecord{
		ID:              j.ID,
		Name:            j.Name,
		NZBPath:         j.NZBPath,
		Category:        j.Category,
		Priority:        j.Priority,
		Status:          j.GetStatus().String(),
		AddedAt:         j.AddedAt,
		StartedAt:       j.StartedAt,
		DoneAt:          j.DoneAt,
		Error:           j.Error,
		TotalSegments:   j.TotalSegments,
		DoneSegments:    j.DoneSegments.Load(),
		TotalBytes:      j.TotalBytes,
		DownloadedBytes: j.DownloadedBytes.Load(),
		FailedSegments:  j.FailedSegments.Load(),
	}
}

func recordToJob(r *storage.JobRecord) *queue.Job {
	j := &queue.Job{
		ID:            r.ID,
		Name:          r.Name,
		NZBPath:       r.NZBPath,
		Category:      r.Category,
		Priority:      r.Priority,
		AddedAt:       r.AddedAt,
		StartedAt:     r.StartedAt,
		DoneAt:        r.DoneAt,
		Error:         r.Error,
		TotalSegments: r.TotalSegments,
		TotalBytes:    r.TotalBytes,
	}
	j.DoneSegments.Store(r.DoneSegments)
	j.DownloadedBytes.Store(r.DownloadedBytes)
	j.FailedSegments.Store(r.FailedSegments)

	// Parse status string back to Status type
	switch r.Status {
	case "queued":
		j.SetStatus(queue.StatusQueued)
	case "downloading":
		j.SetStatus(queue.StatusDownloading)
	case "paused":
		j.SetStatus(queue.StatusPaused)
	case "post-processing":
		j.SetStatus(queue.StatusPostProcessing)
	case "completed":
		j.SetStatus(queue.StatusCompleted)
	case "failed":
		j.SetStatus(queue.StatusFailed)
	default:
		j.SetStatus(queue.StatusQueued)
	}

	return j
}
