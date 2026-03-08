package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockHealthChecker struct{}

func (m *mockHealthChecker) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"alive"}`))
	}
}

func (m *mockHealthChecker) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ready"}`))
	}
}

func newTestServer(apiKey string) *Server {
	return NewServer(apiKey, &mockHealthChecker{}, nil)
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
	part.Write([]byte("<nzb>dummy</nzb>"))
	writer.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/nzb/upload", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", rec.Code)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["filename"] != "test.nzb" {
		t.Errorf("filename = %q, want test.nzb", body["filename"])
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
	json.NewDecoder(rec.Body).Decode(&body)
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
