package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration.
type Config struct {
	// HTTP
	Host string
	Port int

	// Database
	DatabaseURL string

	// Redis
	RedisURL string

	// Security
	SessionCookieName     string
	SessionCookieSecure   bool
	SessionCookieSameSite string
	SessionLifetime       time.Duration
	TrustedProxies        []string

	// Rate limiting
	RateLimitRequests int
	RateLimitWindow   time.Duration

	// Logging
	LogLevel *slog.LevelVar
}

// Load reads configuration from environment variables and validates it.
func Load() (*Config, error) {
	var level slog.LevelVar
	if err := level.UnmarshalText([]byte(getEnv("CDK_LOG_LEVEL", "info"))); err != nil {
		level.Set(slog.LevelInfo)
	}

	cfg := &Config{
		Host:                  getEnv("CDK_HOST", "127.0.0.1"),
		Port:                  getIntEnv("CDK_PORT", 8080),
		DatabaseURL:           os.Getenv("CDK_DATABASE_URL"),
		RedisURL:              os.Getenv("CDK_REDIS_URL"),
		SessionCookieName:     getEnv("CDK_SESSION_COOKIE_NAME", "cdk_session"),
		SessionCookieSecure:   getBoolEnv("CDK_SESSION_COOKIE_SECURE", false),
		SessionCookieSameSite: getEnv("CDK_SESSION_COOKIE_SAMESITE", "lax"),
		SessionLifetime:       getDurationEnv("CDK_SESSION_LIFETIME", 12*time.Hour),
		TrustedProxies:        splitEnv("CDK_TRUSTED_PROXIES", []string{"127.0.0.1/32", "::1/128"}),
		RateLimitRequests:     getIntEnv("CDK_RATE_LIMIT_REQUESTS", 10),
		RateLimitWindow:       getDurationEnv("CDK_RATE_LIMIT_WINDOW", time.Minute),
		LogLevel:              &level,
	}

	if cfg.DatabaseURL == "" {
		return nil, errors.New("CDK_DATABASE_URL is required")
	}
	if err := validatePostgresURL(cfg.DatabaseURL); err != nil {
		return nil, fmt.Errorf("invalid CDK_DATABASE_URL: %w", err)
	}
	if cfg.RedisURL == "" {
		return nil, errors.New("CDK_REDIS_URL is required")
	}

	cfg.SessionCookieSameSite = strings.ToLower(cfg.SessionCookieSameSite)
	switch cfg.SessionCookieSameSite {
	case "strict", "lax", "none":
	default:
		return nil, fmt.Errorf("invalid CDK_SESSION_COOKIE_SAMESITE: %s", cfg.SessionCookieSameSite)
	}

	return cfg, nil
}

func validatePostgresURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return fmt.Errorf("scheme must be postgres or postgresql, got %s", u.Scheme)
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getIntEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getBoolEnv(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func splitEnv(key string, fallback []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parts := strings.Split(v, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
