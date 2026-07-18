package redeem

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cdk-system/internal/store"
)

const (
	MaxCodeLength           = 128
	MaxUserIDLength         = 256
	MaxIdempotencyKeyLength = 128
	MaxPayloadSize          = 64 * 1024
	MaxBatchCodes           = 20
)

// ErrIdempotencyConflict is returned when an idempotency key is reused with a different request.
var ErrIdempotencyConflict = errors.New("idempotency key conflict")

const (
	ResultInvalidInput          = "invalid_input"
	ResultInvalidCode           = "invalid_code"
	ResultCodeDisabled          = "code_disabled"
	ResultBatchDisabled         = "batch_disabled"
	ResultExpired               = "expired"
	ResultCodeUsedUp            = "code_used_up"
	ResultUserLimit             = "user_limit"
	ResultRateLimited           = "rate_limited"
	ResultCredentialUnavailable = "credential_unavailable"
	ResultSuccess               = "success"
)

// Service provides the core redemption logic.
type Service struct {
	pool    *pgxpool.Pool
	queries *store.Queries
}

// NewService creates a new redemption service.
func NewService(pool *pgxpool.Pool, queries *store.Queries) *Service {
	return &Service{pool: pool, queries: queries}
}

// RedeemRequest is the input for redeeming a code.
type RedeemRequest struct {
	UserID         string `json:"user_id"`
	Code           string `json:"code"`
	IdempotencyKey string `json:"-"`
	IP             string `json:"-"`
}

// RedeemResponse is the output of a redemption attempt.
type RedeemResponse struct {
	OK             bool            `json:"ok"`
	Result         string          `json:"result"`
	Message        string          `json:"message"`
	Batch          string          `json:"batch,omitempty"`
	Code           string          `json:"code,omitempty"`
	UserID         string          `json:"user_id,omitempty"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	Credential     json.RawMessage `json:"credential,omitempty"`
	RedeemedAt     string          `json:"redeemed_at,omitempty"`
	EventID        string          `json:"-"`
	WebhookPending bool            `json:"-"`
}

// BatchRedeemRequest is the input for redeeming several codes for one user.
type BatchRedeemRequest struct {
	UserID         string   `json:"user_id"`
	Codes          []string `json:"codes"`
	IdempotencyKey string   `json:"-"`
	IP             string   `json:"-"`
}

// BatchRedeemItem keeps each result associated with its submitted position.
type BatchRedeemItem struct {
	Code       string          `json:"code"`
	OK         bool            `json:"ok"`
	Result     string          `json:"result"`
	Message    string          `json:"message"`
	Redemption *RedeemResponse `json:"redemption,omitempty"`
}

// BatchRedeemResponse summarizes a completed batch request.
type BatchRedeemResponse struct {
	OK        bool              `json:"ok"`
	Message   string            `json:"message,omitempty"`
	Total     int               `json:"total"`
	Succeeded int               `json:"succeeded"`
	Failed    int               `json:"failed"`
	Results   []BatchRedeemItem `json:"results,omitempty"`
}

// BatchRedeem redeems codes in order. Each code uses its own transaction, so
// normal business failures do not roll back or stop the remaining items.
func (s *Service) BatchRedeem(ctx context.Context, req BatchRedeemRequest) (*BatchRedeemResponse, error) {
	req.UserID = strings.TrimSpace(req.UserID)
	if req.UserID == "" {
		return batchFailure("请输入邮箱"), nil
	}
	if utf8.RuneCountInString(req.UserID) > MaxUserIDLength {
		return batchFailure("邮箱过长"), nil
	}
	if len(req.Codes) == 0 || len(req.Codes) > MaxBatchCodes {
		return batchFailure(fmt.Sprintf("每次请输入 1-%d 个兑换码", MaxBatchCodes)), nil
	}
	if !validIdempotencyKey(req.IdempotencyKey) {
		return batchFailure("幂等 key 无效或过长"), nil
	}

	var idempotencyTx pgx.Tx
	var idempotencyQueries *store.Queries
	if req.IdempotencyKey != "" {
		var err error
		idempotencyTx, err = s.pool.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("begin batch idempotency transaction: %w", err)
		}
		defer idempotencyTx.Rollback(ctx)
		idempotencyQueries = s.queries.WithTx(idempotencyTx)
		stored, err := s.handleBatchIdempotency(ctx, idempotencyQueries, req)
		if err != nil {
			return nil, err
		}
		if stored != nil {
			return stored, nil
		}
	}

	resp := &BatchRedeemResponse{
		OK:      true,
		Message: "批量兑换处理完成",
		Total:   len(req.Codes),
		Results: make([]BatchRedeemItem, 0, len(req.Codes)),
	}
	seen := make(map[string]struct{}, len(req.Codes))
	for _, inputCode := range req.Codes {
		normalizedCode := NormalizeCode(inputCode)
		item := BatchRedeemItem{Code: strings.TrimSpace(inputCode)}
		if normalizedCode == "" {
			item.Result = ResultInvalidInput
			item.Message = "兑换码不能为空"
		} else if len(normalizedCode) > MaxCodeLength {
			item.Result = ResultInvalidInput
			item.Message = "兑换码过长"
		} else if _, duplicate := seen[normalizedCode]; duplicate {
			item.Result = ResultInvalidInput
			item.Message = "兑换码重复，已跳过"
		} else {
			seen[normalizedCode] = struct{}{}
			itemResponse, err := s.Redeem(ctx, RedeemRequest{
				UserID: req.UserID,
				Code:   inputCode,
				IP:     req.IP,
			})
			if err != nil {
				return nil, fmt.Errorf("redeem batch code %q: %w", normalizedCode, err)
			}
			item.OK = itemResponse.OK
			item.Result = itemResponse.Result
			item.Message = itemResponse.Message
			if itemResponse.OK {
				item.Code = itemResponse.Code
				item.Redemption = itemResponse
			}
		}

		if item.OK {
			resp.Succeeded++
		} else {
			resp.Failed++
		}
		resp.Results = append(resp.Results, item)
	}

	if req.IdempotencyKey != "" {
		if err := s.storeBatchIdempotencyResponse(ctx, idempotencyQueries, req.IdempotencyKey, resp); err != nil {
			return nil, err
		}
		if err := idempotencyTx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit batch idempotency transaction: %w", err)
		}
		idempotencyTx = nil
	}
	return resp, nil
}

// Redeem attempts to redeem a code atomically.
func (s *Service) Redeem(ctx context.Context, req RedeemRequest) (*RedeemResponse, error) {
	req.UserID = strings.TrimSpace(req.UserID)
	code := NormalizeCode(req.Code)
	userKey := NormalizeUserKey(req.UserID)
	if req.UserID == "" || code == "" {
		return failure(ResultInvalidInput, "请输入邮箱和兑换码"), nil
	}
	if utf8.RuneCountInString(req.UserID) > MaxUserIDLength {
		return failure(ResultInvalidInput, "邮箱过长"), nil
	}
	if len(code) > MaxCodeLength {
		return failure(ResultInvalidInput, "兑换码过长"), nil
	}
	if !validIdempotencyKey(req.IdempotencyKey) {
		return failure(ResultInvalidInput, "幂等 key 无效或过长"), nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := s.queries.WithTx(tx)
	if req.IdempotencyKey != "" {
		stored, err := s.handleIdempotency(ctx, qtx, req.IdempotencyKey, req)
		if err != nil {
			return nil, err
		}
		if stored != nil {
			return stored, nil
		}
	}

	codeRow, err := qtx.GetCodeByCode(ctx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			resp := failure(ResultInvalidCode, "兑换码无效，请核对后重试")
			if err := s.recordFailure(ctx, qtx, req, code, userKey, nil, nil, resp); err != nil {
				return nil, err
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return resp, nil
		}
		return nil, fmt.Errorf("get code: %w", err)
	}

	// Fail-fast checks.
	if resp := s.checkCodeState(codeRow); resp != nil {
		if err := s.recordFailure(ctx, qtx, req, code, userKey, &codeRow.ID, &codeRow.BatchID, resp); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return resp, nil
	}

	var credential *store.Credential
	if codeRow.AssignCredential {
		cred, err := qtx.GetUnusedCredential(ctx, codeRow.CredentialPlanType)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				resp := failure(ResultCredentialUnavailable, "当前套餐凭证库存不足，请稍后重试")
				if err := s.recordFailure(ctx, qtx, req, code, userKey, &codeRow.ID, &codeRow.BatchID, resp); err != nil {
					return nil, err
				}
				if err := tx.Commit(ctx); err != nil {
					return nil, fmt.Errorf("commit credential unavailable result: %w", err)
				}
				return resp, nil
			}
			return nil, fmt.Errorf("get unused credential: %w", err)
		}
		credential = &cred
	}

	_, err = qtx.IncrementCodeUseCount(ctx, store.IncrementCodeUseCountParams{
		ID:       codeRow.ID,
		UseCount: codeRow.MaxUsesPerCode,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("code state changed while row was locked")
		}
		return nil, fmt.Errorf("increment code use count: %w", err)
	}

	// Success: create redemption and outbox event.
	redeemedAt := time.Now().UTC()
	eventID := ""
	webhookPending := codeRow.WebhookUrl != ""
	if webhookPending {
		eventID = newEventID()
	}

	redemption, err := qtx.CreateRedemption(ctx, store.CreateRedemptionParams{
		CodeID:          &codeRow.ID,
		BatchID:         &codeRow.BatchID,
		CodeText:        code,
		UserID:          req.UserID,
		UserKey:         userKey,
		PayloadSnapshot: codeRow.PayloadJson,
		Result:          store.RedemptionResultSuccess,
		Message:         "兑换成功 🎉",
		WebhookStatus:   webhookStatus(webhookPending),
		WebhookAttempt:  0,
		Ip:              pgTextToInet(req.IP),
		IdempotencyKey:  stringPtr(req.IdempotencyKey),
		EventID:         stringPtr(eventID),
	})
	if err != nil {
		return nil, fmt.Errorf("create redemption: %w", err)
	}

	if webhookPending {
		_, err = qtx.CreateWebhookOutboxEvent(ctx, store.CreateWebhookOutboxEventParams{
			EventID:      eventID,
			RedemptionID: redemption.ID,
		})
		if err != nil {
			return nil, fmt.Errorf("create outbox event: %w", err)
		}
	}

	var credentialData json.RawMessage
	if credential != nil {
		_, err := qtx.MarkCredentialUsed(ctx, store.MarkCredentialUsedParams{
			ID:           credential.ID,
			RedemptionID: &redemption.ID,
		})
		if err != nil {
			return nil, fmt.Errorf("mark credential used: %w", err)
		}
		credentialData = credential.Data
	}

	// Store idempotency response if applicable.
	resp := &RedeemResponse{
		OK:             true,
		Result:         ResultSuccess,
		Message:        "兑换成功 🎉",
		Batch:          codeRow.BatchName,
		Code:           DisplayCode(code, codeRow.Prefix),
		UserID:         req.UserID,
		Payload:        codeRow.PayloadJson,
		Credential:     credentialData,
		RedeemedAt:     redeemedAt.Format("2006-01-02 15:04:05"),
		EventID:        eventID,
		WebhookPending: webhookPending,
	}
	if req.IdempotencyKey != "" {
		if err := s.storeIdempotencyResponse(ctx, qtx, req.IdempotencyKey, resp); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}
	return resp, nil
}

func (s *Service) checkCodeState(row store.GetCodeByCodeRow) *RedeemResponse {
	if row.Status == store.CodeStatusDisabled {
		return failure(ResultCodeDisabled, "该兑换码已被禁用")
	}
	if row.BatchStatus == store.BatchStatusDisabled {
		return failure(ResultBatchDisabled, "该兑换码所属批次已停用")
	}
	if row.ExpiresAt.Valid && row.ExpiresAt.Time.Before(time.Now().UTC()) {
		return failure(ResultExpired, "该兑换码已过期")
	}
	if row.UseCount >= row.MaxUsesPerCode {
		return failure(ResultCodeUsedUp, "该兑换码已被使用完")
	}
	return nil
}

func (s *Service) recordFailure(ctx context.Context, qtx *store.Queries, req RedeemRequest, code, userKey string, codeID, batchID *int64, resp *RedeemResponse) error {
	result := store.RedemptionResult(resp.Result)
	_, err := qtx.CreateRedemption(ctx, store.CreateRedemptionParams{
		CodeID:          codeID,
		BatchID:         batchID,
		CodeText:        code,
		UserID:          req.UserID,
		UserKey:         userKey,
		PayloadSnapshot: json.RawMessage("{}"),
		Result:          result,
		Message:         resp.Message,
		WebhookStatus:   store.WebhookStatusNone,
		WebhookAttempt:  0,
		Ip:              pgTextToInet(req.IP),
		IdempotencyKey:  stringPtr(req.IdempotencyKey),
	})
	if err != nil {
		return fmt.Errorf("record failure redemption: %w", err)
	}
	if req.IdempotencyKey != "" {
		if err := s.storeIdempotencyResponse(ctx, qtx, req.IdempotencyKey, resp); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) handleIdempotency(ctx context.Context, qtx *store.Queries, key string, req RedeemRequest) (*RedeemResponse, error) {
	requestHash := hashRequest(req)

	_, err := qtx.CreateIdempotencyKey(ctx, store.CreateIdempotencyKeyParams{
		Key:         key,
		RequestHash: requestHash,
	})
	if err == nil {
		return nil, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("create idempotency key: %w", err)
	}

	existing, err := qtx.LockIdempotencyKeyByKey(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("lock idempotency key: %w", err)
	}

	if !equalHash(existing.RequestHash, requestHash) {
		return nil, ErrIdempotencyConflict
	}
	if len(existing.ResponseBody) == 0 {
		return nil, nil
	}

	var resp RedeemResponse
	if err := json.Unmarshal(existing.ResponseBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal idempotency response: %w", err)
	}
	return &resp, nil
}

func equalHash(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var different byte
	for i := range a {
		different |= a[i] ^ b[i]
	}
	return different == 0
}

func (s *Service) storeIdempotencyResponse(ctx context.Context, qtx *store.Queries, key string, resp *RedeemResponse) error {
	body, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal idempotency response: %w", err)
	}
	return qtx.SetIdempotencyResponse(ctx, store.SetIdempotencyResponseParams{
		Key:          key,
		ResponseBody: body,
	})
}

func (s *Service) handleBatchIdempotency(ctx context.Context, qtx *store.Queries, req BatchRedeemRequest) (*BatchRedeemResponse, error) {
	requestHash := hashBatchRequest(req)
	_, err := qtx.CreateIdempotencyKey(ctx, store.CreateIdempotencyKeyParams{
		Key:         req.IdempotencyKey,
		RequestHash: requestHash,
	})
	if err == nil {
		return nil, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("create batch idempotency key: %w", err)
	}
	existing, err := qtx.LockIdempotencyKeyByKey(ctx, req.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("lock batch idempotency key: %w", err)
	}
	if !equalHash(existing.RequestHash, requestHash) {
		return nil, ErrIdempotencyConflict
	}
	if len(existing.ResponseBody) == 0 {
		return nil, nil
	}
	var resp BatchRedeemResponse
	if err := json.Unmarshal(existing.ResponseBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal batch idempotency response: %w", err)
	}
	return &resp, nil
}

func (s *Service) storeBatchIdempotencyResponse(ctx context.Context, qtx *store.Queries, key string, resp *BatchRedeemResponse) error {
	body, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal batch idempotency response: %w", err)
	}
	if err := qtx.SetIdempotencyResponse(ctx, store.SetIdempotencyResponseParams{Key: key, ResponseBody: body}); err != nil {
		return fmt.Errorf("store batch idempotency response: %w", err)
	}
	return nil
}

func failure(result, message string) *RedeemResponse {
	return &RedeemResponse{OK: false, Result: result, Message: message}
}

func batchFailure(message string) *BatchRedeemResponse {
	return &BatchRedeemResponse{OK: false, Message: message}
}

func webhookStatus(pending bool) store.WebhookStatus {
	if pending {
		return store.WebhookStatusPending
	}
	return store.WebhookStatusNone
}

func hashRequest(req RedeemRequest) []byte {
	h := sha256.New()
	_, _ = h.Write([]byte(req.UserID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(NormalizeCode(req.Code)))
	return h.Sum(nil)
}

func hashBatchRequest(req BatchRedeemRequest) []byte {
	h := sha256.New()
	_, _ = h.Write([]byte("batch\x00"))
	_, _ = h.Write([]byte(req.UserID))
	for _, code := range req.Codes {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(NormalizeCode(code)))
	}
	return h.Sum(nil)
}

func validIdempotencyKey(key string) bool {
	if key == "" {
		return true
	}
	return len(key) <= MaxIdempotencyKeyLength && key == strings.TrimSpace(key)
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func newEventID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// LimitJSONBody wraps the request body with a maximum size reader.
func LimitJSONBody(r *http.Request) io.Reader {
	return http.MaxBytesReader(nil, r.Body, MaxPayloadSize)
}

var codeNormalizer = regexp.MustCompile(`[^A-Za-z0-9]`)

// NormalizeCode strips non-alphanumeric characters and uppercases the code.
func NormalizeCode(raw string) string {
	return strings.ToUpper(codeNormalizer.ReplaceAllString(raw, ""))
}

// NormalizeUserKey normalizes a user identifier into a stable key.
func NormalizeUserKey(raw string) string {
	s := strings.TrimSpace(raw)
	// Unicode NFC normalization would require golang.org/x/text/unicode/norm.
	// For now, trim and lowercase.
	s = strings.ToLower(s)
	// Remove leading/trailing non-printables.
	return strings.TrimFunc(s, func(r rune) bool {
		return !unicode.IsPrint(r) || unicode.IsSpace(r)
	})
}

// DisplayCode formats a code with prefix and dash grouping.
func DisplayCode(code, prefix string) string {
	prefix = strings.ToUpper(prefix)
	if prefix != "" && strings.HasPrefix(code, prefix) {
		return prefix + "-" + formatGroups(code[len(prefix):])
	}
	return formatGroups(code)
}

func formatGroups(s string) string {
	if len(s) <= 4 {
		return s
	}
	var parts []string
	for i := 0; i < len(s); i += 4 {
		end := i + 4
		if end > len(s) {
			end = len(s)
		}
		parts = append(parts, s[i:end])
	}
	return strings.Join(parts, "-")
}

func pgTextToInet(ip string) netip.Addr {
	if ip == "" {
		return netip.MustParseAddr("0.0.0.0")
	}
	if addr, err := netip.ParseAddr(ip); err == nil {
		return addr
	}
	return netip.MustParseAddr("0.0.0.0")
}
