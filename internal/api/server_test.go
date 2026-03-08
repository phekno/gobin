package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phekno/gobin/internal/config"
	"github.com/phekno/gobin/internal/queue"
)

type mockHealthChecker struct{}

func (m *mockHealthChecker) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status":"alive"}`))
	}
}

func (m *mockHealthChecker) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}
}

func testConfigMgr(t *testing.T, cfg *config.Config) *config.Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	return config.NewManager(path, cfg)
}

func newTestServer(apiKey string) *Server {
	cfgMgr := config.NewManager("/tmp/test-config.yaml", &config.Config{
		API: config.API{APIKey: apiKey, Port: 8080},
	})
	return NewServer(&mockHealthChecker{}, queue.NewManager(3), cfgMgr, nil, "test")
}

func TestAuthMiddleware_NoAPIKey(t *testing.T) {
	srv := newTestServer("")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/queue", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Error("should not require auth when apiKey is empty")
	}
}

func TestAuthMiddleware_ValidHeaderKey(t *testing.T) {
	srv := newTestServer("secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/queue", nil)
	req.Header.Set("X-Api-Key", "secret")
	srv.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Errorf("valid X-Api-Key should pass auth, got %d", rec.Code)
	}
}

func TestAuthMiddleware_ValidQueryParam(t *testing.T) {
	srv := newTestServer("secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/queue?apikey=secret", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Errorf("valid apikey query param should pass auth, got %d", rec.Code)
	}
}

func TestAuthMiddleware_ValidBearerToken(t *testing.T) {
	srv := newTestServer("secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/queue", nil)
	req.Header.Set("Authorization", "Bearer secret")
	srv.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Errorf("valid Bearer token should pass auth, got %d", rec.Code)
	}
}

func TestAuthMiddleware_MissingKey(t *testing.T) {
	srv := newTestServer("secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/queue", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("missing key should return 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_WrongKey(t *testing.T) {
	srv := newTestServer("secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/queue", nil)
	req.Header.Set("X-Api-Key", "wrong")
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong key should return 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_SameOriginBypass(t *testing.T) {
	srv := newTestServer("secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/queue", nil)
	req.Host = "localhost:8080"
	req.Header.Set("Referer", "http://localhost:8080/")
	// No API key — should still pass due to same-origin Referer
	srv.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Error("same-origin request should bypass auth")
	}
}

func TestAuthMiddleware_CrossOriginBlocked(t *testing.T) {
	srv := newTestServer("secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/queue", nil)
	req.Host = "localhost:8080"
	req.Header.Set("Referer", "http://evil.com/")
	// No API key and cross-origin Referer
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("cross-origin request without key should return 401, got %d", rec.Code)
	}
}

func TestHealthEndpoints_NoAuth(t *testing.T) {
	srv := newTestServer("secret")

	for _, path := range []string{"/healthz", "/readyz"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", path, nil)
		// No auth headers
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("%s without auth: status = %d, want 200", path, rec.Code)
		}
	}
}

func TestHandleGetQueue(t *testing.T) {
	srv := newTestServer("")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/queue", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if _, ok := body["queue"]; !ok {
		t.Error("response missing 'queue' field")
	}
}

func TestHandleAddToQueue_ValidJSON(t *testing.T) {
	srv := newTestServer("")
	body := `{"name":"test.nzb","url":"http://example.com/test.nzb"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/queue", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", rec.Code)
	}
}

func TestHandleAddToQueue_InvalidJSON(t *testing.T) {
	srv := newTestServer("")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/queue", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleNZBUpload_MissingFile(t *testing.T) {
	srv := newTestServer("")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/nzb/upload", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for missing upload", rec.Code)
	}
}

func TestHandleNZBUpload_ValidFile(t *testing.T) {
	srv := newTestServer("")

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("nzbfile", "test.nzb")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte(`<?xml version="1.0"?>
<nzb xmlns="http://www.newzbin.com/DTD/2003/nzb">
  <head><meta type="title">Test Upload</meta></head>
  <file poster="u@e" date="1000" subject="test">
    <groups><group>alt.test</group></groups>
    <segments><segment bytes="100" number="1">a@b</segment></segments>
  </file>
</nzb>`))
	_ = writer.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/nzb/upload", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", rec.Code)
	}

	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["filename"] != "test.nzb" {
		t.Errorf("filename = %v, want test.nzb", body["filename"])
	}
	if body["id"] == nil || body["id"] == "" {
		t.Error("response missing job id")
	}
}

func TestHandleStatus(t *testing.T) {
	srv := newTestServer("")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/status", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if _, ok := body["version"]; !ok {
		t.Error("response missing 'version' field")
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]string{"key": "value"})

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["key"] != "value" {
		t.Errorf("key = %q, want value", body["key"])
	}
}

func TestResponseWriter_CapturesStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, status: 200}

	rw.WriteHeader(404)
	if rw.status != 404 {
		t.Errorf("status = %d, want 404", rw.status)
	}
}

// --- Forward Auth Tests ---

func TestAuthMiddleware_ForwardAuth_ValidUser(t *testing.T) {
	cfgMgr := testConfigMgr(t, &config.Config{
		API: config.API{
			APIKey:      "secret",
			ForwardAuth: config.ForwardAuth{Enabled: true, UserHeader: "Remote-User"},
		},
	})
	srv := NewServer(&mockHealthChecker{}, queue.NewManager(3), cfgMgr, nil, "test")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/queue", nil)
	req.Header.Set("Remote-User", "testuser")
	srv.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Errorf("forward auth with valid user should pass, got %d", rec.Code)
	}
}

func TestAuthMiddleware_ForwardAuth_NoUserHeader(t *testing.T) {
	cfgMgr := testConfigMgr(t, &config.Config{
		API: config.API{
			APIKey:      "secret",
			ForwardAuth: config.ForwardAuth{Enabled: true, UserHeader: "Remote-User"},
		},
	})
	srv := NewServer(&mockHealthChecker{}, queue.NewManager(3), cfgMgr, nil, "test")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/queue", nil)
	// No Remote-User header, no API key
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no user header and no API key should return 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_ForwardAuth_GroupAllowed(t *testing.T) {
	cfgMgr := testConfigMgr(t, &config.Config{
		API: config.API{
			APIKey: "secret",
			ForwardAuth: config.ForwardAuth{
				Enabled:       true,
				UserHeader:    "Remote-User",
				GroupsHeader:  "Remote-Groups",
				AllowedGroups: []string{"admin"},
			},
		},
	})
	srv := NewServer(&mockHealthChecker{}, queue.NewManager(3), cfgMgr, nil, "test")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/queue", nil)
	req.Header.Set("Remote-User", "testuser")
	req.Header.Set("Remote-Groups", "users,admin")
	srv.ServeHTTP(rec, req)

	if rec.Code == http.StatusForbidden {
		t.Error("user in allowed group should pass")
	}
}

func TestAuthMiddleware_ForwardAuth_GroupDenied(t *testing.T) {
	cfgMgr := testConfigMgr(t, &config.Config{
		API: config.API{
			APIKey: "secret",
			ForwardAuth: config.ForwardAuth{
				Enabled:       true,
				UserHeader:    "Remote-User",
				GroupsHeader:  "Remote-Groups",
				AllowedGroups: []string{"admin"},
			},
		},
	})
	srv := NewServer(&mockHealthChecker{}, queue.NewManager(3), cfgMgr, nil, "test")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/queue", nil)
	req.Header.Set("Remote-User", "testuser")
	req.Header.Set("Remote-Groups", "users")
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("user not in allowed group should get 403, got %d", rec.Code)
	}
}

func TestAuthMiddleware_ForwardAuth_FallbackToAPIKey(t *testing.T) {
	cfgMgr := testConfigMgr(t, &config.Config{
		API: config.API{
			APIKey:      "secret",
			ForwardAuth: config.ForwardAuth{Enabled: true, UserHeader: "Remote-User"},
		},
	})
	srv := NewServer(&mockHealthChecker{}, queue.NewManager(3), cfgMgr, nil, "test")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/queue", nil)
	// No forward auth header, but valid API key
	req.Header.Set("X-Api-Key", "secret")
	srv.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Error("valid API key should work as fallback when forward auth has no user header")
	}
}

// --- Config Editor Tests ---

func TestHandleGetConfig_ReturnsRedactedYAML(t *testing.T) {
	cfgMgr := testConfigMgr(t, &config.Config{
		API: config.API{APIKey: "super-secret"},
		Servers: []config.Server{
			{Name: "test", Host: "news.example.com", Port: 563, Password: "mypass"},
		},
	})
	srv := NewServer(&mockHealthChecker{}, queue.NewManager(3), cfgMgr, nil, "test")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/config", nil)
	req.Header.Set("X-Api-Key", "super-secret")
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	yamlStr, ok := body["config_yaml"].(string)
	if !ok || yamlStr == "" {
		t.Fatal("response missing config_yaml")
	}
	if !strings.Contains(yamlStr, "********") {
		t.Error("config_yaml should contain redacted passwords")
	}
	if strings.Contains(yamlStr, "mypass") {
		t.Error("config_yaml should NOT contain real passwords")
	}
	if strings.Contains(yamlStr, "super-secret") {
		t.Error("config_yaml should NOT contain real API key")
	}
}

func TestHandleUpdateConfig_ValidYAML(t *testing.T) {
	cfgMgr := testConfigMgr(t, &config.Config{
		API: config.API{Port: 8080},
		Servers: []config.Server{
			{Name: "test", Host: "news.example.com", Port: 563, Password: "original"},
		},
	})
	srv := NewServer(&mockHealthChecker{}, queue.NewManager(3), cfgMgr, nil, "test")

	newYAML := `
servers:
  - name: updated
    host: news.updated.com
    port: 563
    password: "newpass"
api:
  port: 9999
`
	rec := httptest.NewRecorder()
	body := map[string]string{"config_yaml": newYAML}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/config", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		var errBody map[string]string
		_ = json.NewDecoder(rec.Body).Decode(&errBody)
		t.Fatalf("status = %d, want 200: %s", rec.Code, errBody["error"])
	}

	// Verify the config was updated
	updated := cfgMgr.Get()
	if updated.Servers[0].Host != "news.updated.com" {
		t.Errorf("server host = %q, want news.updated.com", updated.Servers[0].Host)
	}
}

func TestHandleUpdateConfig_InvalidYAML(t *testing.T) {
	cfgMgr := testConfigMgr(t, &config.Config{API: config.API{Port: 8080}})
	srv := NewServer(&mockHealthChecker{}, queue.NewManager(3), cfgMgr, nil, "test")

	rec := httptest.NewRecorder()
	body := map[string]string{"config_yaml": ": invalid: yaml: ["}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/config", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid YAML should return 400, got %d", rec.Code)
	}
}

func TestHandleUpdateConfig_PreservesRedactedSecrets(t *testing.T) {
	cfgMgr := testConfigMgr(t, &config.Config{
		API: config.API{APIKey: "real-secret", Port: 8080},
		Servers: []config.Server{
			{Name: "test", Host: "news.example.com", Port: 563, Password: "real-password"},
		},
	})
	srv := NewServer(&mockHealthChecker{}, queue.NewManager(3), cfgMgr, nil, "test")

	editedYAML := `
servers:
  - name: test
    host: news.example.com
    port: 563
    password: "********"
api:
  api_key: "********"
  port: 8080
`
	rec := httptest.NewRecorder()
	body := map[string]string{"config_yaml": editedYAML}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/config", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "real-secret")
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		var errBody map[string]string
		_ = json.NewDecoder(rec.Body).Decode(&errBody)
		t.Fatalf("status = %d: %s", rec.Code, errBody["error"])
	}

	// Verify secrets were preserved
	updated := cfgMgr.Get()
	if updated.Servers[0].Password != "real-password" {
		t.Errorf("password = %q, want real-password (should be preserved)", updated.Servers[0].Password)
	}
	if updated.API.APIKey != "real-secret" {
		t.Errorf("api_key = %q, want real-secret (should be preserved)", updated.API.APIKey)
	}
}
