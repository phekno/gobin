package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/phekno/gobin/internal/config"
	"github.com/phekno/gobin/internal/queue"
)

func newSABTestServer(t *testing.T, apiKey string) *Server {
	t.Helper()
	cfgMgr := testConfigMgr(t, &config.Config{
		API: config.API{APIKey: apiKey, Port: 8080},
		Categories: []config.Category{
			{Name: "tv", Dir: "TV"},
			{Name: "movies", Dir: "Movies"},
		},
		Servers: []config.Server{
			{Name: "primary", Host: "news.example.com", Port: 563, TLS: true, Connections: 10},
		},
	})
	return NewServer(&mockHealthChecker{}, queue.NewManager(3), cfgMgr, testStore(t), nil, nil, "0.1.0")
}

func TestSAB_Version_NoAuth(t *testing.T) {
	srv := newSABTestServer(t, "secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/sabnzbd/api?mode=version&output=json", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["version"] != "0.1.0" {
		t.Errorf("version = %q, want 0.1.0", body["version"])
	}
}

func TestSAB_Auth_Required(t *testing.T) {
	srv := newSABTestServer(t, "secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/sabnzbd/api?mode=queue&output=json", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestSAB_Auth_ValidKey(t *testing.T) {
	srv := newSABTestServer(t, "secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/sabnzbd/api?mode=queue&apikey=secret&output=json", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestSAB_GetQueue_Empty(t *testing.T) {
	srv := newSABTestServer(t, "")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/sabnzbd/api?mode=queue&output=json", nil)
	srv.ServeHTTP(rec, req)

	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	q, ok := body["queue"].(map[string]any)
	if !ok {
		t.Fatal("missing queue object")
	}
	if q["paused"] != false {
		t.Error("queue should not be paused")
	}
	slots, ok := q["slots"].([]any)
	if !ok {
		t.Fatal("missing slots array")
	}
	if len(slots) != 0 {
		t.Errorf("expected 0 slots, got %d", len(slots))
	}
}

func TestSAB_AddURL(t *testing.T) {
	srv := newSABTestServer(t, "")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/sabnzbd/api?mode=addurl&name=http://example.com/test.nzb&cat=tv&output=json", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != true {
		t.Error("status should be true")
	}
	ids, ok := body["nzo_ids"].([]any)
	if !ok || len(ids) == 0 {
		t.Error("should return nzo_ids")
	}
}

func TestSAB_QueuePauseResume(t *testing.T) {
	srv := newSABTestServer(t, "")

	// Add a job first
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/sabnzbd/api?mode=addurl&name=test.nzb&output=json", nil)
	srv.ServeHTTP(rec, req)

	var addBody map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&addBody)
	ids := addBody["nzo_ids"].([]any)
	jobID := ids[0].(string)

	// Pause the job
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/sabnzbd/api?mode=queue&name=pause&value="+jobID+"&output=json", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("pause: status = %d", rec.Code)
	}

	// Resume the job
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/sabnzbd/api?mode=queue&name=resume&value="+jobID+"&output=json", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("resume: status = %d", rec.Code)
	}
}

func TestSAB_GlobalPauseResume(t *testing.T) {
	srv := newSABTestServer(t, "")

	// Pause
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/sabnzbd/api?mode=pause&output=json", nil)
	srv.ServeHTTP(rec, req)
	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != true {
		t.Error("pause should return status true")
	}

	// Verify paused
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/sabnzbd/api?mode=queue&output=json", nil)
	srv.ServeHTTP(rec, req)
	_ = json.NewDecoder(rec.Body).Decode(&body)
	q := body["queue"].(map[string]any)
	if q["paused"] != true {
		t.Error("queue should be paused")
	}

	// Resume
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/sabnzbd/api?mode=resume&output=json", nil)
	srv.ServeHTTP(rec, req)

	// Verify resumed
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/sabnzbd/api?mode=queue&output=json", nil)
	srv.ServeHTTP(rec, req)
	_ = json.NewDecoder(rec.Body).Decode(&body)
	q = body["queue"].(map[string]any)
	if q["paused"] != false {
		t.Error("queue should be resumed")
	}
}

func TestSAB_DeleteJob(t *testing.T) {
	srv := newSABTestServer(t, "")

	// Add a job
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/sabnzbd/api?mode=addurl&name=test.nzb&output=json", nil)
	srv.ServeHTTP(rec, req)
	var addBody map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&addBody)
	jobID := addBody["nzo_ids"].([]any)[0].(string)

	// Delete it
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/sabnzbd/api?mode=queue&name=delete&value="+jobID+"&output=json", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: status = %d", rec.Code)
	}

	// Verify empty
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/sabnzbd/api?mode=queue&output=json", nil)
	srv.ServeHTTP(rec, req)
	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	q := body["queue"].(map[string]any)
	slots := q["slots"].([]any)
	if len(slots) != 0 {
		t.Errorf("expected 0 slots after delete, got %d", len(slots))
	}
}

func TestSAB_GetConfig(t *testing.T) {
	srv := newSABTestServer(t, "")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/sabnzbd/api?mode=get_config&output=json", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	cfg, ok := body["config"].(map[string]any)
	if !ok {
		t.Fatal("missing config object")
	}
	cats, ok := cfg["categories"].([]any)
	if !ok {
		t.Fatal("missing categories")
	}
	// Default "*" + tv + movies = 3
	if len(cats) != 3 {
		t.Errorf("expected 3 categories, got %d", len(cats))
	}
}

func TestSAB_History_Empty(t *testing.T) {
	srv := newSABTestServer(t, "")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/sabnzbd/api?mode=history&output=json", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	h := body["history"].(map[string]any)
	slots := h["slots"].([]any)
	if len(slots) != 0 {
		t.Errorf("expected 0 history slots, got %d", len(slots))
	}
}
