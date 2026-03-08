package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCounter_IncAndValue(t *testing.T) {
	c := &Counter{}
	for i := 0; i < 5; i++ {
		c.Inc()
	}
	if c.Value() != 5 {
		t.Errorf("Value = %d, want 5", c.Value())
	}
}

func TestCounter_Add(t *testing.T) {
	c := &Counter{}
	c.Add(10)
	c.Add(5)
	if c.Value() != 15 {
		t.Errorf("Value = %d, want 15", c.Value())
	}
}

func TestGauge_SetAndValue(t *testing.T) {
	g := &Gauge{}
	g.Set(42)
	if g.Value() != 42 {
		t.Errorf("Value = %d, want 42", g.Value())
	}
	g.Set(0)
	if g.Value() != 0 {
		t.Errorf("Value = %d, want 0", g.Value())
	}
}

func TestGauge_IncDec(t *testing.T) {
	g := &Gauge{}
	g.Inc()
	g.Inc()
	g.Inc()
	g.Dec()
	if g.Value() != 2 {
		t.Errorf("Value = %d, want 2", g.Value())
	}
}

func TestGetCounter_CreateAndReuse(t *testing.T) {
	name := "test_counter_" + t.Name()
	c1 := GetCounter(name)
	c2 := GetCounter(name)
	if c1 != c2 {
		t.Error("GetCounter should return the same instance for the same name")
	}
}

func TestGetGauge_CreateAndReuse(t *testing.T) {
	name := "test_gauge_" + t.Name()
	g1 := GetGauge(name)
	g2 := GetGauge(name)
	if g1 != g2 {
		t.Error("GetGauge should return the same instance for the same name")
	}
}

func TestHandler_OutputFormat(t *testing.T) {
	// Use unique names to avoid conflicts with other tests
	cName := "test_handler_counter_" + t.Name()
	gName := "test_handler_gauge_" + t.Name()

	c := GetCounter(cName)
	c.Add(42)
	g := GetGauge(gName)
	g.Set(7)

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))

	body := rec.Body.String()

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	if !strings.Contains(body, "# TYPE "+cName+" counter") {
		t.Errorf("missing counter TYPE line for %s", cName)
	}
	if !strings.Contains(body, cName+" 42") {
		t.Errorf("missing counter value line for %s", cName)
	}
	if !strings.Contains(body, "# TYPE "+gName+" gauge") {
		t.Errorf("missing gauge TYPE line for %s", gName)
	}
	if !strings.Contains(body, gName+" 7") {
		t.Errorf("missing gauge value line for %s", gName)
	}
}

func TestMiddleware_IncrementsRequestCounter(t *testing.T) {
	before := HTTPRequestsTotal.Value()

	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/test", nil))

	after := HTTPRequestsTotal.Value()
	if after != before+1 {
		t.Errorf("HTTPRequestsTotal = %d, want %d", after, before+1)
	}
}

func TestStatusWriter_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec, status: 200}

	sw.WriteHeader(503)
	if sw.status != 503 {
		t.Errorf("status = %d, want 503", sw.status)
	}
	if rec.Code != 503 {
		t.Errorf("underlying recorder code = %d, want 503", rec.Code)
	}
}

func TestWellKnownMetrics_Exist(t *testing.T) {
	// Smoke test: verify package-level metrics are initialized and usable
	metrics := []*Counter{
		DownloadBytesTotal,
		DownloadSegmentsOK,
		DownloadSegmentsFailed,
		YEncCRCErrors,
		YEncDecodedBytesTotal,
		HTTPRequestsTotal,
	}
	for _, m := range metrics {
		if m == nil {
			t.Error("well-known counter is nil")
		}
		m.Inc() // should not panic
	}

	gauges := []*Gauge{
		DownloadSpeedBps,
		QueueSize,
		NNTPConnectionsActive,
		DiskFreeBytes,
	}
	for _, g := range gauges {
		if g == nil {
			t.Error("well-known gauge is nil")
		}
		g.Set(1) // should not panic
	}
}
