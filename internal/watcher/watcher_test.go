package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/phekno/gobin/internal/queue"
)

var testIDCounter int

func testIDGen() string {
	testIDCounter++
	return fmt.Sprintf("test-%d", testIDCounter)
}

func TestProcessFile_ValidNZB(t *testing.T) {
	dir := t.TempDir()
	q := queue.NewManager(3)

	w := New(dir, q, 1*time.Second, testIDGen)

	nzbContent := `<?xml version="1.0"?>
<nzb xmlns="http://www.newzbin.com/DTD/2003/nzb">
  <head><meta type="title">Test File</meta></head>
  <file poster="u@e" date="1000" subject="test">
    <groups><group>alt.test</group></groups>
    <segments><segment bytes="100" number="1">a@b</segment></segments>
  </file>
</nzb>`

	path := filepath.Join(dir, "test.nzb")
	_ = os.WriteFile(path, []byte(nzbContent), 0644)

	w.processFile(path, "test.nzb")

	jobs := q.List()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Name != "Test File" {
		t.Errorf("name = %q, want 'Test File'", jobs[0].Name)
	}
	if jobs[0].TotalSegments != 1 {
		t.Errorf("segments = %d", jobs[0].TotalSegments)
	}
}

func TestProcessFile_InvalidNZB(t *testing.T) {
	dir := t.TempDir()
	q := queue.NewManager(3)

	w := New(dir, q, 1*time.Second, testIDGen)

	path := filepath.Join(dir, "bad.nzb")
	_ = os.WriteFile(path, []byte("not valid xml"), 0644)

	w.processFile(path, "bad.nzb")

	// Should not add to queue
	if len(q.List()) != 0 {
		t.Error("invalid NZB should not be added to queue")
	}
	// Should be renamed to .failed
	if _, err := os.Stat(path + ".failed"); err != nil {
		t.Error("invalid NZB should be renamed to .failed")
	}
}

func TestScan(t *testing.T) {
	dir := t.TempDir()
	q := queue.NewManager(3)
	w := New(dir, q, 1*time.Second, testIDGen)

	nzb := `<?xml version="1.0"?>
<nzb xmlns="http://www.newzbin.com/DTD/2003/nzb">
  <head></head>
  <file poster="u@e" date="1000" subject="scan test">
    <groups><group>alt.test</group></groups>
    <segments><segment bytes="50" number="1">x@y</segment></segments>
  </file>
</nzb>`

	// Write two NZB files and one non-NZB
	_ = os.WriteFile(filepath.Join(dir, "file1.nzb"), []byte(nzb), 0644)
	_ = os.WriteFile(filepath.Join(dir, "file2.nzb"), []byte(nzb), 0644)
	_ = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not an nzb"), 0644)

	w.scan()

	if len(q.List()) != 2 {
		t.Errorf("expected 2 jobs from scan, got %d", len(q.List()))
	}
}
