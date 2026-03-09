package storage

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpenClose(t *testing.T) {
	s := testStore(t)
	if s == nil {
		t.Fatal("store is nil")
	}
}

func TestSaveAndGetJob(t *testing.T) {
	s := testStore(t)
	job := &JobRecord{
		ID:     "job-1",
		Name:   "Test Download",
		Status: "queued",
	}
	if err := s.SaveJob(job); err != nil {
		t.Fatalf("SaveJob: %v", err)
	}

	got, err := s.GetJob("job-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.Name != "Test Download" {
		t.Errorf("Name = %q", got.Name)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	s := testStore(t)
	_, err := s.GetJob("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent job")
	}
}

func TestListJobs(t *testing.T) {
	s := testStore(t)
	_ = s.SaveJob(&JobRecord{ID: "j1", Name: "First"})
	_ = s.SaveJob(&JobRecord{ID: "j2", Name: "Second"})

	jobs, err := s.ListJobs()
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}
}

func TestDeleteJob(t *testing.T) {
	s := testStore(t)
	_ = s.SaveJob(&JobRecord{ID: "del-1", Name: "To Delete"})

	if err := s.DeleteJob("del-1"); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}

	jobs, _ := s.ListJobs()
	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs after delete, got %d", len(jobs))
	}
}

func TestSaveAndListHistory(t *testing.T) {
	s := testStore(t)
	_ = s.SaveHistory(&HistoryEntry{
		ID:          "h1",
		Name:        "Completed Download",
		Status:      "completed",
		CompletedAt: time.Now(),
	})
	_ = s.SaveHistory(&HistoryEntry{
		ID:          "h2",
		Name:        "Failed Download",
		Status:      "failed",
		CompletedAt: time.Now(),
	})

	entries, err := s.ListHistory(10)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestListHistory_Limit(t *testing.T) {
	s := testStore(t)
	for i := range 5 {
		_ = s.SaveHistory(&HistoryEntry{
			ID:   fmt.Sprintf("h%d", i),
			Name: fmt.Sprintf("Download %d", i),
		})
	}

	entries, err := s.ListHistory(3)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries with limit, got %d", len(entries))
	}
}

func TestDeleteHistory(t *testing.T) {
	s := testStore(t)
	_ = s.SaveHistory(&HistoryEntry{ID: "hdel", Name: "To Delete"})

	if err := s.DeleteHistory("hdel"); err != nil {
		t.Fatalf("DeleteHistory: %v", err)
	}
	if s.CountHistory() != 0 {
		t.Error("expected 0 history entries after delete")
	}
}

func TestCountHistory(t *testing.T) {
	s := testStore(t)
	if s.CountHistory() != 0 {
		t.Error("expected 0 initially")
	}
	_ = s.SaveHistory(&HistoryEntry{ID: "c1"})
	_ = s.SaveHistory(&HistoryEntry{ID: "c2"})
	if s.CountHistory() != 2 {
		t.Errorf("expected 2, got %d", s.CountHistory())
	}
}
