# GoBin — A Modern Usenet Binary Downloader

GoBin is a from-scratch replacement for [SABnzbd](https://sabnzbd.org) written in Go. It is designed to be cloud-native, running comfortably in a Kubernetes cluster alongside other homelab services, while remaining simple enough to run standalone via Docker Compose.

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
- **Standard library strength**: Go's stdlib covers HTTP servers, XML parsing, TLS, JSON, and structured logging (`log/slog`). The only external dependency is `gopkg.in/yaml.v3`.
- **Ecosystem fit**: Most Kubernetes tooling, many homelab projects, and the container ecosystem are Go. It's a natural language for this domain.

### Why roll our own core libraries?

Three critical components are implemented from scratch rather than using external libraries:

1. **NZB parser** (`internal/nzb/`): NZB is a simple XML format. A custom parser gives full control over memory allocation, error handling, and has zero external dependencies. The parser extracts filenames from Usenet subject lines (which have multiple conventions), sorts segments by number, and handles malformed NZBs gracefully.

2. **NNTP client** (`internal/nntp/`): The NNTP protocol is straightforward (RFC 3977). A custom client gives us control over TLS configuration, connection pooling, authentication, dot-stuffing, and buffer sizes. The connection pool maintains persistent connections to each server and reuses them across downloads, avoiding the overhead of repeated TLS handshakes.

3. **yEnc decoder** (`internal/decoder/`): yEnc decoding is the throughput bottleneck in a Usenet downloader. The hot path (`decodeLine`) is kept tight and allocation-free. CRC32 validation catches data corruption. Building this ourselves means we can tune it for performance without pulling in an unmaintained third-party library. (The Go library `go-nzb` exists but appears to be abandoned.)

### Post-processing: shell out, don't reimplement

PAR2 repair and archive extraction are handled by shelling out to mature C tools (`par2cmdline`, `unrar`, `7z`) rather than reimplementing them in Go. These tools are battle-tested and bundled in the Docker image. The `internal/postprocess/` package (not yet fully implemented) orchestrates the pipeline: verify → repair → unpack → rename → cleanup → notify.

### Configuration: single YAML file with env var expansion

All configuration lives in a single `config.yaml` file. This file supports `${ENV_VAR}` syntax, which is expanded at load time using Go's `os.ExpandEnv`. This means Kubernetes secrets can be injected as environment variables and referenced in the config without duplicating values.

The config is loaded and validated at startup with sane defaults (port 8080, 3 retries, standard directory paths). See `config.example.yaml` for the full schema.

### Metrics: stdlib stubs with Prometheus upgrade path

The current metrics implementation uses `sync/atomic` counters exposed as JSON on `:9090/metrics`. This was a pragmatic choice to keep the dependency tree minimal during initial development. The metric names and structure already follow Prometheus naming conventions (`gobin_download_bytes_total`, `gobin_nntp_command_duration_seconds`, etc.), so upgrading to `prometheus/client_golang` is a mechanical find-and-replace. The Grafana dashboard JSON is already written against the Prometheus metric names.

---

## Cloud-Native Design

### Kubernetes deployment

GoBin is designed to run in a Kubernetes cluster. A plain Kustomize-based deployment is included in `k8s/` with base + prod overlay.

For production, the intended deployment method is the [bjw-s app-template](https://github.com/bjw-s-labs/helm-charts) Helm chart managed by Flux CD, defined in the cluster GitOps repo (not this repo). The app is built to be compatible with that pattern: health probes are mounted outside auth middleware, metrics are on a separate port, and the container runs as non-root.

Key k8s design choices:

- **Strategy: Recreate** (not RollingUpdate). A Usenet downloader is stateful — only one instance should own the download directory at a time. Recreate ensures the old pod is fully terminated before the new one starts.
- **Health probes**: `/healthz` (liveness) and `/readyz` (readiness) are separate endpoints mounted outside the API key auth middleware, so kubelet can reach them without credentials.
- **Network policy**: Egress should be locked down to DNS (53), NNTP (563, 119), and HTTPS (443 for webhooks/RSS).
- **Service monitor**: A Prometheus ServiceMonitor can be configured for automatic scrape target discovery.
- **Stakater Reloader**: The deployment supports `reloader.stakater.com/auto: "true"` for automatic restarts on config changes.

### Docker image

The Dockerfile is a multi-stage build:

1. **Builder stage**: Compiles the Go binary with `CGO_ENABLED=0` for a static build. Accepts `VERSION` and `COMMIT` build args for stamping.
2. **Runtime stage**: Alpine 3.19 with `par2cmdline`, `unrar`, `p7zip`, and `tini` installed. Runs as non-root user (UID 1000). `tini` is used as PID 1 for proper signal forwarding in Kubernetes (Go doesn't always handle SIGTERM correctly when it's PID 1 in a container).

The image exposes two ports: 8080 (API/UI) and 9090 (metrics).

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

1. **Test**: `go test ./... -v -race -count=1` with coverage upload to Codecov.
2. **Lint**: `golangci-lint` for static analysis.
3. **Frontend**: `npm ci && npm run build` in the `web/` directory.
4. **Build**: Cross-compile for `linux/amd64` and `linux/arm64` to verify the binary compiles on both architectures.

### Release (`release.yaml`) — runs on `v*` tags

1. **Test**: Full test suite as a gate.
2. **Frontend build**: Produces the static assets.
3. **Docker**: Multi-arch build via `docker/buildx-action` with QEMU. Pushes to GHCR (`ghcr.io/phekno/gobin`) with semver tags (`0.1.0`, `0.1`, `0`, `latest`, plus the Git SHA).
4. **GitHub Release**: Attaches `linux/amd64` and `linux/arm64` binaries. Generates a changelog from commit messages since the previous tag.
5. **Flux notify**: Dispatches a `repository_dispatch` event to the GitOps repo so Flux can pick up the new image tag. (Alternatively, Flux image automation can be used for fully automated tag bumps.)

---

## Project Structure

```
gobin/
├── cmd/gobin/
│   └── main.go                     # Entry point — wires config, queue, API, health, metrics
│
├── internal/
│   ├── config/
│   │   ├── config.go               # YAML config parsing, env var expansion, validation, defaults
│   │   └── config_test.go          # Tests: loading, defaults, env expansion, validation errors
│   │
│   ├── nzb/
│   │   ├── parser.go               # NZB XML parser, segment sorting, filename extraction
│   │   └── parser_test.go          # Tests: parsing, filenames, byte/segment totals, empty NZB
│   │
│   ├── nntp/
│   │   └── client.go               # NNTP client (TLS, auth, BODY command, dot-stuffing)
│   │                                # Connection pool (Get/Put, health checks, auto-reconnect)
│   │
│   ├── decoder/
│   │   ├── yenc.go                 # yEnc single/multi-part decoder, CRC32 validation
│   │   └── yenc_test.go            # Tests: basic decode, escape sequences, multipart, field extraction
│   │
│   ├── queue/
│   │   ├── manager.go              # Job queue: add/remove/pause/resume, priority ordering
│   │   │                            # Atomic progress counters, sliding-window speed tracker
│   │   └── manager_test.go         # Tests: CRUD, duplicates, pause/resume, priority, progress
│   │
│   ├── api/
│   │   └── server.go               # HTTP API (Go 1.22 routing), SSE, auth middleware
│   │                                # Health probes mounted outside auth
│   │
│   ├── logging/
│   │   └── logging.go              # slog JSON logger, context propagation, job/trace enrichment
│   │                                # Lifecycle log functions (download.start, .progress, .complete)
│   │
│   ├── metrics/
│   │   └── metrics.go              # sync/atomic counters (Prometheus upgrade path documented)
│   │
│   └── health/
│       └── health.go               # Liveness (/healthz) and readiness (/readyz) probes
│                                    # Per-component status tracking
│
├── deploy/
│   └── grafana/
│       ├── gobin-dashboard.json     # 15-panel Grafana dashboard
│       └── dashboard-configmap.yaml # Auto-provisioning via sidecar label
│
├── k8s/                             # Plain Kustomize deployment
│   ├── base/
│   │   ├── namespace.yaml
│   │   ├── configmap.yaml
│   │   ├── secret.yaml
│   │   ├── deployment.yaml          # ServiceAccount, probes, preStop hook
│   │   ├── service.yaml             # ClusterIP + Ingress + PVC + NetworkPolicy + ServiceMonitor
│   │   └── kustomization.yaml
│   └── overlays/prod/
│       └── kustomization.yaml       # Production patches (more resources, Longhorn, 2Ti PVC)
│
├── .github/workflows/
│   ├── ci.yaml                      # Test + lint + build on push/PR
│   └── release.yaml                 # Multi-arch Docker → GHCR + GitHub Release on v* tag
│
├── Dockerfile                       # Multi-stage: Go build → Alpine runtime with tini, par2, unrar, 7z
├── docker-compose.yml               # Simple local deployment
├── Makefile                         # build, run, test, cover, lint, docker, frontend, clean
├── go.mod                           # Module: github.com/phekno/gobin — single dep: gopkg.in/yaml.v3
├── config.example.yaml              # Full annotated example config
├── setup.sh                         # One-command local dev setup (prereqs, build, test)
├── .gitignore
└── LICENSE                          # MIT
```

---

## Packages Not Yet Implemented

The following packages are referenced in the architecture but do not yet have code. They represent the next phases of development:

- **`internal/assembler/`**: Reassembles decoded yEnc segments into complete files. Needs a buffered disk writer that handles temp files and atomic renames.
- **`internal/postprocess/`**: Orchestrates the post-download pipeline: PAR2 verify → repair → unrar/7z extract → obfuscation rename → cleanup → script execution → notification.
- **`internal/scheduler/`**: Speed limits, time-window scheduling (e.g., limit to 50 MB/s during work hours), bandwidth management.
- **`internal/rss/`**: RSS feed polling with regex/category-based filters for automatic NZB addition.
- **`internal/notify/`**: Notification dispatcher for webhooks (Discord, Slack, generic) and Apprise integration.
- **`internal/storage/`**: Persistent state via SQLite or bbolt for download history, queue state across restarts, and schema migrations.
- **`web/`**: React frontend using Material UI with dark mode. A prototype UI exists as a standalone JSX artifact but has not been integrated into the build. The Go binary will serve it via `go:embed`.

---

## API

All API endpoints (except health probes) require authentication via `X-Api-Key` header, `?apikey=` query parameter, or `Authorization: Bearer` header.

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

---

## Configuration Reference

See `config.example.yaml` for the complete annotated configuration. Key sections:

- **`general`**: Download/complete/watch directories, permissions, log level.
- **`servers`**: Multiple Usenet servers with priority (0 = primary, 1+ = backup/block). Per-server connection limits, TLS, auth.
- **`categories`**: Map category names to output subdirectories and post-processing scripts.
- **`downloads`**: Retry count, article cache size, temp directory, speed limit.
- **`schedule`**: Time-based speed limiting (e.g., slower during work hours).
- **`postprocess`**: Paths to par2/unrar/7z binaries, cleanup toggle, script directory.
- **`api`**: Listen address, port, API key, base URL (for reverse proxy subpaths), CORS origins.
- **`notifications`**: Webhook URLs with Go template payloads.
- **`rss`**: Feed URLs with include/exclude regex filters per category.

---

## Local Development

```bash
git clone git@github.com:phekno/gobin.git
cd gobin
chmod +x setup.sh
./setup.sh
```

The setup script checks for Go 1.22+, runs `go mod tidy`, builds the binary, runs all tests with the race detector, runs `go vet`, and creates local config/download directories.

To run locally after setup:

```bash
# Edit config with your Usenet server details
vim config/config.yaml

# Run
./bin/gobin --config config/config.yaml

# Or via Docker
docker compose up --build
```

The API is at `http://localhost:8080`, health probes at `/healthz` and `/readyz`, metrics at `http://localhost:9090/metrics`.

---

## Suggested Development Order

For anyone picking this up (including Claude Code):

1. **Get tests green**: Run `go mod tidy && go test ./... -v -race`. Fix any issues. The existing tests cover config loading, NZB parsing, yEnc decoding, and queue management.
2. **Assembler**: Implement `internal/assembler/` to stitch decoded segments into files on disk. This is the glue between the decoder and the filesystem.
3. **Download engine**: Wire up the NNTP client pool + NZB parser + decoder + assembler into a download pipeline. Fetch a real NZB end-to-end.
4. **Post-processing**: Implement `internal/postprocess/` — shell out to par2/unrar/7z.
5. **Persistent state**: Add SQLite or bbolt for queue state and history that survives restarts.
6. **Web UI**: Integrate the React frontend via `go:embed`.
7. **RSS + notifications**: Add automated NZB fetching and completion alerts.
8. **Prometheus upgrade**: Replace `sync/atomic` metrics with `prometheus/client_golang`.

---

## Technology Choices Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Language | Go 1.22 | Concurrency, single binary, stdlib, ecosystem fit |
| Config format | YAML | Human-readable, env var expansion, k8s native |
| Logging | `log/slog` (JSON) | Stdlib, structured, Loki/ELK-ready |
| HTTP framework | Go stdlib (`net/http`) | Go 1.22 routing is sufficient, zero dependencies |
| Metrics | `sync/atomic` (stub) → `prometheus/client_golang` | Minimal deps now, clear upgrade path |
| NZB parsing | Custom | Simple format, full control, zero deps |
| NNTP client | Custom | Full control over pooling, TLS, buffers |
| yEnc decoder | Custom | Performance-critical hot path, abandoned alternatives |
| PAR2/unpack | Shell out to `par2cmdline`, `unrar`, `7z` | Mature C tools, not worth reimplementing |
| Container base | Alpine 3.19 | Small, has par2/unrar/7z packages |
| PID 1 | `tini` | Proper signal forwarding in containers |
| Helm chart | bjw-s app-template v4 (in cluster repo) | Homelab community standard |
| GitOps | Flux CD (in cluster repo) | Already running on target cluster |
| CI/CD | GitHub Actions | Repo is on GitHub, native integration |
| Container registry | GHCR | Free for public repos, native to GitHub |
| Secret management | SOPS or ExternalSecret | Both supported, user's choice |
| License | MIT | Personal project, no restrictions needed |
