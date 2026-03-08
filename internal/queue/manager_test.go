package queue

import (
	"testing"
)

func TestNewManager(t *testing.T) {
	m := NewManager(5)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if len(m.List()) != 0 {
		t.Errorf("new manager should have empty queue, got %d", len(m.List()))
	}
}

func TestManager_Add(t *testing.T) {
	m := NewManager(3)
	job := &Job{ID: "job-1", Name: "Test Download"}

	if err := m.Add(job); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	jobs := m.List()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Status != StatusQueued {
		t.Errorf("Status = %v, want StatusQueued", jobs[0].Status)
	}
	if jobs[0].AddedAt.IsZero() {
		t.Error("AddedAt should be set")
	}
}

func TestManager_AddDuplicate(t *testing.T) {
	m := NewManager(3)
	job := &Job{ID: "dup-1", Name: "First"}
	m.Add(job)

	err := m.Add(&Job{ID: "dup-1", Name: "Second"})
	if err == nil {
		t.Error("expected error for duplicate job ID")
	}
}

func TestManager_Remove(t *testing.T) {
	m := NewManager(3)
	m.Add(&Job{ID: "rm-1", Name: "To Remove"})

	if err := m.Remove("rm-1"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if len(m.List()) != 0 {
		t.Error("queue should be empty after removal")
	}
}

func TestManager_RemoveNotFound(t *testing.T) {
	m := NewManager(3)
	err := m.Remove("nonexistent")
	if err == nil {
		t.Error("expected error for removing nonexistent job")
	}
}

func TestManager_Next_PriorityOrder(t *testing.T) {
	m := NewManager(3)
	m.Add(&Job{ID: "low", Name: "Low", Priority: 1})
	m.Add(&Job{ID: "high", Name: "High", Priority: 5})
	m.Add(&Job{ID: "mid", Name: "Mid", Priority: 3})

	next := m.Next()
	if next == nil {
		t.Fatal("Next returned nil")
	}
	if next.ID != "high" {
		t.Errorf("Next returned job %q, want high (priority 5)", next.ID)
	}
}

func TestManager_Next_WhenPaused(t *testing.T) {
	m := NewManager(3)
	m.Add(&Job{ID: "p-1", Name: "Test"})

	m.Pause("")
	if next := m.Next(); next != nil {
		t.Error("Next should return nil when queue is paused")
	}

	m.Resume("")
	if next := m.Next(); next == nil {
		t.Error("Next should return job after resume")
	}
}

func TestManager_PauseResumeJob(t *testing.T) {
	m := NewManager(3)
	m.Add(&Job{ID: "pr-1", Name: "Test"})

	m.Pause("pr-1")

	jobs := m.List()
	if jobs[0].Status != StatusPaused {
		t.Errorf("Status = %v, want StatusPaused", jobs[0].Status)
	}

	if next := m.Next(); next != nil {
		t.Error("paused job should not be returned by Next")
	}

	m.Resume("pr-1")
	jobs = m.List()
	if jobs[0].Status != StatusQueued {
		t.Errorf("Status = %v, want StatusQueued after resume", jobs[0].Status)
	}
}

func TestManager_ActiveJobs(t *testing.T) {
	m := NewManager(3)

	downloading := &Job{ID: "dl-1", Name: "Downloading"}
	downloading.Status = StatusDownloading

	queued := &Job{ID: "q-1", Name: "Queued"}

	// Add directly to avoid status override — Add sets StatusQueued,
	// so set downloading status after add
	m.Add(downloading)
	m.Add(queued)

	// Override status after Add
	m.mu.Lock()
	m.jobs[0].Status = StatusDownloading
	m.mu.Unlock()

	active := m.ActiveJobs()
	if len(active) != 1 {
		t.Fatalf("expected 1 active job, got %d", len(active))
	}
	if active[0].ID != "dl-1" {
		t.Errorf("active job ID = %q, want dl-1", active[0].ID)
	}
}

func TestJob_Progress(t *testing.T) {
	job := &Job{TotalSegments: 100}
	job.DoneSegments.Store(50)

	p := job.Progress()
	if p != 50.0 {
		t.Errorf("Progress = %f, want 50.0", p)
	}
}

func TestJob_Progress_ZeroTotal(t *testing.T) {
	job := &Job{TotalSegments: 0}
	p := job.Progress()
	if p != 0 {
		t.Errorf("Progress = %f, want 0 for zero total", p)
	}
}

func TestSpeedTracker_BytesPerSecond(t *testing.T) {
	st := &SpeedTracker{}

	// Zero samples
	if bps := st.BytesPerSecond(); bps != 0 {
		t.Errorf("BytesPerSecond with no samples = %f, want 0", bps)
	}

	// One sample
	st.Record(1000)
	if bps := st.BytesPerSecond(); bps != 0 {
		t.Errorf("BytesPerSecond with 1 sample = %f, want 0", bps)
	}

	// Multiple samples
	st.Record(2000)
	st.Record(3000)
	bps := st.BytesPerSecond()
	if bps < 0 {
		t.Errorf("BytesPerSecond should be non-negative, got %f", bps)
	}
}

func TestStatus_String(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusQueued, "queued"},
		{StatusDownloading, "downloading"},
		{StatusAssembling, "assembling"},
		{StatusPostProcessing, "post-processing"},
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
		{StatusPaused, "paused"},
		{Status(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("Status(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}
