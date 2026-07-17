package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"

	"cdk-system/internal/auth"
	"cdk-system/internal/config"
	"cdk-system/internal/httpapi"
	"cdk-system/internal/observability"
	"cdk-system/internal/ratelimit"
	"cdk-system/internal/redeem"
	rcache "cdk-system/internal/redis"
	"cdk-system/internal/store"
)

func distDir() string {
	// Find web/dist relative to the executable or fallback to current directory.
	exe, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exe), "web", "dist")
	}
	return filepath.Join(".", "web", "dist")
}

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

	if err := httpapi.AdminMustExist(ctx, queries); err != nil {
		logger.Error("admin check failed", "error", err)
		os.Exit(1)
	}

	sessions := auth.NewSessionStore(redisClient, cfg.SessionLifetime)
	loginLimiter := ratelimit.NewLimiter(redisClient, cfg.RateLimitRequests, cfg.RateLimitWindow)
	redeemLimiter := ratelimit.NewLimiter(redisClient, cfg.RateLimitRequests, cfg.RateLimitWindow)
	authHandler := httpapi.NewAuthHandler(queries, sessions, loginLimiter, cfg)
	redeemService := redeem.NewService(pool, queries)
	redeemHandler := httpapi.NewRedeemHandler(redeemService, redeemLimiter)
	adminHandler := httpapi.NewAdminHandler(pool, queries, nil)
	credentialsHandler := httpapi.NewCredentialsHandler(pool, queries)
	siteSettingsHandler := httpapi.NewSiteSettingsHandler(queries)

	reg := prometheus.NewRegistry()
	metrics := observability.NewMetrics(reg)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(observability.RecoveryMiddleware)
	r.Use(observability.TrustedProxyMiddleware(cfg.TrustedProxies))
	r.Use(observability.LoggingMiddleware)
	r.Use(metrics.MetricsMiddleware)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			logger.Error("database ping failed", "error", err)
			respondJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "database unavailable"})
			return
		}
		if err := redisClient.Do(r.Context(), redisClient.B().Ping().Build()).Error(); err != nil {
			logger.Error("redis ping failed", "error", err)
			respondJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "redis unavailable"})
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	r.Get("/metrics", metrics.Handler().ServeHTTP)

	r.Route("/api", func(r chi.Router) {
		r.Get("/faq", adminHandler.ListFAQ)
		r.Get("/site-settings", siteSettingsHandler.Get)
		r.Get("/site-logo", siteSettingsHandler.Logo)
		r.Post("/redeem", redeemHandler.Redeem)

		r.Route("/admin", func(r chi.Router) {
			r.Post("/login", authHandler.Login)
			r.Post("/logout", authHandler.Logout)
			r.With(httpapi.AdminAuthMiddleware(sessions, false)).Get("/me", authHandler.Me)

			r.With(httpapi.AdminAuthMiddleware(sessions, false)).Get("/stats", adminHandler.Stats)
			r.With(httpapi.AdminAuthMiddleware(sessions, false)).Get("/batches", adminHandler.ListBatches)
			r.With(httpapi.AdminAuthMiddleware(sessions, true)).Post("/batches", adminHandler.CreateBatch)
			r.With(httpapi.AdminAuthMiddleware(sessions, false)).Get("/batches/{id}", adminHandler.GetBatch)
			r.With(httpapi.AdminAuthMiddleware(sessions, true)).Post("/batches/{id}/toggle", adminHandler.ToggleBatch)
			r.With(httpapi.AdminAuthMiddleware(sessions, false)).Get("/batches/{id}/codes", adminHandler.ListCodes)
			r.With(httpapi.AdminAuthMiddleware(sessions, false)).Get("/batches/{id}/export", adminHandler.ExportBatch)
			r.With(httpapi.AdminAuthMiddleware(sessions, true)).Post("/codes/{id}/toggle", adminHandler.ToggleCode)
			r.With(httpapi.AdminAuthMiddleware(sessions, false)).Get("/logs", adminHandler.ListLogs)
			r.With(httpapi.AdminAuthMiddleware(sessions, true)).Post("/logs/{id}/resend", adminHandler.ResendWebhook)
			r.With(httpapi.AdminAuthMiddleware(sessions, false)).Get("/faq", adminHandler.ListFAQ)
			r.With(httpapi.AdminAuthMiddleware(sessions, true)).Post("/faq", adminHandler.CreateFAQ)
			r.With(httpapi.AdminAuthMiddleware(sessions, true)).Put("/faq/{id}", adminHandler.UpdateFAQ)
			r.With(httpapi.AdminAuthMiddleware(sessions, true)).Delete("/faq/{id}", adminHandler.DeleteFAQ)
			r.With(httpapi.AdminAuthMiddleware(sessions, true)).Post("/faq/reorder", adminHandler.ReorderFAQ)
			r.With(httpapi.AdminAuthMiddleware(sessions, true)).Post("/site-settings", siteSettingsHandler.Update)

			r.With(httpapi.AdminAuthMiddleware(sessions, false)).Get("/credentials", credentialsHandler.ListCredentials)
			r.With(httpapi.AdminAuthMiddleware(sessions, false)).Get("/credentials/stats", credentialsHandler.StatsCredentials)
			r.With(httpapi.AdminAuthMiddleware(sessions, false)).Get("/credentials/plans", credentialsHandler.ListCredentialPlans)
			r.With(httpapi.AdminAuthMiddleware(sessions, false)).Get("/credentials/{id}", credentialsHandler.GetCredential)
			r.With(httpapi.AdminAuthMiddleware(sessions, false)).Get("/credentials/{id}/download", credentialsHandler.DownloadCredential)
			r.With(httpapi.AdminAuthMiddleware(sessions, true)).Post("/credentials/upload", credentialsHandler.UploadCredentials)
			r.With(httpapi.AdminAuthMiddleware(sessions, true)).Delete("/credentials/{id}", credentialsHandler.DeleteCredential)
			r.With(httpapi.AdminAuthMiddleware(sessions, true)).Post("/credentials/batch-delete", credentialsHandler.BatchDeleteCredentials)
		})
	})

	r.NotFound(httpapi.NewSPAHandler(distDir()).ServeHTTP)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		logger.Info("api server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("api server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down api server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
		os.Exit(1)
	}
	logger.Info("api server stopped")
}

func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
