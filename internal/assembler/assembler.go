// Package assembler writes decoded yEnc segments to files on disk.
package assembler

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// Assembler writes decoded segment data to files in a working directory,
// then moves completed files to the output directory.
type Assembler struct {
	workDir   string // incomplete downloads
	outputDir string // completed files
}

// New creates an assembler with the given directories.
func New(workDir, outputDir string) (*Assembler, error) {
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("creating work dir: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}
	return &Assembler{workDir: workDir, outputDir: outputDir}, nil
}

// CreateFile opens a file in the work directory for writing segments.
// Returns the file handle. Caller is responsible for closing it.
func (a *Assembler) CreateFile(filename string) (*os.File, error) {
	path := filepath.Join(a.workDir, filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("creating parent dir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("creating file %s: %w", filename, err)
	}
	return f, nil
}

// WriteSegment appends decoded data to the given file handle.
func (a *Assembler) WriteSegment(f *os.File, data []byte) error {
	_, err := f.Write(data)
	return err
}

// Finalize closes the file and moves it from the work directory to the output directory.
func (a *Assembler) Finalize(filename string) error {
	src := filepath.Join(a.workDir, filename)
	dst := filepath.Join(a.outputDir, filename)

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("moving %s to output: %w", filename, err)
	}

	slog.Info("file completed", "filename", filename, "path", dst)
	return nil
}

// Cleanup removes the job's working files.
func (a *Assembler) Cleanup(jobName string) {
	dir := filepath.Join(a.workDir, jobName)
	_ = os.RemoveAll(dir)
}
