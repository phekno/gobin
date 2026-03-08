package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Checker tracks the health of various subsystems.
type Checker struct {
	mu     sync.RWMutex
	checks map[string]Check
}

// Check represents the status of a single health dependency.
type Check struct {
	Name    string    `json:"name"`
	Status  string    `json:"status"` // "healthy", "degraded", "unhealthy"
	Message string    `json:"message,omitempty"`
	Updated time.Time `json:"updated_at"`
}

func New() *Checker {
	return &Checker{
		checks: make(map[string]Check),
	}
}

// Set updates the health status of a named component.
func (h *Checker) Set(name, status, message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks[name] = Check{
		Name:    name,
		Status:  status,
		Message: message,
		Updated: time.Now(),
	}
}

// Healthy marks a component as healthy.
func (h *Checker) Healthy(name string) {
	h.Set(name, "healthy", "")
}

// Unhealthy marks a component as unhealthy.
func (h *Checker) Unhealthy(name, reason string) {
	h.Set(name, "unhealthy", reason)
}

// Degraded marks a component as degraded (working but impaired).
func (h *Checker) Degraded(name, reason string) {
	h.Set(name, "degraded", reason)
}

// LivenessHandler returns HTTP 200 if the process is alive.
// Used by k8s livenessProbe — should only fail if the process is wedged.
func (h *Checker) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Liveness is simple: if we can respond, we're alive
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "alive"})
	}
}

// ReadinessHandler returns HTTP 200 if the app is ready to serve traffic.
// Used by k8s readinessProbe — fails if critical dependencies are down.
func (h *Checker) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.mu.RLock()
		defer h.mu.RUnlock()

		allHealthy := true
		checks := make([]Check, 0, len(h.checks))
		for _, c := range h.checks {
			checks = append(checks, c)
			if c.Status == "unhealthy" {
				allHealthy = false
			}
		}

		status := http.StatusOK
		overall := "ready"
		if !allHealthy {
			status = http.StatusServiceUnavailable
			overall = "not_ready"
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]any{
			"status": overall,
			"checks": checks,
		})
	}
}

// StartPeriodicChecks runs health checks on a timer.
func (h *Checker) StartPeriodicChecks(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.runChecks(ctx)
		}
	}
}

func (h *Checker) runChecks(ctx context.Context) {
	// Placeholder: real implementation would check NNTP connectivity,
	// disk space, database connectivity, etc.
	//
	// Example:
	//   if pool.ActiveConnections() > 0 {
	//       h.Healthy("nntp")
	//   } else {
	//       h.Unhealthy("nntp", "no active connections")
	//   }
}
