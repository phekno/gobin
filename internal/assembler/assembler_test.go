package assembler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateAndWriteSegment(t *testing.T) {
	workDir := t.TempDir()
	outDir := t.TempDir()

	asm, err := New(workDir, outDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	f, err := asm.CreateFile("test.bin")
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}

	if err := asm.WriteSegment(f, []byte("hello ")); err != nil {
		t.Fatalf("WriteSegment: %v", err)
	}
	if err := asm.WriteSegment(f, []byte("world")); err != nil {
		t.Fatalf("WriteSegment: %v", err)
	}
	_ = f.Close()

	// Verify file exists in work dir
	data, err := os.ReadFile(filepath.Join(workDir, "test.bin"))
	if err != nil {
		t.Fatalf("reading work file: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("content = %q, want 'hello world'", data)
	}
}

func TestFinalize(t *testing.T) {
	workDir := t.TempDir()
	outDir := t.TempDir()

	asm, err := New(workDir, outDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Create a file in workDir
	f, _ := asm.CreateFile("movie.mkv")
	_ = asm.WriteSegment(f, []byte("video data"))
	_ = f.Close()

	// Finalize should move it to outDir
	if err := asm.Finalize("movie.mkv"); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	// Should exist in output, not in work
	if _, err := os.Stat(filepath.Join(outDir, "movie.mkv")); err != nil {
		t.Error("file should exist in output dir")
	}
	if _, err := os.Stat(filepath.Join(workDir, "movie.mkv")); err == nil {
		t.Error("file should not exist in work dir after finalize")
	}
}

func TestCleanup(t *testing.T) {
	workDir := t.TempDir()
	outDir := t.TempDir()

	asm, err := New(workDir, outDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Create a subdirectory in workDir
	jobDir := filepath.Join(workDir, "my-download")
	_ = os.MkdirAll(jobDir, 0755)
	_ = os.WriteFile(filepath.Join(jobDir, "temp.bin"), []byte("data"), 0644)

	asm.Cleanup("my-download")

	if _, err := os.Stat(jobDir); err == nil {
		t.Error("cleanup should remove the job directory")
	}
}
