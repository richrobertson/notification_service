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
	"github.com/richrobertson/notification-platform/internal/pressure"
	"github.com/richrobertson/notification-platform/internal/queue"
	"github.com/richrobertson/notification-platform/internal/store"
)

func main() {
	cfg := config.Load()
	if err := cfg.ValidateForAPI(); err != nil {
		slog.Error("invalid configuration", slog.Any("error", err))
		os.Exit(1)
	}
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

	monitor := pressure.NewMonitor(redisQueue, cfg.QueueSoftLimit, cfg.QueueHardLimit, cfg.BackpressureRetryAfter)
	router := httpserver.NewRouter(httpserver.RouterDeps{
		AppName:             cfg.AppName,
		AdminToken:          cfg.AdminToken,
		MaxRequestBodyBytes: cfg.MaxRequestBodyBytes,
		DBPing:              postgres.Ping,
		RedisPing:           redisQueue.Ping,
		Store:               postgres,
		Queue:               redisQueue,
		Monitor:             monitor,
		Limiter:             queue.NewTenantRateLimiter(redisQueue, cfg.APIRateLimitPerSecond, cfg.APIRateLimitWindow),
	})
	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}

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

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.HTTPShutdownTimeout)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("failed to shut down http server", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("api server stopped")
}
