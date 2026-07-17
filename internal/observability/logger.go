package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
)

type ctxKey int

const requestIDKey ctxKey = iota

// RequestIDHeader is the HTTP header used to accept or return request IDs.
const RequestIDHeader = "X-Request-ID"

// NewRequestID generates a random 16-byte hex request ID.
func NewRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// WithRequestID returns a context with the given request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFromContext extracts the request ID from the context.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// Logger returns a slog logger with request_id attached if present.
func Logger(ctx context.Context) *slog.Logger {
	if id := RequestIDFromContext(ctx); id != "" {
		return slog.Default().With("request_id", id)
	}
	return slog.Default()
}
