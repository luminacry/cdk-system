package httpapi

import (
	"context"
	"net/http"

	"cdk-system/internal/auth"
)

const (
	SessionCookieName = "cdk_session"
	CSRFCookieName    = "csrf_token"
	CSRFHeaderName    = "X-CSRF-Token"
)

type ctxKey int

const (
	adminIDKey ctxKey = iota
	adminUsernameKey
	csrfTokenKey
)

// WithAdmin returns a context with admin ID and username.
func WithAdmin(ctx context.Context, adminID int64, username string) context.Context {
	ctx = context.WithValue(ctx, adminIDKey, adminID)
	return context.WithValue(ctx, adminUsernameKey, username)
}

// AdminIDFromContext returns the admin ID from the context.
func AdminIDFromContext(ctx context.Context) int64 {
	if id, ok := ctx.Value(adminIDKey).(int64); ok {
		return id
	}
	return 0
}

// AdminUsernameFromContext returns the admin username from the context.
func AdminUsernameFromContext(ctx context.Context) string {
	if username, ok := ctx.Value(adminUsernameKey).(string); ok {
		return username
	}
	return ""
}

// WithCSRFToken returns a context with the CSRF token.
func WithCSRFToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, csrfTokenKey, token)
}

// CSRFTokenFromContext returns the CSRF token from the context.
func CSRFTokenFromContext(ctx context.Context) string {
	if token, ok := ctx.Value(csrfTokenKey).(string); ok {
		return token
	}
	return ""
}

// AdminAuthMiddleware validates the session cookie and sets admin context.
func AdminAuthMiddleware(sessions *auth.SessionStore, requireCSRF bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil {
				RespondError(w, http.StatusUnauthorized, "未登录")
				return
			}

			sess, err := sessions.Get(r.Context(), cookie.Value)
			if err != nil {
				RespondError(w, http.StatusInternalServerError, "会话读取失败")
				return
			}
			if sess == nil {
				RespondError(w, http.StatusUnauthorized, "会话已过期")
				return
			}

			if requireCSRF && isMutatingMethod(r.Method) {
				csrfCookie, err := r.Cookie(CSRFCookieName)
				if err != nil {
					RespondError(w, http.StatusForbidden, "缺少 CSRF cookie")
					return
				}
				headerToken := r.Header.Get(CSRFHeaderName)
				if !auth.VerifyCSRFToken(headerToken, csrfCookie.Value) {
					RespondError(w, http.StatusForbidden, "CSRF token 不匹配")
					return
				}
			}

			r = r.WithContext(WithAdmin(r.Context(), sess.AdminID, sess.Username))
			next.ServeHTTP(w, r)
		})
	}
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// SetSessionCookie writes the session cookie to the response.
func SetSessionCookie(w http.ResponseWriter, value string, maxAge int, secure bool, sameSite http.SameSite) {
	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	}
	http.SetCookie(w, cookie)
}

// SetCSRFCookie writes the CSRF cookie to the response.
func SetCSRFCookie(w http.ResponseWriter, value string, maxAge int, secure bool, sameSite http.SameSite) {
	cookie := &http.Cookie{
		Name:     CSRFCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: false, // must be readable by frontend JavaScript
		Secure:   secure,
		SameSite: sameSite,
	}
	http.SetCookie(w, cookie)
}

// ClearAuthCookies clears session and CSRF cookies.
func ClearAuthCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   SessionCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name:   CSRFCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

// ParseSameSite parses a same-site string.
func ParseSameSite(s string) http.SameSite {
	switch s {
	case "strict":
		return http.SameSiteStrictMode
	case "lax":
		return http.SameSiteLaxMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}
