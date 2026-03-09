// Package postprocess handles post-download processing:
// PAR2 verify/repair → archive extraction → cleanup.
package postprocess

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/phekno/gobin/internal/config"
	"github.com/phekno/gobin/internal/logging"
)

// Runner executes the post-processing pipeline on a completed download.
type Runner struct {
	cfg config.PostProcess
}

// New creates a post-processing runner.
func New(cfg config.PostProcess) *Runner {
	return &Runner{cfg: cfg}
}

// Result contains the outcome of post-processing.
type Result struct {
	Repaired  bool
	Extracted bool
	Error     error
}

// Run executes the full pipeline on the given directory.
// The directory should contain the downloaded files.
func (r *Runner) Run(logger *slog.Logger, jobID, dir string) Result {
	result := Result{}

	// Stage 1: PAR2 verify and repair
	if r.cfg.Par2Enabled {
		logging.LogPostProcess(logger, jobID, "par2", "starting")
		repaired, err := r.par2Repair(dir)
		if err != nil {
			logger.Warn("par2 repair failed", "error", err)
			logging.LogPostProcess(logger, jobID, "par2", "failed")
			// Don't abort — files might still be usable
		} else {
			result.Repaired = repaired
			if repaired {
				logging.LogPostProcess(logger, jobID, "par2", "repaired")
			} else {
				logging.LogPostProcess(logger, jobID, "par2", "verified")
			}
		}
	}

	// Stage 2: Extract archives
	if r.cfg.UnpackEnabled {
		logging.LogPostProcess(logger, jobID, "unpack", "starting")
		extracted, err := r.extract(dir)
		if err != nil {
			logger.Error("extraction failed", "error", err)
			logging.LogPostProcess(logger, jobID, "unpack", "failed")
			result.Error = fmt.Errorf("extraction failed: %w", err)
			return result
		}
		result.Extracted = extracted
		if extracted {
			logging.LogPostProcess(logger, jobID, "unpack", "completed")
		} else {
			logging.LogPostProcess(logger, jobID, "unpack", "no archives found")
		}
	}

	// Stage 3: Cleanup archive files after successful extraction
	if r.cfg.CleanupAfterUnpack && result.Extracted {
		logging.LogPostProcess(logger, jobID, "cleanup", "starting")
		r.cleanup(dir)
		logging.LogPostProcess(logger, jobID, "cleanup", "completed")
	}

	return result
}

// par2Repair finds PAR2 files and runs par2 verify/repair.
func (r *Runner) par2Repair(dir string) (bool, error) {
	par2Path := r.cfg.Par2Path
	if par2Path == "" {
		par2Path = "par2"
	}

	// Find the main .par2 file (not the .vol ones)
	par2Files, err := filepath.Glob(filepath.Join(dir, "*.par2"))
	if err != nil || len(par2Files) == 0 {
		return false, nil // No PAR2 files, nothing to do
	}

	// Find the base par2 file (shortest name, no .volXX+XX)
	var basePar2 string
	for _, f := range par2Files {
		name := strings.ToLower(filepath.Base(f))
		if !strings.Contains(name, ".vol") {
			basePar2 = f
			break
		}
	}
	if basePar2 == "" {
		basePar2 = par2Files[0]
	}

	// First try verify
	cmd := exec.Command(par2Path, "verify", basePar2)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()

	if err == nil {
		slog.Debug("par2 verify passed", "file", basePar2)
		return false, nil
	}

	// Verify failed — try repair
	slog.Info("par2 verify failed, attempting repair", "output", string(output))

	cmd = exec.Command(par2Path, "repair", basePar2)
	cmd.Dir = dir
	output, err = cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("par2 repair failed: %s: %w", string(output), err)
	}

	slog.Info("par2 repair succeeded")
	return true, nil
}

// extract finds and extracts archive files using 7z.
func (r *Runner) extract(dir string) (bool, error) {
	sevenzPath := r.cfg.SevenzPath
	if sevenzPath == "" {
		sevenzPath = "7z"
	}

	// Look for archives: .rar, .7z, .zip
	patterns := []string{"*.rar", "*.7z", "*.zip"}
	var archives []string
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(filepath.Join(dir, pattern))
		archives = append(archives, matches...)
	}

	// Also check for split RAR files (.r00, .r01, etc.) — only extract the .rar
	// 7z handles this automatically when given the .rar file

	if len(archives) == 0 {
		return false, nil
	}

	// Extract each archive
	for _, archive := range archives {
		slog.Info("extracting", "archive", filepath.Base(archive))

		// 7z x = extract with full paths, -y = assume yes, -o = output dir
		cmd := exec.Command(sevenzPath, "x", "-y", "-o"+dir, archive)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return false, fmt.Errorf("extracting %s: %s: %w", filepath.Base(archive), string(output), err)
		}
	}

	return true, nil
}

// cleanup removes archive and parity files after successful extraction.
func (r *Runner) cleanup(dir string) {
	cleanupPatterns := []string{
		"*.rar", "*.r[0-9][0-9]", "*.r[0-9][0-9][0-9]",
		"*.par2",
		"*.7z", "*.zip",
	}

	for _, pattern := range cleanupPatterns {
		matches, _ := filepath.Glob(filepath.Join(dir, pattern))
		for _, f := range matches {
			slog.Debug("cleanup removing", "file", filepath.Base(f))
			_ = os.Remove(f)
		}
	}
}
