# GoBin — A Modern Usenet Binary Downloader

[![CI](https://github.com/phekno/gobin/actions/workflows/ci.yaml/badge.svg)](https://github.com/phekno/gobin/actions/workflows/ci.yaml)
[![Release](https://github.com/phekno/gobin/actions/workflows/release.yaml/badge.svg)](https://github.com/phekno/gobin/actions/workflows/release.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/phekno/gobin)](https://goreportcard.com/report/github.com/phekno/gobin)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

GoBin is a from-scratch replacement for [SABnzbd](https://sabnzbd.org) written in Go. It is designed to be cloud-native, running comfortably in a Kubernetes cluster alongside other homelab services, while remaining simple enough to run standalone via Docker or Podman.

**Repository:** [github.com/phekno/gobin](https://github.com/phekno/gobin)
**License:** MIT

---

## Motivation and Goals

SABnzbd is a mature, capable Usenet downloader — but it's written in Python, has 15+ years of accumulated complexity, and doesn't fit naturally into a modern Kubernetes homelab. GoBin exists to solve that.

The goals are:

- Replace the core functionality of SABnzbd (download NZBs, decode yEnc, verify/repair with PAR2, unpack archives).
- Be a single static binary that compiles to a tiny Docker image.
- Be cloud-native from day one: health probes, structured logging, Prometheus metrics, Helm chart, Flux GitOps.
- Use YAML for all configuration with environment variable expansion for secrets.
- Provide a clean Material-inspired dark-mode web UI.
- Be something one person can understand, maintain, and extend.

This is a personal project. It does not aim for feature parity with SABnzbd — it aims to be a simpler, faster, more deployable alternative for a single-user homelab.

---

## Architecture Decisions

### Why Go?

Go was chosen deliberately over Python, Rust, and other candidates:

- **Concurrency model**: Goroutines and channels are a natural fit for a download manager. Each NNTP connection runs in its own goroutine. Segment fetching, yEnc decoding, and disk assembly can all run as concurrent pipeline stages connected by channels.
- **Single binary deployment**: `CGO_ENABLED=0 go build` produces a static binary with zero runtime dependencies. The Docker image is Alpine-based and around 50MB (vs. SABnzbd's Python + pip dependencies).
- **Standard library strength**: Go's stdlib covers HTTP servers, XML parsing, TLS, JSON, and structured logging (`log/slog`). External dependencies are minimal: `gopkg.in/yaml.v3` for config and `go.etcd.io/bbolt` for persistent state.
- **Ecosystem fit**: Most Kubernetes tooling, many homelab projects, and the container ecosystem are Go. It's a natural language for this domain.

### Why roll our own core libraries?

Three critical components are implemented from scratch rather than using external libraries:

1. **NZB parser** (`internal/nzb/`): NZB is a simple XML format. A custom parser gives full control over memory allocation, error handling, and has zero external dependencies. The parser extracts filenames from Usenet subject lines (which have multiple conventions), sorts segments by number, and handles malformed NZBs gracefully.

2. **NNTP client** (`internal/nntp/`): The NNTP protocol is straightforward (RFC 3977). A custom client gives us control over TLS configuration, connection pooling, authentication, dot-stuffing, and buffer sizes. The connection pool maintains persistent connections to each server and reuses them across downloads, avoiding the overhead of repeated TLS handshakes.

3. **yEnc decoder** (`internal/decoder/`): yEnc decoding is the throughput bottleneck in a Usenet downloader. The hot path (`decodeLine`) is kept tight and allocation-free. CRC32 validation catches data corruption. Building this ourselves means we can tune it for performance without pulling in an unmaintained third-party library. (The Go library `go-nzb` exists but appears to be abandoned.)

### Post-processing: shell out, don't reimplement

PAR2 repair and archive extraction are handled by shelling out to mature C tools (`par2cmdline`, `7z`) rather than reimplementing them in Go. These tools are battle-tested and bundled in the Docker image. The `internal/postprocess/` package orchestrates the pipeline: PAR2 verify → repair → 7z extract → cleanup.

### Configuration: single YAML file with env var expansion

All configuration lives in a single `config.yaml` file. This file supports `${ENV_VAR}` syntax, which is expanded at load time using Go's `os.ExpandEnv`. This means Kubernetes secrets can be injected as environment variables and referenced in the config without duplicating values.

The config is loaded and validated at startup with sane defaults (port 8080, 3 retries, standard directory paths). See `config.example.yaml` for the full schema.

### Metrics: stdlib stubs with Prometheus upgrade path

The metrics implementation uses `sync/atomic` counters exposed in Prometheus text format on `:9090/metrics`. Metric names follow Prometheus conventions (`gobin_download_bytes_total`, `gobin_download_speed_bps`, `gobin_nntp_connections_active`, etc.). Upgrading to `prometheus/client_golang` for histograms is a mechanical find-and-replace. The Grafana dashboard JSON is already written against these metric names.

---

## Cloud-Native Design

### Kubernetes deployment

GoBin is designed to run in a Kubernetes cluster. The intended deployment method is the [bjw-s app-template](https://github.com/bjw-s-labs/helm-charts) Helm chart managed by Flux CD, defined in the cluster GitOps repo (not this repo). Deployment manifests (HelmRelease, Kustomize overlays, Secrets) live there, not here.

The app is built to be compatible with that pattern: health probes are mounted outside auth middleware, metrics are on a separate port, and the container runs as non-root.

Key k8s design choices:

- **Strategy: Recreate** (not RollingUpdate). A Usenet downloader is stateful — only one instance should own the download directory at a time. Recreate ensures the old pod is fully terminated before the new one starts.
- **Health probes**: `/healthz` (liveness) and `/readyz` (readiness) are separate endpoints mounted outside the API key auth middleware, so kubelet can reach them without credentials.
- **Network policy**: Egress should be locked down to DNS (53), NNTP (563, 119), and HTTPS (443 for webhooks/RSS).
- **Service monitor**: A Prometheus ServiceMonitor can be configured for automatic scrape target discovery.
- **Stakater Reloader**: The deployment supports `reloader.stakater.com/auto: "true"` for automatic restarts on config changes.

### Docker image

The Dockerfile is a three-stage build:

1. **Frontend stage**: Node 24 Alpine builds the React UI via Vite.
2. **Builder stage**: Go 1.25 Alpine compiles the binary with `CGO_ENABLED=0` and embeds the built frontend.
3. **Runtime stage**: Alpine 3.23 with `par2cmdline` and `7zip` installed. Runs as non-root user (UID 1000).

The image exposes two ports: 8080 (API/UI) and 9090 (metrics). Persistent state (bbolt database) is stored in the downloads volume.

### Structured logging

All logging uses Go 1.21+'s `log/slog` with JSON output. Every log line includes:

- RFC3339Nano timestamp (for sub-millisecond correlation)
- Component name (for filtering in multi-service log aggregators)
- Hostname (for pod identification in k8s)
- Go version (for debugging)

The `internal/logging/` package provides context propagation (`WithContext`/`FromContext`), job-scoped loggers (`WithJob`), and purpose-built log functions for every download lifecycle event (`LogDownloadStart`, `LogDownloadProgress`, `LogDownloadComplete`, `LogSegmentError`, `LogPostProcess`). Each of these emits a structured `event` field for Loki/ELK queries.

### Observability stack

The full observability story:

- **Metrics**: Prometheus-compatible counters, gauges, and histograms covering download throughput, segment success/failure rates, NNTP connection counts, command latency, post-processing duration, yEnc CRC errors, HTTP request rates, and disk usage.
- **Dashboard**: A 15-panel Grafana dashboard (`deploy/grafana/gobin-dashboard.json`) organized into five sections: Overview, Download Performance, NNTP Connections, Post-Processing & Decoder, HTTP API, and Disk & Resources. Includes a `server` template variable for filtering by Usenet server.
- **Auto-provisioning**: A ConfigMap with the `grafana_dashboard: "true"` label for automatic sidecar import.
- **Logging**: JSON structured logs ready for Loki, Fluentd, or ELK.

---

## CI/CD

Two GitHub Actions workflows:

### CI (`ci.yaml`) — runs on push to main and PRs

1. **Test**: `go test ./... -v -race` with coverage upload to Codecov.
2. **Lint**: `golangci-lint` for static analysis.
3. **Build**: Frontend build (Vite) + Go compile for `linux/amd64`.
4. **Trivy FS scan**: Scans Go dependencies for CRITICAL/HIGH vulnerabilities.
5. **Trivy image scan**: Builds the Docker image and scans for OS + library vulnerabilities.

### Release (`release.yaml`) — runs on `v*` tags

1. **Test**: Full test suite as a gate.
2. **Docker**: Builds the image (Dockerfile includes its own frontend stage), scans with Trivy, then pushes to GHCR (`ghcr.io/phekno/gobin`) with semver tags and digest.
3. **GitHub Release**: Builds a `linux/amd64` binary, generates a changelog, and creates a GitHub Release with Docker pull instructions (by tag and by digest).

---

## Project Structure

```
gobin/
├── cmd/gobin/
│   └── main.go                     # Entry point — wires all components
│
├── internal/
│   ├── api/
│   │   ├── server.go               # HTTP API, SSE, auth middleware (forward auth + API key)
│   │   ├── sabnzbd.go              # SABnzbd API compatibility layer for *arr apps
│   │   └── *_test.go
│   ├── assembler/
│   │   └── assembler.go            # Writes decoded segments to files, moves to output dir
│   ├── config/
│   │   ├── config.go               # YAML config parsing, env var expansion, validation
│   │   ├── manager.go              # Thread-safe config with hot-reload and persistence
│   │   └── *_test.go
│   ├── decoder/
│   │   ├── yenc.go                 # yEnc decoder, CRC32 validation
│   │   └── *_test.go
│   ├── engine/
│   │   └── engine.go               # Download pipeline: queue → NNTP → decode → assemble → post-process
│   ├── health/
│   │   ├── health.go               # Liveness/readiness probes
│   │   └── *_test.go
│   ├── logging/
│   │   ├── logging.go              # slog JSON logger, context propagation, lifecycle events
│   │   └── *_test.go
│   ├── metrics/
│   │   ├── metrics.go              # Prometheus-compatible counters and gauges
│   │   └── *_test.go
│   ├── nntp/
│   │   ├── client.go               # NNTP client + connection pool
│   │   └── *_test.go
│   ├── notify/
│   │   └── notify.go               # Webhook notifications (Discord, Slack, generic)
│   ├── nzb/
│   │   ├── parser.go               # NZB XML parser
│   │   └── *_test.go
│   ├── postprocess/
│   │   └── postprocess.go          # PAR2 verify/repair → 7z extract → cleanup
│   ├── queue/
│   │   ├── manager.go              # Job queue with priority, pause/resume, speed tracking
│   │   └── *_test.go
│   ├── rss/
│   │   └── rss.go                  # RSS feed polling with regex filters
│   ├── scheduler/
│   │   └── scheduler.go            # Time-based speed limiting
│   ├── storage/
│   │   └── storage.go              # bbolt persistent state (queue + history)
│   ├── watcher/
│   │   └── watcher.go              # Watch directory for auto-adding NZB files
│   └── webui/
│       └── webui.go                # go:embed for built frontend assets
│
├── web/                             # React frontend (Vite)
│   ├── src/App.jsx                  # Dark-mode UI with queue, history, config editor
│   └── ...
│
├── deploy/grafana/                  # Grafana dashboard JSON
├── .github/workflows/
│   ├── ci.yaml                      # Test + lint + build + Trivy scan
│   └── release.yaml                 # Docker → GHCR + GitHub Release on v* tag
│
├── Dockerfile                       # Multi-stage: Node → Go → Alpine with par2 + 7z
├── docker-compose.yml
├── Makefile
├── go.mod
├── config.example.yaml
├── renovate.json
└── LICENSE
```

---

## API

API endpoints support multiple auth methods: forward auth headers (Authelia/Pocket ID), same-origin bypass (embedded UI), `X-Api-Key` header, `?apikey=` query parameter, or `Authorization: Bearer` header.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/queue` | List all jobs in queue with progress |
| `POST` | `/api/queue` | Add a job (by NZB URL or inline) |
| `DELETE` | `/api/queue/:id` | Remove a job (cancels if active) |
| `POST` | `/api/queue/:id/pause` | Pause a specific job |
| `POST` | `/api/queue/:id/resume` | Resume a specific job |
| `POST` | `/api/queue/pause` | Pause entire queue |
| `POST` | `/api/queue/resume` | Resume entire queue |
| `POST` | `/api/nzb/upload` | Upload an NZB file (multipart) |
| `GET` | `/api/history` | Completed/failed download history |
| `DELETE` | `/api/history/:id` | Delete a history entry |
| `GET` | `/api/config` | Get current config (passwords redacted) |
| `PUT` | `/api/config` | Update config at runtime |
| `GET` | `/api/status` | Server version, uptime, speed, queue size |
| `GET` | `/api/events` | Server-sent events for live progress updates |
| `GET` | `/healthz` | Liveness probe (no auth) |
| `GET` | `/readyz` | Readiness probe with per-component status (no auth) |
| `GET` | `:9090/metrics` | Prometheus-compatible metrics (separate port) |
| `GET/POST` | `/sabnzbd/api` | SABnzbd-compatible API for Sonarr/Radarr/Lidarr |

---

## Configuration Reference

See `config.example.yaml` for the complete annotated configuration. Key sections:

- **`general`**: Download/complete/watch directories, permissions, log level.
- **`servers`**: Multiple Usenet servers with priority (0 = primary, 1+ = backup/block). Per-server connection limits, TLS, auth.
- **`categories`**: Map category names to output subdirectories and post-processing scripts.
- **`downloads`**: Retry count, article cache size, temp directory, speed limit.
- **`schedule`**: Time-based speed limiting (e.g., slower during work hours).
- **`postprocess`**: PAR2 verify/repair, 7z extraction, cleanup toggle, script directory.
- **`api`**: Listen address, port, API key, forward auth (Authelia/Pocket ID), CORS origins.
- **`notifications`**: Webhook URLs with Go template payloads.
- **`rss`**: Feed URLs with include/exclude regex filters per category.

---

## Running with Docker

The quickest way to run GoBin. If no config file is found, GoBin creates a default one automatically.

```bash
# 1. Create a config directory and copy the example config
mkdir -p config
cp config.example.yaml config/config.yaml

# 2. Edit with your Usenet server details (or use env vars — see below)
vim config/config.yaml

# 3. Run with Docker
docker run -d \
  -p 8080:8080 \
  -p 9090:9090 \
  -v $(pwd)/config:/config:ro \
  -v gobin-downloads:/downloads \
  -e USENET_HOST=news.example.com \
  -e USENET_USER=myuser \
  -e USENET_PASS=mypassword \
  -e GOBIN_API_KEY=change-me-to-something-random \
  ghcr.io/phekno/gobin:latest
```

Or use the included `docker-compose.yml`:

```bash
mkdir -p config
cp config.example.yaml config/config.yaml
# Edit config/config.yaml with your settings
docker compose up --build
```

The config file supports `${ENV_VAR}` syntax, so you can reference the environment variables above in your YAML (e.g., `host: ${USENET_HOST}`).

Once running:
- **API**: http://localhost:8080
- **Health probes**: `/healthz` (liveness), `/readyz` (readiness)
- **Metrics**: http://localhost:9090/metrics

---

## Local Development

```bash
git clone git@github.com:phekno/gobin.git
cd gobin
chmod +x setup.sh
./setup.sh
```

The setup script checks for Go 1.25+, runs `go mod tidy`, builds the binary, runs all tests with the race detector, runs `go vet`, and creates local config/download directories.

To run locally after setup:

```bash
# Create a config from the example
mkdir -p config
cp config.example.yaml config/config.yaml
vim config/config.yaml

# Build and run
make build
./bin/gobin --config config/config.yaml
```

Other useful make targets:

```bash
make test       # run tests with race detector
make cover      # tests + coverage report (coverage.html)
make lint       # golangci-lint
make image      # build container image (auto-detects podman/docker)
make image-run  # build + run the container
make frontend   # build the React UI only
make help       # list all targets
```

---

## *arr Integration

GoBin is compatible with Sonarr, Radarr, and Lidarr via the SABnzbd API at `/sabnzbd/api`. To add GoBin as a download client:

1. In Sonarr/Radarr: **Settings → Download Clients → Add → SABnzbd**
2. **Host**: your GoBin host (e.g., `gobin.downloads.svc.cluster.local`)
3. **Port**: `8080`
4. **API Key**: your GoBin API key from `config.yaml`
5. **Category**: `tv`, `movies`, etc. (must match a configured category)

---

## Technology Choices Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Language | Go 1.25 | Concurrency, single binary, stdlib, ecosystem fit |
| Config format | YAML | Human-readable, env var expansion, k8s native |
| Logging | `log/slog` (JSON) | Stdlib, structured, Loki/ELK-ready |
| HTTP framework | Go stdlib (`net/http`) | Go 1.22+ routing is sufficient, zero dependencies |
| Metrics | `sync/atomic` (Prometheus-compatible) | Minimal deps, clear upgrade path to `prometheus/client_golang` |
| Persistent state | bbolt | Pure Go, single-file DB, no CGO needed |
| Frontend | React + Vite | Fast builds, embedded via `go:embed` |
| NZB parsing | Custom | Simple format, full control, zero deps |
| NNTP client | Custom | Full control over pooling, TLS, buffers |
| yEnc decoder | Custom | Performance-critical hot path, abandoned alternatives |
| PAR2/unpack | Shell out to `par2cmdline`, `7z` | Mature C tools, not worth reimplementing |
| *arr integration | SABnzbd API compat | Works with Sonarr/Radarr/Lidarr out of the box |
| Auth | Forward auth + API key | Authelia/Pocket ID via reverse proxy, API key for clients |
| Container base | Alpine 3.23 | Small, has par2/7zip packages |
| Helm chart | bjw-s app-template v4 (in cluster repo) | Homelab community standard |
| GitOps | Flux CD (in cluster repo) | Already running on target cluster |
| CI/CD | GitHub Actions + Trivy | Test, lint, build, security scan |
| Container registry | GHCR | Free for public repos, native to GitHub |
| License | MIT | Personal project, no restrictions needed |
