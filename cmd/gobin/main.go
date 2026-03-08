package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/phekno/gobin/internal/api"
	"github.com/phekno/gobin/internal/config"
	"github.com/phekno/gobin/internal/health"
	"github.com/phekno/gobin/internal/logging"
	"github.com/phekno/gobin/internal/queue"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	configPath := flag.String("config", "/config/config.yaml", "path to config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("gobin %s (commit: %s)\n", version, commit)
		os.Exit(0)
	}

	// Bootstrap logger (will be replaced once config is loaded)
	logger := logging.New("info", "main")
	slog.SetDefault(logger)

	slog.Info("starting gobin", "version", version, "commit", commit, "config", *configPath)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Reconfigure logger with config-specified level
	logger = logging.New(cfg.General.LogLevel, "main")
	slog.SetDefault(logger)

	// Context with graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Health checker
	checker := health.New()
	checker.Healthy("config")

	// Queue manager
	queueMgr := queue.NewManager(3) // max 3 concurrent downloads
	_ = queueMgr                    // TODO: wire to download engine

	// API server
	apiAddr := fmt.Sprintf("%s:%d", cfg.API.Listen, cfg.API.Port)
	srv := api.NewServer(cfg.API.APIKey, checker)

	// Metrics server (separate port)
	metricsMux := http.NewServeMux()
	metricsMux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		// Placeholder — will use promhttp.Handler() once prometheus dep is added
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "# gobin metrics placeholder\n")
	})
	metricsAddr := "0.0.0.0:9090"

	// Start health checks
	go checker.StartPeriodicChecks(ctx, 15*time.Second)

	// Start metrics server
	metricsServer := &http.Server{Addr: metricsAddr, Handler: metricsMux}
	go func() {
		slog.Info("metrics server starting", "addr", metricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server error", "error", err)
		}
	}()

	// Start API server
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe(apiAddr)
	}()

	// Wait for shutdown signal or fatal error
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		slog.Info("received signal, shutting down", "signal", sig)
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("fatal error", "error", err)
			os.Exit(1)
		}
	}

	// Graceful shutdown
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	metricsServer.Shutdown(shutdownCtx)

	slog.Info("gobin stopped")
}
