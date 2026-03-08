package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("New returned nil")
	}
}

func TestLivenessHandler_AlwaysOK(t *testing.T) {
	c := New()
	// Even with an unhealthy component, liveness should be OK
	c.Unhealthy("db", "down")

	rec := httptest.NewRecorder()
	c.LivenessHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != "alive" {
		t.Errorf("status = %q, want alive", body["status"])
	}
}

func TestReadinessHandler_AllHealthy(t *testing.T) {
	c := New()
	c.Healthy("db")
	c.Healthy("nntp")

	rec := httptest.NewRecorder()
	c.ReadinessHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var body map[string]any
	json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != "ready" {
		t.Errorf("status = %q, want ready", body["status"])
	}
	checks, ok := body["checks"].([]any)
	if !ok {
		t.Fatal("checks field missing or wrong type")
	}
	if len(checks) != 2 {
		t.Errorf("expected 2 checks, got %d", len(checks))
	}
}

func TestReadinessHandler_OneUnhealthy(t *testing.T) {
	c := New()
	c.Healthy("db")
	c.Unhealthy("nntp", "connection timeout")

	rec := httptest.NewRecorder()
	c.ReadinessHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}

	var body map[string]any
	json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != "not_ready" {
		t.Errorf("status = %q, want not_ready", body["status"])
	}
}

func TestReadinessHandler_DegradedIsNotUnhealthy(t *testing.T) {
	c := New()
	c.Degraded("disk", "low space")

	rec := httptest.NewRecorder()
	c.ReadinessHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))

	// Degraded != unhealthy, should still be 200
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (degraded is not unhealthy)", rec.Code)
	}
}

func TestReadinessHandler_NoChecks(t *testing.T) {
	c := New()

	rec := httptest.NewRecorder()
	c.ReadinessHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for no checks", rec.Code)
	}
}

func TestSetOverwritesPrevious(t *testing.T) {
	c := New()
	c.Unhealthy("db", "down")
	c.Healthy("db")

	rec := httptest.NewRecorder()
	c.ReadinessHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 after overwrite to healthy", rec.Code)
	}
}
