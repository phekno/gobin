// Package storage provides persistent state using bbolt (pure-Go key-value store).
// Stores queue state and download history so they survive restarts.
package storage

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	bucketJobs    = []byte("jobs")
	bucketHistory = []byte("history")
)

// Store wraps a bbolt database for job and history persistence.
type Store struct {
	db *bolt.DB
}

// JobRecord is a serializable representation of a queue job.
// queue.Job has sync/atomic fields that can't be serialized directly.
type JobRecord struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	NZBPath         string    `json:"nzb_path"`
	Category        string    `json:"category"`
	Priority        int       `json:"priority"`
	Status          string    `json:"status"`
	AddedAt         time.Time `json:"added_at"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	DoneAt          time.Time `json:"done_at,omitempty"`
	Error           string    `json:"error,omitempty"`
	TotalSegments   int       `json:"total_segments"`
	DoneSegments    int64     `json:"done_segments"`
	TotalBytes      int64     `json:"total_bytes"`
	DownloadedBytes int64     `json:"downloaded_bytes"`
	FailedSegments  int64     `json:"failed_segments"`
}

// HistoryEntry represents a completed or failed download.
type HistoryEntry struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Category      string    `json:"category"`
	Status        string    `json:"status"` // "completed" or "failed"
	Error         string    `json:"error,omitempty"`
	TotalBytes    int64     `json:"total_bytes"`
	DownloadedBytes int64   `json:"downloaded_bytes"`
	TotalSegments int       `json:"total_segments"`
	FailedSegments int64    `json:"failed_segments"`
	AddedAt       time.Time `json:"added_at"`
	StartedAt     time.Time `json:"started_at"`
	CompletedAt   time.Time `json:"completed_at"`
	Duration      string    `json:"duration"`
}

// Open creates or opens a bbolt database at the given path.
func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Create buckets if they don't exist
	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketJobs); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(bucketHistory)
		return err
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("creating buckets: %w", err)
	}

	slog.Info("storage opened", "path", path)
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// --- Job persistence ---

// SaveJob persists a job record.
func (s *Store) SaveJob(job *JobRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketJobs)
		data, err := json.Marshal(job)
		if err != nil {
			return err
		}
		return b.Put([]byte(job.ID), data)
	})
}

// GetJob retrieves a single job by ID.
func (s *Store) GetJob(id string) (*JobRecord, error) {
	var job JobRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketJobs)
		data := b.Get([]byte(id))
		if data == nil {
			return fmt.Errorf("job %s not found", id)
		}
		return json.Unmarshal(data, &job)
	})
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// ListJobs returns all persisted jobs.
func (s *Store) ListJobs() ([]*JobRecord, error) {
	var jobs []*JobRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketJobs)
		return b.ForEach(func(_, v []byte) error {
			var job JobRecord
			if err := json.Unmarshal(v, &job); err != nil {
				return err
			}
			jobs = append(jobs, &job)
			return nil
		})
	})
	return jobs, err
}

// DeleteJob removes a job record.
func (s *Store) DeleteJob(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketJobs).Delete([]byte(id))
	})
}

// --- History persistence ---

// SaveHistory persists a history entry.
func (s *Store) SaveHistory(entry *HistoryEntry) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketHistory)
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		return b.Put([]byte(entry.ID), data)
	})
}

// ListHistory returns history entries, newest first.
func (s *Store) ListHistory(limit int) ([]*HistoryEntry, error) {
	var entries []*HistoryEntry
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketHistory)
		c := b.Cursor()
		// Iterate in reverse (newest first by key order)
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			var entry HistoryEntry
			if err := json.Unmarshal(v, &entry); err != nil {
				continue
			}
			entries = append(entries, &entry)
			if limit > 0 && len(entries) >= limit {
				break
			}
		}
		return nil
	})
	return entries, err
}

// DeleteHistory removes a history entry.
func (s *Store) DeleteHistory(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketHistory).Delete([]byte(id))
	})
}

// CountHistory returns the number of history entries.
func (s *Store) CountHistory() int {
	count := 0
	_ = s.db.View(func(tx *bolt.Tx) error {
		count = tx.Bucket(bucketHistory).Stats().KeyN
		return nil
	})
	return count
}
