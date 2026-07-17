package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cdk-system/internal/auth"
	"cdk-system/internal/config"
	"cdk-system/internal/observability"
	"cdk-system/internal/ratelimit"
	"cdk-system/internal/store"
)

// AuthHandler holds dependencies for authentication endpoints.
type AuthHandler struct {
	queries   *store.Queries
	sessions  *auth.SessionStore
	rateLimit *ratelimit.Limiter
	cfg       *config.Config
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(queries *store.Queries, sessions *auth.SessionStore, rateLimit *ratelimit.Limiter, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		queries:   queries,
		sessions:  sessions,
		rateLimit: rateLimit,
		cfg:       cfg,
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login handles administrator login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)

	var req loginRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Username) > 128 || req.Password == "" || len(req.Password) > 1024 {
		RespondError(w, http.StatusBadRequest, "用户名或密码格式错误")
		return
	}

	ip := r.RemoteAddr
	if ip == "" {
		ip = "unknown"
	}
	allowed, err := h.rateLimit.Allow(ctx, "login:ip:"+ip)
	if err != nil {
		observability.Logger(ctx).Error("rate limit check failed", "error", err)
	}
	if !allowed {
		RespondError(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
		return
	}

	admin, err := h.queries.GetAdminByUsername(ctx, req.Username)
	if err != nil {
		RespondError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	ok, err := auth.VerifyPassword(req.Password, admin.PasswordHash)
	if err != nil || !ok {
		RespondError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	csrfToken := auth.NewCSRFToken()
	sessID, err := h.sessions.Create(ctx, auth.Session{
		AdminID:        admin.ID,
		Username:       admin.Username,
		SessionVersion: admin.SessionVersion,
		CreatedAt:      time.Now().UTC(),
	})
	if err != nil {
		observability.Logger(ctx).Error("failed to create session", "error", err)
		RespondError(w, http.StatusInternalServerError, "会话创建失败")
		return
	}

	sameSite := ParseSameSite(h.cfg.SessionCookieSameSite)
	SetSessionCookie(w, sessID, int(h.cfg.SessionLifetime.Seconds()), h.cfg.SessionCookieSecure, sameSite)
	SetCSRFCookie(w, csrfToken, int(h.cfg.SessionLifetime.Seconds()), h.cfg.SessionCookieSecure, sameSite)

	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"username": admin.Username,
	})
}

// Logout handles administrator logout.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		_ = h.sessions.Delete(ctx, cookie.Value)
	}
	ClearAuthCookies(w)
	OK(w)
}

// Me returns the current administrator username.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	username := AdminUsernameFromContext(r.Context())
	if username == "" {
		RespondError(w, http.StatusUnauthorized, "未登录")
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"username": username,
	})
}

// AdminMustExist checks that at least one admin exists before server startup.
func AdminMustExist(ctx context.Context, queries *store.Queries) error {
	count, err := queries.CountAdmins(ctx)
	if err != nil {
		return fmt.Errorf("count admins: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("no admin exists; create one with: go run ./cmd/admin-create <username>")
	}
	return nil
}
