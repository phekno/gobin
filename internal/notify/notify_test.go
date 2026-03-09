package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/phekno/gobin/internal/config"
)

func testCfgMgr(t *testing.T, cfg *config.Config) *config.Manager {
	t.Helper()
	return config.NewManager(filepath.Join(t.TempDir(), "config.yaml"), cfg)
}

func TestNotify_Complete(t *testing.T) {
	var received map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfgMgr := testCfgMgr(t, &config.Config{
		Notifications: config.Notifications{
			OnComplete: true,
			Webhooks: []config.Webhook{
				{Name: "test", URL: srv.URL},
			},
		},
	})

	n := New(cfgMgr)
	n.Notify(context.Background(), Event{
		Type:     "complete",
		Name:     "Test.Download",
		Category: "tv",
		Status:   "completed",
		Size:     1024,
		Duration: 5 * time.Minute,
	})

	if received == nil {
		t.Fatal("webhook was not called")
	}
	if received["event"] != "complete" {
		t.Errorf("event = %v", received["event"])
	}
	if received["name"] != "Test.Download" {
		t.Errorf("name = %v", received["name"])
	}
}

func TestNotify_Template(t *testing.T) {
	var body []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading body: %v", err)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfgMgr := testCfgMgr(t, &config.Config{
		Notifications: config.Notifications{
			OnComplete: true,
			Webhooks: []config.Webhook{
				{Name: "discord", URL: srv.URL, Template: `{"content": "{{.Name}} is {{.Status}}"}`},
			},
		},
	})

	n := New(cfgMgr)
	n.Notify(context.Background(), Event{
		Type:   "complete",
		Name:   "My.Movie",
		Status: "completed",
	})

	if body == nil {
		t.Fatal("webhook was not called")
	}
	if string(body) != `{"content": "My.Movie is completed"}` {
		t.Errorf("body = %q", string(body))
	}
}

func TestNotify_Disabled(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfgMgr := testCfgMgr(t, &config.Config{
		Notifications: config.Notifications{
			OnComplete: false, // disabled
			Webhooks:   []config.Webhook{{Name: "test", URL: srv.URL}},
		},
	})

	n := New(cfgMgr)
	n.Notify(context.Background(), Event{Type: "complete", Name: "Test"})

	if called {
		t.Error("webhook should not be called when disabled")
	}
}

func TestNotify_FailureEvent(t *testing.T) {
	var received map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfgMgr := testCfgMgr(t, &config.Config{
		Notifications: config.Notifications{
			OnFailure: true,
			Webhooks:  []config.Webhook{{Name: "test", URL: srv.URL}},
		},
	})

	n := New(cfgMgr)
	n.Notify(context.Background(), Event{
		Type:  "failed",
		Name:  "Bad.NZB",
		Error: "download failed",
	})

	if received == nil {
		t.Fatal("webhook was not called for failure")
	}
	if received["error"] != "download failed" {
		t.Errorf("error = %v", received["error"])
	}
}
