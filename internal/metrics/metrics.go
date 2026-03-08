package metrics

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// This is a stdlib-only metrics implementation.
// To enable Prometheus, run:
//   go get github.com/prometheus/client_golang/prometheus
// Then replace this file with the full prometheus version in metrics_prometheus.go

// --- Counters and Gauges (atomic, lock-free) ---

type Counter struct {
	value atomic.Int64
}

func (c *Counter) Inc()            { c.value.Add(1) }
func (c *Counter) Add(n int64)     { c.value.Add(n) }
func (c *Counter) Value() int64    { return c.value.Load() }

type Gauge struct {
	value atomic.Int64
}

func (g *Gauge) Set(n int64)       { g.value.Store(n) }
func (g *Gauge) Inc()              { g.value.Add(1) }
func (g *Gauge) Dec()              { g.value.Add(-1) }
func (g *Gauge) Value() int64      { return g.value.Load() }

// --- Registry ---

type Registry struct {
	mu       sync.RWMutex
	counters map[string]*Counter
	gauges   map[string]*Gauge
}

var defaultRegistry = &Registry{
	counters: make(map[string]*Counter),
	gauges:   make(map[string]*Gauge),
}

func GetCounter(name string) *Counter {
	defaultRegistry.mu.Lock()
	defer defaultRegistry.mu.Unlock()
	if c, ok := defaultRegistry.counters[name]; ok {
		return c
	}
	c := &Counter{}
	defaultRegistry.counters[name] = c
	return c
}

func GetGauge(name string) *Gauge {
	defaultRegistry.mu.Lock()
	defer defaultRegistry.mu.Unlock()
	if g, ok := defaultRegistry.gauges[name]; ok {
		return g
	}
	g := &Gauge{}
	defaultRegistry.gauges[name] = g
	return g
}

// --- Well-known metrics ---

var (
	DownloadBytesTotal     = GetCounter("gobin_download_bytes_total")
	DownloadSegmentsOK     = GetCounter("gobin_download_segments_success")
	DownloadSegmentsFailed = GetCounter("gobin_download_segments_failed")
	YEncCRCErrors          = GetCounter("gobin_yenc_crc_errors_total")
	YEncDecodedBytesTotal  = GetCounter("gobin_yenc_decoded_bytes_total")
	HTTPRequestsTotal      = GetCounter("gobin_http_requests_total")

	DownloadSpeedBps       = GetGauge("gobin_download_speed_bps")
	QueueSize              = GetGauge("gobin_queue_size")
	NNTPConnectionsActive  = GetGauge("gobin_nntp_connections_active")
	DiskFreeBytes          = GetGauge("gobin_disk_free_bytes")
)

// --- HTTP Handler (text format, Prometheus-compatible) ---

func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defaultRegistry.mu.RLock()
		defer defaultRegistry.mu.RUnlock()

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for name, c := range defaultRegistry.counters {
			_, _ = fmt.Fprintf(w, "# TYPE %s counter\n%s %d\n", name, name, c.Value())
		}
		for name, g := range defaultRegistry.gauges {
			_, _ = fmt.Fprintf(w, "# TYPE %s gauge\n%s %d\n", name, name, g.Value())
		}
	}
}

// --- HTTP Middleware ---

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(wrapped, r)
		_ = time.Since(start)
		_ = strconv.Itoa(wrapped.status)
		HTTPRequestsTotal.Inc()
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
