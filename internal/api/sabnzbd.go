package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/phekno/gobin/internal/nzb"
	"github.com/phekno/gobin/internal/queue"
)

// SABnzbd API compatibility layer.
// Implements the subset of the SABnzbd API that Sonarr/Radarr/Lidarr use
// to communicate with download clients.
//
// Endpoint: /sabnzbd/api?mode=...&apikey=...&output=json
// Also accessible at: /api?mode=...

// sabVersionCompat is the SABnzbd version reported to *arr apps.
// Radarr/Sonarr require >= 0.7.0; we report 4.0.0 to satisfy all checks.
const sabVersionCompat = "4.0.0"

func (s *Server) handleSABnzbd(w http.ResponseWriter, r *http.Request) {
	mode := r.FormValue("mode")

	// version and auth don't require API key
	switch mode {
	case "version":
		writeJSON(w, http.StatusOK, map[string]string{
			"version": sabVersionCompat,
		})
		return
	case "auth":
		writeJSON(w, http.StatusOK, map[string]string{
			"auth": "apikey",
		})
		return
	}

	// All other modes require API key
	cfg := s.configMgr.Get()
	apiKey := cfg.API.APIKey
	if apiKey != "" {
		key := r.FormValue("apikey")
		if key == "" {
			key = r.Header.Get("X-Api-Key")
		}
		if key != apiKey {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"status": false,
				"error":  "API Key Incorrect",
			})
			return
		}
	}

	switch mode {
	case "get_config":
		s.sabGetConfig(w, r)
	case "queue":
		name := r.FormValue("name")
		switch name {
		case "pause":
			s.sabPauseJob(w, r)
		case "resume":
			s.sabResumeJob(w, r)
		case "delete":
			s.sabDeleteJob(w, r)
		default:
			s.sabGetQueue(w, r)
		}
	case "history":
		name := r.FormValue("name")
		if name == "delete" {
			s.sabDeleteHistory(w, r)
		} else {
			s.sabGetHistory(w, r)
		}
	case "addurl":
		s.sabAddURL(w, r)
	case "addfile":
		s.sabAddFile(w, r)
	case "pause":
		s.queue.Pause("")
		writeJSON(w, http.StatusOK, map[string]any{"status": true})
	case "resume":
		s.queue.Resume("")
		writeJSON(w, http.StatusOK, map[string]any{"status": true})
	case "fullstatus", "status":
		s.sabStatus(w, r)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"status": false,
			"error":  fmt.Sprintf("unknown mode: %s", mode),
		})
	}
}

func (s *Server) sabGetConfig(w http.ResponseWriter, _ *http.Request) {
	cfg := s.configMgr.Get()

	categories := []map[string]any{
		{"name": "*", "dir": "", "priority": -100, "pp": -1, "script": "None"},
	}
	for _, cat := range cfg.Categories {
		categories = append(categories, map[string]any{
			"name":     cat.Name,
			"dir":      cat.Dir,
			"priority": -100,
			"pp":       3,
			"script":   "None",
		})
	}

	servers := make([]map[string]any, len(cfg.Servers))
	for i, srv := range cfg.Servers {
		servers[i] = map[string]any{
			"name":        srv.Name,
			"host":        srv.Host,
			"port":        srv.Port,
			"ssl":         boolToInt(srv.TLS),
			"connections": srv.Connections,
			"priority":    srv.Priority,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"config": map[string]any{
			"misc": map[string]any{
				"port":         cfg.API.Port,
				"complete_dir": cfg.General.CompleteDir,
				"download_dir": cfg.General.DownloadDir,
			},
			"categories": categories,
			"servers":    servers,
		},
	})
}

func (s *Server) sabGetQueue(w http.ResponseWriter, _ *http.Request) {
	jobs := s.queue.List()
	slots := make([]map[string]any, 0, len(jobs))

	for i, j := range jobs {
		if j.Status == queue.StatusCompleted || j.Status == queue.StatusFailed {
			continue
		}
		totalMB := float64(j.TotalBytes) / 1024 / 1024
		doneMB := float64(j.DownloadedBytes.Load()) / 1024 / 1024
		leftMB := totalMB - doneMB

		slots = append(slots, map[string]any{
			"nzo_id":     j.ID,
			"filename":   j.Name,
			"status":     sabStatus(j.Status),
			"percentage": int(j.Progress()),
			"mb":         fmt.Sprintf("%.1f", totalMB),
			"mbleft":     fmt.Sprintf("%.1f", leftMB),
			"size":       formatSize(j.TotalBytes),
			"sizeleft":   formatSize(j.TotalBytes - j.DownloadedBytes.Load()),
			"priority":   sabPriority(j.Priority),
			"cat":        j.Category,
			"timeleft":   "00:00:00",
			"eta":        "unknown",
			"index":      i,
			"avg_age":    "0d",
			"script":     "Default",
			"unpackopts": 3,
			"msgid":      "",
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"queue": map[string]any{
			"status":          sabGlobalStatus(s.queue.IsPaused()),
			"paused":          s.queue.IsPaused(),
			"noofslots":       len(slots),
			"noofslots_total": len(slots),
			"speed":           "0",
			"kbpersec":        "0.00",
			"speedlimit":      0,
			"speedlimit_abs":  "",
			"eta":             "unknown",
			"timeleft":        "00:00:00",
			"mb":              "0.0",
			"mbleft":          "0.0",
			"diskspace1":      "0",
			"diskspace2":      "0",
			"version":         sabVersionCompat,
			"slots":           slots,
		},
	})
}

func (s *Server) sabGetHistory(w http.ResponseWriter, _ *http.Request) {
	entries, err := s.store.ListHistory(100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"status": false, "error": err.Error()})
		return
	}

	slots := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		slots = append(slots, map[string]any{
			"nzo_id":   e.ID,
			"name":     e.Name,
			"nzbname":  e.Name,
			"status":   capitalizeFirst(e.Status),
			"category": e.Category,
			"size":     e.TotalBytes / 1024 / 1024, // MB
			"downloaded": e.DownloadedBytes / 1024 / 1024,
			"completed":  e.CompletedAt.Unix(),
			"fail_message": e.Error,
			"download_time": int(e.CompletedAt.Sub(e.StartedAt).Seconds()),
			"stage_log": []any{},
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"history": map[string]any{
			"noofslots":       len(slots),
			"noofslots_total": s.store.CountHistory(),
			"slots":           slots,
		},
	})
}

func (s *Server) sabAddURL(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"status": false,
			"error":  "name parameter required",
		})
		return
	}

	nzbName := r.FormValue("nzbname")
	if nzbName == "" {
		nzbName = name
	}

	job := &queue.Job{
		ID:       GenerateID(),
		Name:     nzbName,
		Category: r.FormValue("cat"),
		Priority: sabPriorityToInt(r.FormValue("priority")),
	}

	if err := s.addAndPersistJob(job); err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"status": false,
			"error":  err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  true,
		"nzo_ids": []string{job.ID},
	})
}

func (s *Server) sabAddFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"status": false,
			"error":  "invalid upload",
		})
		return
	}

	file, header, err := r.FormFile("nzbfile")
	if err != nil {
		file, header, err = r.FormFile("name")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"status": false,
				"error":  "missing nzbfile",
			})
			return
		}
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"status": false,
			"error":  "reading upload: " + err.Error(),
		})
		return
	}

	parsed, err := nzb.Parse(bytes.NewReader(data))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"status": false,
			"error":  "invalid NZB: " + err.Error(),
		})
		return
	}

	name := r.FormValue("nzbname")
	if name == "" {
		if title, ok := parsed.Meta["title"]; ok && title != "" {
			name = title
		} else {
			name = strings.TrimSuffix(header.Filename, ".nzb")
		}
	}

	nzbPath, err := s.saveNZB(name, data)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"status": false,
			"error":  "saving NZB: " + err.Error(),
		})
		return
	}

	job := &queue.Job{
		ID:            GenerateID(),
		Name:          name,
		NZBPath:       nzbPath,
		Category:      r.FormValue("cat"),
		Priority:      sabPriorityToInt(r.FormValue("priority")),
		TotalSegments: parsed.TotalSegments(),
		TotalBytes:    parsed.TotalBytes(),
	}

	if err := s.addAndPersistJob(job); err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"status": false,
			"error":  err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  true,
		"nzo_ids": []string{job.ID},
	})
}

func (s *Server) sabPauseJob(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("value")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": false, "error": "value required"})
		return
	}
	s.queue.Pause(id)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  true,
		"nzo_ids": []string{id},
	})
}

func (s *Server) sabResumeJob(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("value")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": false, "error": "value required"})
		return
	}
	s.queue.Resume(id)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  true,
		"nzo_ids": []string{id},
	})
}

func (s *Server) sabDeleteJob(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("value")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": false, "error": "value required"})
		return
	}
	if err := s.queue.Remove(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"status": false, "error": err.Error()})
		return
	}
	_ = s.store.DeleteJob(id)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  true,
		"nzo_ids": []string{id},
	})
}

func (s *Server) sabDeleteHistory(w http.ResponseWriter, _ *http.Request) {
	// History not yet implemented
	writeJSON(w, http.StatusOK, map[string]any{"status": true})
}

func (s *Server) sabStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     sabGlobalStatus(s.queue.IsPaused()),
		"paused":     s.queue.IsPaused(),
		"speed":      "0",
		"speedlimit": 0,
		"uptime":     fmt.Sprintf("%ds", int(time.Since(s.startedAt).Seconds())),
		"version":    sabVersionCompat,
		"diskspace1": "0",
		"diskspace2": "0",
	})
}

// --- SABnzbd format helpers ---

func sabStatus(s queue.Status) string {
	switch s {
	case queue.StatusDownloading:
		return "Downloading"
	case queue.StatusQueued:
		return "Queued"
	case queue.StatusPaused:
		return "Paused"
	case queue.StatusPostProcessing:
		return "Extracting"
	case queue.StatusCompleted:
		return "Completed"
	case queue.StatusFailed:
		return "Failed"
	default:
		return "Queued"
	}
}

func sabGlobalStatus(paused bool) string {
	if paused {
		return "Paused"
	}
	return "Idle"
}

func sabPriority(p int) string {
	switch {
	case p >= 2:
		return "Force"
	case p == 1:
		return "High"
	case p == 0:
		return "Normal"
	case p == -1:
		return "Low"
	default:
		return "Normal"
	}
}

func sabPriorityToInt(s string) int {
	switch s {
	case "2":
		return 2
	case "1":
		return 1
	case "0", "":
		return 0
	case "-1":
		return -1
	case "-2":
		return 0 // Paused priority → normal, handled via status
	default:
		return 0
	}
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func formatSize(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	sizes := []string{"KB", "MB", "GB", "TB"}
	exp := 0
	val := float64(bytes) / float64(unit)
	for val >= float64(unit) && exp < len(sizes)-1 {
		val /= float64(unit)
		exp++
	}
	return fmt.Sprintf("%.1f %s", val, sizes[exp])
}
