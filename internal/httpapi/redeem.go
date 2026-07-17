package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"cdk-system/internal/observability"
	"cdk-system/internal/ratelimit"
	"cdk-system/internal/redeem"
)

// RedeemHandler holds dependencies for the public redeem endpoint.
type RedeemHandler struct {
	service   *redeem.Service
	rateLimit *ratelimit.Limiter
}

// NewRedeemHandler creates a new RedeemHandler.
func NewRedeemHandler(service *redeem.Service, rateLimit *ratelimit.Limiter) *RedeemHandler {
	return &RedeemHandler{service: service, rateLimit: rateLimit}
}

// Redeem handles POST /api/redeem.
func (h *RedeemHandler) Redeem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Limit request body size.
	r.Body = io.NopCloser(redeem.LimitJSONBody(r))

	var req redeem.RedeemRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	req.IP = r.RemoteAddr
	req.IdempotencyKey = r.Header.Get("Idempotency-Key")

	// Layered rate limiting.
	ip := r.RemoteAddr
	if ip == "" {
		ip = "unknown"
	}
	allowed, err := h.rateLimit.Allow(ctx, "redeem:ip:"+ip)
	if err != nil {
		observability.Logger(ctx).Error("rate limit check failed", "error", err)
	}
	if !allowed {
		RespondError(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
		return
	}
	if req.UserID != "" {
		allowed, err = h.rateLimit.Allow(ctx, "redeem:user:"+redeem.NormalizeUserKey(req.UserID))
		if err != nil {
			observability.Logger(ctx).Error("rate limit check failed", "error", err)
		}
		if !allowed {
			RespondError(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
			return
		}
	}

	resp, err := h.service.Redeem(ctx, req)
	if err != nil {
		if errors.Is(err, redeem.ErrIdempotencyConflict) {
			RespondError(w, http.StatusConflict, "幂等 key 与请求不匹配")
			return
		}
		observability.Logger(ctx).Error("redeem failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "兑换处理失败")
		return
	}

	status := http.StatusOK
	if !resp.OK {
		status = http.StatusBadRequest
		if resp.Result == redeem.ResultRateLimited {
			status = http.StatusTooManyRequests
		}
	}
	RespondJSON(w, status, resp)
}
