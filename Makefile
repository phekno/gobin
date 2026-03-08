MODULE   := github.com/phekno/gobin
BIN      := bin/gobin
IMAGE    := gobin
VERSION  ?= dev
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS  := -X main.version=$(VERSION) -X main.commit=$(COMMIT)

# Auto-detect container engine: prefer podman, fall back to docker
ENGINE   ?= $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)

.PHONY: build run test cover lint vet frontend image image-run image-stop clean

## frontend: build the web UI
frontend:
	cd web && npm ci && npm run build

## build: build frontend and compile the binary
build: frontend
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/gobin

## run: build and run with example config
run: build
	./$(BIN) --config config.example.yaml

## test: run all tests with race detector
test:
	go test ./... -race -count=1

## cover: run tests with coverage and open HTML report
cover:
	go test ./... -race -count=1 -coverprofile=coverage.out -covermode=atomic
	go tool cover -func=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: run golangci-lint (install: https://golangci-lint.run/welcome/install/)
lint:
	golangci-lint run ./...

## vet: run go vet
vet:
	go vet ./...

## image: build the container image
image:
	$(ENGINE) build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t $(IMAGE):$(VERSION) .

## image-run: build image, set up config, and run the container
image-run: image
	@mkdir -p config
	@test -f config/config.yaml || (cp config.example.yaml config/config.yaml && chmod 644 config/config.yaml)
	$(ENGINE) run --rm \
		--name gobin \
		-p 8080:8080 \
		-p 9090:9090 \
		-v $(CURDIR)/config:/config:ro \
		$(IMAGE):$(VERSION)

## image-stop: stop the running container
image-stop:
	-$(ENGINE) stop gobin

## clean: remove build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html
	find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete 2>/dev/null || true

## help: show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'
