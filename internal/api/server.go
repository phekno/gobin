package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Dependencies injected from main.
type Server struct {
	// These would be interfaces in production for testability
	// Queue   queue.ManagerInterface
	// Config  *config.Config
	apiKey   string
	health   HealthChecker
	staticFS fs.FS
	mux      *http.ServeMux
}

// HealthChecker is satisfied by health.Checker
type HealthChecker interface {
	LivenessHandler() http.HandlerFunc
	ReadinessHandler() http.HandlerFunc
}

func NewServer(apiKey string, health HealthChecker, staticFS fs.FS) *Server {
	s := &Server{
		apiKey:   apiKey,
		health:   health,
		staticFS: staticFS,
		mux:      http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// Wrap all API routes with auth middleware
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
	api.HandleFunc("DELETE /api/history/{id}", s.handleDeleteHistory)

	// NZB upload
	api.HandleFunc("POST /api/nzb/upload", s.handleNZBUpload)

	// Config
	api.HandleFunc("GET /api/config", s.handleGetConfig)
	api.HandleFunc("PUT /api/config", s.handleUpdateConfig)

	// Status / stats
	api.HandleFunc("GET /api/status", s.handleStatus)

	// SSE for live updates
	api.HandleFunc("GET /api/events", s.handleSSE)

	// Mount with auth
	s.mux.Handle("/api/", s.authMiddleware(api))

	// Health probes (unauthenticated — k8s needs access)
	s.mux.HandleFunc("/healthz", s.health.LivenessHandler())
	s.mux.HandleFunc("/readyz", s.health.ReadinessHandler())

	// Static web UI (served from embedded files)
	if s.staticFS != nil {
		s.mux.Handle("/", http.FileServer(http.FS(s.staticFS)))
	} else {
		s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<h1>GoBin</h1><p>Web UI not built. Run: make frontend</p>")
		})
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Request logging
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

// ListenAndServe starts the HTTP server.
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
		if s.apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Check header first, then query param
		key := r.Header.Get("X-Api-Key")
		if key == "" {
			key = r.URL.Query().Get("apikey")
		}
		// Also support Bearer token
		if key == "" {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				key = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		if key != s.apiKey {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "invalid or missing API key",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// --- Handlers (stubs — wire to real queue/config in production) ---

func (s *Server) handleGetQueue(w http.ResponseWriter, r *http.Request) {
	// TODO: return m.queue.List()
	writeJSON(w, http.StatusOK, map[string]any{
		"queue":    []any{},
		"paused":   false,
		"speed_bps": 0,
	})
}

func (s *Server) handleAddToQueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		URL      string `json:"url,omitempty"`      // Fetch NZB from URL
		Category string `json:"category,omitempty"`
		Priority int    `json:"priority,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// TODO: parse NZB, create job, add to queue
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "added",
		"id":     "placeholder-id",
	})
}

func (s *Server) handleRemoveFromQueue(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = id // TODO: m.queue.Remove(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (s *Server) handlePauseJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = id // TODO: m.queue.Pause(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (s *Server) handleResumeJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = id // TODO: m.queue.Resume(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

func (s *Server) handlePauseAll(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "queue paused"})
}

func (s *Server) handleResumeAll(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "queue resumed"})
}

func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"history": []any{}})
}

func (s *Server) handleDeleteHistory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleNZBUpload(w http.ResponseWriter, r *http.Request) {
	// Handle multipart file upload
	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32MB max
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid upload"})
		return
	}

	file, header, err := r.FormFile("nzbfile")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing nzbfile"})
		return
	}
	defer file.Close()

	_ = header // TODO: save to temp, parse NZB, create job
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":   "uploaded",
		"filename": header.Filename,
	})
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	// TODO: return sanitized config (redact passwords)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":     "0.1.0",
		"uptime_secs": 0,
		"speed_bps":   0,
		"queue_size":  0,
		"disk_free":   0,
		"paused":      false,
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

	// Send heartbeat until client disconnects
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// TODO: send real queue status updates
			fmt.Fprintf(w, "event: status\ndata: {\"speed_bps\": 0}\n\n")
			flusher.Flush()
		}
	}
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
