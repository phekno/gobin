# --- Frontend ---
FROM node:24-alpine AS frontend

WORKDIR /web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ .
# Vite builds to ../internal/webui/dist/ relative to web/
# In the container we adjust the output path
RUN npx vite build --outDir /webui-dist

# --- Builder ---
FROM golang:1.26-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Copy built frontend into the embed directory
COPY --from=frontend /webui-dist ./internal/webui/dist/

RUN CGO_ENABLED=0 go build \
    -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /gobin ./cmd/gobin

# --- Runtime ---
FROM alpine:3.23

RUN apk add --no-cache \
    par2cmdline \
    7zip \
    && adduser -D -u 1000 gobin

RUN mkdir -p /config /downloads/incomplete /downloads/complete /downloads/nzb /tmp/gobin \
    && chown -R gobin:gobin /config /downloads /tmp/gobin

COPY --from=builder /gobin /usr/local/bin/gobin

USER gobin

EXPOSE 8080 9090

CMD ["gobin", "--config", "/config/config.yaml"]
