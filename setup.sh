#!/usr/bin/env bash
set -euo pipefail

# GoBin local development setup
# Prerequisites: Go 1.22+, Node 20+ (for frontend)

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

info() { echo -e "${CYAN}→${NC} $1"; }
ok()   { echo -e "${GREEN}✓${NC} $1"; }
fail() { echo -e "${RED}✗${NC} $1"; exit 1; }

echo ""
echo "╔═══════════════════════════════════════╗"
echo "║       GoBin Development Setup         ║"
echo "╚═══════════════════════════════════════╝"
echo ""

# --- Check prerequisites ---
info "Checking prerequisites..."

command -v go >/dev/null 2>&1 || fail "Go is not installed. Install Go 1.22+ from https://go.dev/dl/"
GO_VERSION=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+')
ok "Go ${GO_VERSION} found"

# --- Initialize module ---
info "Tidying Go modules..."
go mod tidy
ok "Go modules ready"

# --- Run tests ---
info "Running tests..."
if go test ./... -count=1 -timeout 30s; then
    ok "All tests passed"
else
    fail "Tests failed"
fi

# --- Build binary ---
info "Building gobin..."
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o bin/gobin \
    ./cmd/gobin

ok "Binary built: bin/gobin"

# --- Setup local config ---
if [ ! -f config/config.yaml ]; then
    info "Creating local config from template..."
    mkdir -p config
    cp config.example.yaml config/config.yaml
    ok "Config created at config/config.yaml — edit it with your Usenet server details"
else
    ok "Config already exists at config/config.yaml"
fi

# --- Create local directories ---
mkdir -p /tmp/gobin-dev/{incomplete,complete,nzb}
ok "Local download directories created in /tmp/gobin-dev/"

echo ""
echo "╔═══════════════════════════════════════╗"
echo "║            Setup Complete!            ║"
echo "╚═══════════════════════════════════════╝"
echo ""
echo "  Run locally:    ./bin/gobin --config config/config.yaml"
echo "  Run tests:      go test ./... -v"
echo "  Build Docker:   make docker"
echo "  Full CI:        make test lint build"
echo ""
echo "  API will be at: http://localhost:8080"
echo "  Metrics at:     http://localhost:9090/metrics"
echo ""
