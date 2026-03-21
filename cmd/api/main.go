package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/richrobertson/notification-platform/internal/config"
	httpserver "github.com/richrobertson/notification-platform/internal/http"
	"github.com/richrobertson/notification-platform/internal/platform"
	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
)

func main() {
	cfg := config.Load()
	logger := platform.NewLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStartup()

	telemetryShutdown, err := platform.SetupTelemetry(startupCtx, cfg)
	if err != nil {
		logger.Error("failed to initialize telemetry", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := telemetryShutdown(shutdownCtx); err != nil {
			logger.Error("failed to shut down telemetry", slog.Any("error", err))
		}
	}()

	postgres, err := store.NewPostgres(startupCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to postgres", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		if err := postgres.Close(); err != nil {
			logger.Error("failed to close postgres", slog.Any("error", err))
		}
	}()

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err := redisQueue.Ping(startupCtx); err != nil {
		logger.Error("failed to connect to redis", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		if err := redisQueue.Close(); err != nil {
			logger.Error("failed to close redis", slog.Any("error", err))
		}
	}()

	router := httpserver.NewRouter(httpserver.RouterDeps{AppName: cfg.AppName, DBPing: postgres.Ping, Store: postgres, Queue: redisQueue})
	server := &http.Server{Addr: ":" + cfg.HTTPPort, Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting api server", slog.String("addr", server.Addr), slog.String("app_name", cfg.AppName), slog.String("environment", cfg.Environment), slog.String("redis_addr", cfg.RedisAddr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http server stopped: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		logger.Error("server exited with error", slog.Any("error", err))
		os.Exit(1)
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("failed to shut down http server", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("api server stopped")
}
