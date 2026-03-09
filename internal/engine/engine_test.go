package engine

import (
	"testing"
	"time"

	"github.com/phekno/gobin/internal/queue"
	"github.com/phekno/gobin/internal/storage"
)

func TestJobToRecord(t *testing.T) {
	job := &queue.Job{
		ID:            "test-1",
		Name:          "Test Download",
		NZBPath:       "/tmp/test.nzb",
		Category:      "tv",
		Priority:      5,
		AddedAt:       time.Now(),
		TotalSegments: 100,
		TotalBytes:    50000,
	}
	job.SetStatus(queue.StatusDownloading)
	job.DoneSegments.Store(50)
	job.DownloadedBytes.Store(25000)
	job.FailedSegments.Store(2)

	rec := jobToRecord(job)

	if rec.ID != "test-1" {
		t.Errorf("ID = %q", rec.ID)
	}
	if rec.Status != "downloading" {
		t.Errorf("Status = %q", rec.Status)
	}
	if rec.DoneSegments != 50 {
		t.Errorf("DoneSegments = %d", rec.DoneSegments)
	}
	if rec.DownloadedBytes != 25000 {
		t.Errorf("DownloadedBytes = %d", rec.DownloadedBytes)
	}
	if rec.FailedSegments != 2 {
		t.Errorf("FailedSegments = %d", rec.FailedSegments)
	}
}

func TestRecordToJob(t *testing.T) {
	rec := &storage.JobRecord{
		ID:              "rec-1",
		Name:            "From Record",
		NZBPath:         "/nzb/test.nzb",
		Category:        "movies",
		Priority:        3,
		Status:          "paused",
		TotalSegments:   200,
		TotalBytes:      100000,
		DoneSegments:    75,
		DownloadedBytes: 40000,
		FailedSegments:  5,
	}

	job := recordToJob(rec)

	if job.ID != "rec-1" {
		t.Errorf("ID = %q", job.ID)
	}
	if job.GetStatus() != queue.StatusPaused {
		t.Errorf("Status = %v", job.GetStatus())
	}
	if job.DoneSegments.Load() != 75 {
		t.Errorf("DoneSegments = %d", job.DoneSegments.Load())
	}
	if job.DownloadedBytes.Load() != 40000 {
		t.Errorf("DownloadedBytes = %d", job.DownloadedBytes.Load())
	}
	if job.FailedSegments.Load() != 5 {
		t.Errorf("FailedSegments = %d", job.FailedSegments.Load())
	}
}

func TestRecordToJob_AllStatuses(t *testing.T) {
	statuses := map[string]queue.Status{
		"queued":          queue.StatusQueued,
		"downloading":     queue.StatusDownloading,
		"paused":          queue.StatusPaused,
		"post-processing": queue.StatusPostProcessing,
		"completed":       queue.StatusCompleted,
		"failed":          queue.StatusFailed,
		"unknown":         queue.StatusQueued, // default
	}
	for s, expected := range statuses {
		rec := &storage.JobRecord{ID: "s-" + s, Status: s}
		job := recordToJob(rec)
		if job.GetStatus() != expected {
			t.Errorf("status %q: got %v, want %v", s, job.GetStatus(), expected)
		}
	}
}
