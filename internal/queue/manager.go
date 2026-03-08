package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// Status represents the state of a download job.
type Status int

const (
	StatusQueued Status = iota
	StatusDownloading
	StatusAssembling
	StatusPostProcessing
	StatusCompleted
	StatusFailed
	StatusPaused
)

func (s Status) String() string {
	switch s {
	case StatusQueued:
		return "queued"
	case StatusDownloading:
		return "downloading"
	case StatusAssembling:
		return "assembling"
	case StatusPostProcessing:
		return "post-processing"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	case StatusPaused:
		return "paused"
	default:
		return "unknown"
	}
}

// Job represents a single NZB download job.
type Job struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	NZBPath   string    `json:"nzb_path"`
	Category  string    `json:"category"`
	Priority  int       `json:"priority"` // Higher = more urgent
	Status    Status    `json:"status"`
	AddedAt   time.Time `json:"added_at"`
	StartedAt time.Time `json:"started_at,omitempty"`
	DoneAt    time.Time `json:"done_at,omitempty"`
	Error     string    `json:"error,omitempty"`

	// Progress tracking
	TotalSegments    int   `json:"total_segments"`
	DoneSegments     atomic.Int64 // Use atomic for lock-free progress updates
	TotalBytes       int64 `json:"total_bytes"`
	DownloadedBytes  atomic.Int64
	FailedSegments   atomic.Int64

	mu sync.RWMutex
}

// Progress returns download progress as a percentage (0-100).
func (j *Job) Progress() float64 {
	total := j.TotalSegments
	if total == 0 {
		return 0
	}
	return float64(j.DoneSegments.Load()) / float64(total) * 100
}

// SpeedBps returns current download speed (caller tracks this externally).
// This is a placeholder — real implementation would use a sliding window.
type SpeedTracker struct {
	samples []speedSample
	mu      sync.Mutex
}

type speedSample struct {
	bytes int64
	at    time.Time
}

func (st *SpeedTracker) Record(bytes int64) {
	st.mu.Lock()
	defer st.mu.Unlock()
	now := time.Now()
	st.samples = append(st.samples, speedSample{bytes, now})
	// Keep only last 10 seconds of samples
	cutoff := now.Add(-10 * time.Second)
	for len(st.samples) > 0 && st.samples[0].at.Before(cutoff) {
		st.samples = st.samples[1:]
	}
}

func (st *SpeedTracker) BytesPerSecond() float64 {
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.samples) < 2 {
		return 0
	}
	first := st.samples[0]
	last := st.samples[len(st.samples)-1]
	duration := last.at.Sub(first.at).Seconds()
	if duration == 0 {
		return 0
	}
	var totalBytes int64
	for _, s := range st.samples {
		totalBytes += s.bytes
	}
	return float64(totalBytes) / duration
}

// Manager orchestrates the download queue.
type Manager struct {
	jobs    []*Job
	mu      sync.RWMutex
	paused  bool

	// Channels for coordinating with the download engine
	added   chan *Job
	cancel  map[string]context.CancelFunc

	maxConcurrent int
	activeCount   atomic.Int32
}

// NewManager creates a new queue manager.
func NewManager(maxConcurrent int) *Manager {
	return &Manager{
		jobs:          make([]*Job, 0),
		added:         make(chan *Job, 100),
		cancel:        make(map[string]context.CancelFunc),
		maxConcurrent: maxConcurrent,
	}
}

// Add adds a new job to the queue.
func (m *Manager) Add(job *Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicate
	for _, existing := range m.jobs {
		if existing.ID == job.ID {
			return fmt.Errorf("job %s already exists", job.ID)
		}
	}

	job.Status = StatusQueued
	job.AddedAt = time.Now()
	m.jobs = append(m.jobs, job)

	slog.Info("job added to queue", "id", job.ID, "name", job.Name, "category", job.Category)

	// Signal that a new job is available
	select {
	case m.added <- job:
	default:
	}

	return nil
}

// Remove removes a job from the queue (cancels if active).
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, job := range m.jobs {
		if job.ID == id {
			// Cancel if running
			if cancelFn, ok := m.cancel[id]; ok {
				cancelFn()
				delete(m.cancel, id)
			}
			m.jobs = append(m.jobs[:i], m.jobs[i+1:]...)
			slog.Info("job removed", "id", id)
			return nil
		}
	}
	return fmt.Errorf("job %s not found", id)
}

// Pause pauses a specific job or the entire queue.
func (m *Manager) Pause(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if id == "" {
		m.paused = true
		slog.Info("queue paused")
		return
	}

	for _, job := range m.jobs {
		if job.ID == id {
			job.mu.Lock()
			job.Status = StatusPaused
			job.mu.Unlock()
			slog.Info("job paused", "id", id)
			return
		}
	}
}

// Resume resumes a specific job or the entire queue.
func (m *Manager) Resume(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if id == "" {
		m.paused = false
		slog.Info("queue resumed")
		return
	}

	for _, job := range m.jobs {
		if job.ID == id {
			job.mu.Lock()
			if job.Status == StatusPaused {
				job.Status = StatusQueued
			}
			job.mu.Unlock()
			slog.Info("job resumed", "id", id)
			return
		}
	}
}

// Next returns the next job eligible for download.
func (m *Manager) Next() *Job {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.paused {
		return nil
	}

	// Find highest priority queued job
	var best *Job
	for _, job := range m.jobs {
		job.mu.RLock()
		status := job.Status
		job.mu.RUnlock()

		if status == StatusQueued {
			if best == nil || job.Priority > best.Priority {
				best = job
			}
		}
	}
	return best
}

// List returns all jobs (for API responses).
func (m *Manager) List() []*Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Job, len(m.jobs))
	copy(result, m.jobs)
	return result
}

// IsPaused returns whether the entire queue is paused.
func (m *Manager) IsPaused() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.paused
}

// ActiveJobs returns jobs currently downloading.
func (m *Manager) ActiveJobs() []*Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var active []*Job
	for _, job := range m.jobs {
		job.mu.RLock()
		if job.Status == StatusDownloading {
			active = append(active, job)
		}
		job.mu.RUnlock()
	}
	return active
}
