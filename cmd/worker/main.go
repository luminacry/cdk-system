package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"cdk-system/internal/config"
	rcache "cdk-system/internal/redis"
	"cdk-system/internal/store"
	"cdk-system/internal/webhook"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	ctx := context.Background()

	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	redisClient, err := rcache.NewClient(cfg.RedisURL)
	if err != nil {
		logger.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	queries := store.NewQueries(pool)
	deliverer := webhook.NewDeliverer(webhook.DefaultConfig())
	worker := webhook.NewWorker(pool, queries, deliverer, webhook.DefaultConfig())
	worker.Run()

	logger.Info("webhook worker started")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down webhook worker")
	worker.Stop()
	logger.Info("webhook worker stopped")
}
