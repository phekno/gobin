package api

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/phekno/gobin/internal/config"
	"github.com/phekno/gobin/internal/nzb"
	"github.com/phekno/gobin/internal/queue"
	"github.com/phekno/gobin/internal/storage"
)

// Server handles API requests.
type Server struct {
	health    HealthChecker
	queue     *queue.Manager
	configMgr *config.Manager
	store     *storage.Store
	speed     *queue.SpeedTracker
	staticFS  fs.FS
	mux       *http.ServeMux
	startedAt time.Time
	version   string
}

// HealthChecker is satisfied by health.Checker
type HealthChecker interface {
	LivenessHandler() http.HandlerFunc
	ReadinessHandler() http.HandlerFunc
}

func NewServer(health HealthChecker, queueMgr *queue.Manager, cfgMgr *config.Manager, store *storage.Store, speed *queue.SpeedTracker, staticFS fs.FS, version string) *Server {
	s := &Server{
		health:    health,
		queue:     queueMgr,
		configMgr: cfgMgr,
		store:     store,
		speed:     speed,
		staticFS:  staticFS,
		mux:       http.NewServeMux(),
		startedAt: time.Now(),
		version:   version,
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	api := http.NewServeMux()

	// Queue management
	api.HandleFunc("GET /api/queue", s.handleGetQueue)
	api.HandleFunc("POST /api/queue", s.handleAddToQueue)
	api.HandleFunc("DELETE /api/queue/{id}", s.handleRemoveFromQueue)
	api.HandleFunc("POST /api/queue/{id}/pause", s.handlePauseJob)
	api.HandleFunc("POST /api/queue/{id}/resume", s.handleResumeJob)
	api.HandleFunc("POST /api/queue/pause", s.handlePauseAll)
	api.HandleFunc("POST /api/queue/resume", s.handleResumeAll)

	// History
	api.HandleFunc("GET /api/history", s.handleGetHistory)
	api.HandleFunc("DELETE /api/history", s.handleClearHistory)
	api.HandleFunc("DELETE /api/history/{id}", s.handleDeleteHistory)

	// NZB upload
	api.HandleFunc("POST /api/nzb/upload", s.handleNZBUpload)

	// Config
	api.HandleFunc("GET /api/config", s.handleGetConfig)
	api.HandleFunc("PUT /api/config", s.handleUpdateConfig)

	// API key management
	api.HandleFunc("GET /api/apikey", s.handleGetAPIKey)
	api.HandleFunc("POST /api/apikey/roll", s.handleRollAPIKey)

	// Status
	api.HandleFunc("GET /api/status", s.handleStatus)

	// Mount with auth
	s.mux.Handle("/api/", s.authMiddleware(api))

	// SSE — outside auth (EventSource can't set headers; read-only, same-origin)
	s.mux.HandleFunc("/api/events", s.handleSSE)

	// SABnzbd API compatibility (for Sonarr/Radarr/Lidarr)
	// Auth is handled inside the handler (SABnzbd uses ?apikey= param)
	s.mux.HandleFunc("/sabnzbd/api", s.handleSABnzbd)

	// Health probes (unauthenticated)
	s.mux.HandleFunc("/healthz", s.health.LivenessHandler())
	s.mux.HandleFunc("/readyz", s.health.ReadinessHandler())

	// Static web UI
	if s.staticFS != nil {
		s.mux.Handle("/", http.FileServer(http.FS(s.staticFS)))
	} else {
		s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, "<h1>GoBin</h1><p>Web UI not built. Run: make frontend</p>")
		})
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	wrapped := &responseWriter{ResponseWriter: w, status: 200}
	s.mux.ServeHTTP(wrapped, r)
	slog.Debug("http request",
		"method", r.Method,
		"path", r.URL.Path,
		"status", wrapped.status,
		"duration", time.Since(start),
	)
}

func (s *Server) ListenAndServe(addr string) error {
	srv := &http.Server{
		Addr:         addr,
		Handler:      s,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	slog.Info("api server starting", "addr", addr)
	return srv.ListenAndServe()
}

// --- Middleware ---

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := s.configMgr.Get()
		apiKey := cfg.API.APIKey
		fwdAuth := cfg.API.ForwardAuth

		// No auth configured at all
		if apiKey == "" && !fwdAuth.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Forward auth: trust headers set by reverse proxy (Authelia, Pocket ID, etc.)
		if fwdAuth.Enabled {
			user := r.Header.Get(fwdAuth.UserHeader)
			if user != "" {
				// Check group membership if configured
				if len(fwdAuth.AllowedGroups) > 0 {
					groups := r.Header.Get(fwdAuth.GroupsHeader)
					if !hasAllowedGroup(groups, fwdAuth.AllowedGroups) {
						writeJSON(w, http.StatusForbidden, map[string]string{
							"error": "user not in allowed groups",
						})
						return
					}
				}
				slog.Debug("forward auth", "user", user)
				next.ServeHTTP(w, r)
				return
			}
			// No user header — fall through to other auth methods
		}

		// Same-origin bypass for the embedded UI
		if referer := r.Header.Get("Referer"); referer != "" {
			if strings.HasPrefix(referer, "http://"+r.Host) || strings.HasPrefix(referer, "https://"+r.Host) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// API key check (for external clients: Radarr, Sonarr, curl, etc.)
		if apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}

		key := r.Header.Get("X-Api-Key")
		if key == "" {
			key = r.URL.Query().Get("apikey")
		}
		if key == "" {
			if auth, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
				key = auth
			}
		}

		if key != apiKey {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "invalid or missing API key",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func hasAllowedGroup(groupsHeader string, allowed []string) bool {
	for _, g := range strings.Split(groupsHeader, ",") {
		g = strings.TrimSpace(g)
		for _, a := range allowed {
			if strings.EqualFold(g, a) {
				return true
			}
		}
	}
	return false
}

// --- Job response DTO ---

type jobResponse struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	NZBPath         string  `json:"nzb_path,omitempty"`
	Category        string  `json:"category"`
	Priority        int     `json:"priority"`
	Status          string  `json:"status"`
	Progress        float64 `json:"progress"`
	AddedAt         string  `json:"added_at"`
	StartedAt       string  `json:"started_at,omitempty"`
	Error           string  `json:"error,omitempty"`
	TotalSegments   int     `json:"total_segments"`
	DoneSegments    int64   `json:"done_segments"`
	FailedSegments  int64   `json:"failed_segments"`
	TotalBytes      int64   `json:"total_bytes"`
	DownloadedBytes int64   `json:"downloaded_bytes"`
}

func jobToResponse(j *queue.Job) jobResponse {
	r := jobResponse{
		ID:              j.ID,
		Name:            j.Name,
		NZBPath:         j.NZBPath,
		Category:        j.Category,
		Priority:        j.Priority,
		Status:          j.Status.String(),
		Progress:        j.Progress(),
		AddedAt:         j.AddedAt.Format(time.RFC3339),
		Error:           j.Error,
		TotalSegments:   j.TotalSegments,
		DoneSegments:    j.DoneSegments.Load(),
		FailedSegments:  j.FailedSegments.Load(),
		TotalBytes:      j.TotalBytes,
		DownloadedBytes: j.DownloadedBytes.Load(),
	}
	if !j.StartedAt.IsZero() {
		r.StartedAt = j.StartedAt.Format(time.RFC3339)
	}
	return r
}

// --- Handlers ---

func (s *Server) handleGetQueue(w http.ResponseWriter, r *http.Request) {
	jobs := s.queue.List()
	resp := make([]jobResponse, len(jobs))
	for i, j := range jobs {
		resp[i] = jobToResponse(j)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"queue":  resp,
		"paused": s.queue.IsPaused(),
	})
}

func (s *Server) handleAddToQueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		URL      string `json:"url,omitempty"`
		Category string `json:"category,omitempty"`
		Priority int    `json:"priority,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	job := &queue.Job{
		ID:       GenerateID(),
		Name:     req.Name,
		Category: req.Category,
		Priority: req.Priority,
	}
	if err := s.addAndPersistJob(job); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status": "added",
		"id":     job.ID,
		"job":    jobToResponse(job),
	})
}

func (s *Server) handleRemoveFromQueue(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.queue.Remove(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	_ = s.store.DeleteJob(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed", "id": id})
}

func (s *Server) handlePauseJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.queue.Pause(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused", "id": id})
}

func (s *Server) handleResumeJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.queue.Resume(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed", "id": id})
}

func (s *Server) handlePauseAll(w http.ResponseWriter, r *http.Request) {
	s.queue.Pause("")
	writeJSON(w, http.StatusOK, map[string]string{"status": "queue paused"})
}

func (s *Server) handleResumeAll(w http.ResponseWriter, r *http.Request) {
	s.queue.Resume("")
	writeJSON(w, http.StatusOK, map[string]string{"status": "queue resumed"})
}

func (s *Server) handleGetHistory(w http.ResponseWriter, _ *http.Request) {
	entries, err := s.store.ListHistory(100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if entries == nil {
		entries = []*storage.HistoryEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": entries})
}

func (s *Server) handleClearHistory(w http.ResponseWriter, _ *http.Request) {
	if err := s.store.ClearHistory(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "history cleared"})
}

func (s *Server) handleDeleteHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.DeleteHistory(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleNZBUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid upload"})
		return
	}
	file, header, err := r.FormFile("nzbfile")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing nzbfile"})
		return
	}
	defer func() { _ = file.Close() }()

	// Read the entire NZB into memory so we can parse AND save it
	data, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reading upload: " + err.Error()})
		return
	}

	parsed, err := nzb.Parse(bytes.NewReader(data))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid NZB: " + err.Error()})
		return
	}

	name := header.Filename
	if title, ok := parsed.Meta["title"]; ok && title != "" {
		name = title
	}
	name = strings.TrimSuffix(name, ".nzb")

	// Save NZB file to disk
	nzbPath, err := s.saveNZB(name, data)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "saving NZB: " + err.Error()})
		return
	}

	job := &queue.Job{
		ID:            GenerateID(),
		Name:          name,
		NZBPath:       nzbPath,
		Category:      r.FormValue("category"),
		TotalSegments: parsed.TotalSegments(),
		TotalBytes:    parsed.TotalBytes(),
	}
	if err := s.addAndPersistJob(job); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":   "uploaded",
		"filename": header.Filename,
		"id":       job.ID,
		"job":      jobToResponse(job),
	})
}

func (s *Server) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	cfg := s.configMgr.Get()
	redacted := cfg.Redacted()
	writeJSON(w, http.StatusOK, map[string]any{
		"config": redacted,
		"path":   s.configMgr.FilePath(),
	})
}

func (s *Server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Config *config.Config `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Config == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config is required"})
		return
	}

	if err := s.configMgr.Update(req.Config); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "config saved and reloaded"})
}

func (s *Server) handleGetAPIKey(w http.ResponseWriter, _ *http.Request) {
	cfg := s.configMgr.Get()
	writeJSON(w, http.StatusOK, map[string]string{
		"api_key": cfg.API.APIKey,
	})
}

func (s *Server) handleRollAPIKey(w http.ResponseWriter, _ *http.Request) {
	cfg := s.configMgr.Get()

	// Deep copy so we don't mutate the live config directly
	newCfg := *cfg
	newCfg.API.APIKey = config.GenerateAPIKey()

	if err := s.configMgr.Update(&newCfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	slog.Info("API key rolled")
	writeJSON(w, http.StatusOK, map[string]string{
		"api_key": newCfg.API.APIKey,
		"status":  "API key regenerated",
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	jobs := s.queue.List()
	active := s.queue.ActiveJobs()
	var speedBps float64
	if s.speed != nil {
		speedBps = s.speed.BytesPerSecond()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":     s.version,
		"uptime_secs": int(time.Since(s.startedAt).Seconds()),
		"queue_size":  len(jobs),
		"active":      len(active),
		"paused":      s.queue.IsPaused(),
		"speed_bps":   int64(speedBps),
	})
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Disable reverse proxy buffering (nginx, Traefik, Cloudflare, etc.)
	w.Header().Set("X-Accel-Buffering", "no")

	sendUpdate := func() {
		jobs := s.queue.List()
		resp := make([]jobResponse, len(jobs))
		for i, j := range jobs {
			resp[i] = jobToResponse(j)
		}
		var speedBps float64
		if s.speed != nil {
			speedBps = s.speed.BytesPerSecond()
		}
		data, _ := json.Marshal(map[string]any{
			"queue":       resp,
			"paused":      s.queue.IsPaused(),
			"speed_bps":   int64(speedBps),
			"uptime_secs": int(time.Since(s.startedAt).Seconds()),
			"version":     s.version,
		})
		_, _ = fmt.Fprintf(w, "event: queue\ndata: %s\n\n", data)
		flusher.Flush()
	}

	// Send initial state immediately so the client doesn't wait for the first tick
	sendUpdate()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			sendUpdate()
		}
	}
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// saveNZB writes an NZB file to the watch/nzb directory for the engine to process.
func (s *Server) saveNZB(name string, data []byte) (string, error) {
	cfg := s.configMgr.Get()
	nzbDir := cfg.General.DownloadDir + "/nzb"
	if err := os.MkdirAll(nzbDir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(nzbDir, name+".nzb")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

// addAndPersistJob adds a job to the queue and persists it to storage.
func (s *Server) addAndPersistJob(job *queue.Job) error {
	if err := s.queue.Add(job); err != nil {
		return err
	}
	// Persist to storage so it survives restarts
	rec := &storage.JobRecord{
		ID:            job.ID,
		Name:          job.Name,
		NZBPath:       job.NZBPath,
		Category:      job.Category,
		Priority:      job.Priority,
		Status:        job.GetStatus().String(),
		AddedAt:       job.AddedAt,
		TotalSegments: job.TotalSegments,
		TotalBytes:    job.TotalBytes,
	}
	_ = s.store.SaveJob(rec)
	return nil
}

// GenerateID creates a random hex ID for jobs.
func GenerateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
