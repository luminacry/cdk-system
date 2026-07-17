package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"cdk-system/internal/observability"
	"cdk-system/internal/store"
)

// CredentialsHandler manages JSON credential inventory.
type CredentialsHandler struct {
	pool             *pgxpool.Pool
	queries          *store.Queries
	credentialLookup credentialLookup
	credentialList   credentialList
	credentialPlans  credentialPlans
	credentialImport credentialImportFunc
}

type credentialLookup interface {
	GetCredentialByID(context.Context, int64) (store.Credential, error)
}

type credentialList interface {
	ListCredentials(context.Context, store.ListCredentialsParams) ([]store.ListCredentialsRow, error)
	CountCredentials(context.Context, string) (int64, error)
}

type credentialPlans interface {
	ListCredentialPlans(context.Context) ([]store.ListCredentialPlansRow, error)
}

type credentialImportFunc func(context.Context, []credentialItem) (int, error)

// NewCredentialsHandler creates a new CredentialsHandler.
func NewCredentialsHandler(pool *pgxpool.Pool, queries *store.Queries) *CredentialsHandler {
	handler := &CredentialsHandler{
		pool:             pool,
		queries:          queries,
		credentialLookup: queries,
		credentialList:   queries,
		credentialPlans:  queries,
	}
	handler.credentialImport = handler.importCredentials
	return handler
}

const defaultCredentialsPageSize int32 = 50

var allowedCredentialPageSizes = map[int32]struct{}{
	20:  {},
	50:  {},
	100: {},
	200: {},
	500: {},
}

var errInvalidCredentialID = errors.New("invalid credential ID")

// ListCredentials handles GET /api/admin/credentials.
func (h *CredentialsHandler) ListCredentials(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	state := r.URL.Query().Get("state")
	if state == "" {
		state = "all"
	}
	if state != "all" && state != "used" && state != "unused" {
		RespondError(w, http.StatusBadRequest, "无效的状态筛选")
		return
	}

	page, pageSize, offset, err := parseCredentialPagination(
		r.URL.Query().Get("page"),
		r.URL.Query().Get("page_size"),
	)
	if err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	items, err := h.credentialList.ListCredentials(ctx, store.ListCredentialsParams{
		StateFilter: state,
		PageSize:    pageSize,
		PageOffset:  offset,
	})
	if err != nil {
		observability.Logger(ctx).Error("list credentials failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "获取凭证列表失败")
		return
	}

	total, err := h.credentialList.CountCredentials(ctx, state)
	if err != nil {
		observability.Logger(ctx).Error("count credentials failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "获取凭证列表失败")
		return
	}

	pages := int(total / int64(pageSize))
	if total%int64(pageSize) != 0 {
		pages++
	}

	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"items":     items,
		"total":     total,
		"page":      page,
		"pages":     pages,
		"page_size": pageSize,
	})
}

func parseCredentialPagination(pageRaw, pageSizeRaw string) (int, int32, int32, error) {
	pageSize := defaultCredentialsPageSize
	if pageSizeRaw != "" {
		parsed, err := strconv.ParseInt(pageSizeRaw, 10, 32)
		if err != nil {
			return 0, 0, 0, errors.New("page_size 必须是 20、50、100、200 或 500")
		}
		pageSize = int32(parsed)
		if _, ok := allowedCredentialPageSizes[pageSize]; !ok {
			return 0, 0, 0, errors.New("page_size 必须是 20、50、100、200 或 500")
		}
	}

	page := int64(1)
	if pageRaw != "" {
		parsed, err := strconv.ParseInt(pageRaw, 10, 64)
		if err == nil && parsed > 0 {
			page = parsed
		}
	}

	const maxOffset = int64(1<<31 - 1)
	if page-1 > maxOffset/int64(pageSize) {
		return 0, 0, 0, errors.New("页码超出范围")
	}
	offset := (page - 1) * int64(pageSize)
	return int(page), pageSize, int32(offset), nil
}

// StatsCredentials handles GET /api/admin/credentials/stats.
func (h *CredentialsHandler) StatsCredentials(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats, err := h.queries.CountCredentialsByStatus(ctx)
	if err != nil {
		observability.Logger(ctx).Error("count credentials by status failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "获取凭证统计失败")
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"total":  stats.Total,
		"used":   stats.Used,
		"unused": stats.Unused,
	})
}

// ListCredentialPlans handles GET /api/admin/credentials/plans.
func (h *CredentialsHandler) ListCredentialPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.credentialPlans.ListCredentialPlans(r.Context())
	if err != nil {
		observability.Logger(r.Context()).Error("list credential plans failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "获取凭证套餐失败")
		return
	}
	if plans == nil {
		plans = []store.ListCredentialPlansRow{}
	}

	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"plans": plans,
	})
}

// credentialItem is the expected shape of each uploaded credential object.
type credentialItem struct {
	Email    string             `json:"email"`
	PlanType string             `json:"plan_type"`
	Expired  pgtype.Timestamptz `json:"expired"`
	Data     json.RawMessage    `json:"-"`
}

// UploadCredentials handles POST /api/admin/credentials/upload.
func (h *CredentialsHandler) UploadCredentials(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	const maxFileSize = 16 << 20 // 16 MiB
	r.Body = http.MaxBytesReader(w, r.Body, maxFileSize)

	var reader io.Reader = r.Body
	var selectedPlanType string
	contentType := r.Header.Get("Content-Type")
	mediaType := ""
	if contentType != "" {
		var err error
		mediaType, _, err = mime.ParseMediaType(contentType)
		if err != nil {
			RespondError(w, http.StatusBadRequest, "无效的 Content-Type", err.Error())
			return
		}
	}
	if mediaType == "multipart/form-data" {
		if err := r.ParseMultipartForm(maxFileSize); err != nil {
			RespondError(w, http.StatusBadRequest, "解析上传文件失败", err.Error())
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
			if values := r.MultipartForm.Value["plan_type"]; len(values) > 0 {
				selectedPlanType = values[0]
			}
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			RespondError(w, http.StatusBadRequest, "请上传文件")
			return
		}
		defer file.Close()
		reader = file
	} else {
		selectedPlanType = r.URL.Query().Get("plan_type")
	}

	selectedPlanType, err := normalizeCredentialPlanType(selectedPlanType)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "套餐类型无效", err.Error())
		return
	}

	var rawItems []map[string]any
	if err := json.NewDecoder(reader).Decode(&rawItems); err != nil {
		RespondError(w, http.StatusBadRequest, "文件必须是 JSON 数组", err.Error())
		return
	}

	items, skipped := h.normalizeCredentials(rawItems, selectedPlanType)

	imported, err := h.credentialImport(ctx, items)
	if err != nil {
		observability.Logger(ctx).Error("import credentials failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "导入失败")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"imported": imported,
		"skipped":  skipped,
		"message":  fmt.Sprintf("成功导入 %d 条凭证，跳过 %d 条", imported, skipped),
	})
}

func (h *CredentialsHandler) importCredentials(ctx context.Context, items []credentialItem) (int, error) {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := h.queries.WithTx(tx)
	for _, item := range items {
		if _, err := qtx.CreateCredential(ctx, store.CreateCredentialParams{
			Email:    item.Email,
			PlanType: item.PlanType,
			Expired:  item.Expired,
			Data:     item.Data,
		}); err != nil {
			return 0, fmt.Errorf("create credential: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}
	return len(items), nil
}

func (h *CredentialsHandler) normalizeCredentials(rawItems []map[string]any, selectedPlanType string) ([]credentialItem, int) {
	items := make([]credentialItem, 0, len(rawItems))
	skipped := 0
	for _, raw := range rawItems {
		email, _ := raw["email"].(string)
		email = strings.TrimSpace(email)
		if email == "" || utf8.RuneCountInString(email) > 320 {
			skipped++
			continue
		}
		raw["email"] = email
		raw["plan_type"] = selectedPlanType

		var expired pgtype.Timestamptz
		if rawExpired, exists := raw["expired"]; exists && rawExpired != nil {
			exp, ok := rawExpired.(string)
			if !ok {
				skipped++
				continue
			}
			if exp != "" {
				t, err := parseCredentialTime(exp)
				if err != nil {
					skipped++
					continue
				}
				expired = pgtype.Timestamptz{Valid: true, Time: t}
			}
		}

		data, err := json.Marshal(raw)
		if err != nil {
			skipped++
			continue
		}

		items = append(items, credentialItem{
			Email:    email,
			PlanType: selectedPlanType,
			Expired:  expired,
			Data:     data,
		})
	}
	return items, skipped
}

func normalizeCredentialPlanType(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", errors.New("套餐类型必须是有效 UTF-8 文本")
	}

	value = strings.TrimSpace(value)
	runeCount := utf8.RuneCountInString(value)
	if runeCount < 1 || runeCount > 100 {
		return "", errors.New("套餐类型长度必须为 1-100 个字符")
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return "", errors.New("套餐类型不能包含控制字符")
		}
	}
	return value, nil
}

func parseCredentialTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, errors.New("invalid time format")
}

type credentialDetail struct {
	ID           int64              `json:"id"`
	Email        string             `json:"email"`
	PlanType     string             `json:"plan_type"`
	Expired      pgtype.Timestamptz `json:"expired"`
	Used         bool               `json:"used"`
	RedemptionID *int64             `json:"redemption_id"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

func newCredentialDetail(credential store.Credential) credentialDetail {
	return credentialDetail{
		ID:           credential.ID,
		Email:        credential.Email,
		PlanType:     credential.PlanType,
		Expired:      credential.Expired,
		Used:         credential.Used,
		RedemptionID: credential.RedemptionID,
		CreatedAt:    credential.CreatedAt,
		UpdatedAt:    credential.UpdatedAt,
	}
}

func (h *CredentialsHandler) findCredential(r *http.Request) (store.Credential, int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		return store.Credential{}, 0, errInvalidCredentialID
	}
	credential, err := h.credentialLookup.GetCredentialByID(r.Context(), id)
	return credential, id, err
}

// GetCredential handles GET /api/admin/credentials/{id} without exposing secret data.
func (h *CredentialsHandler) GetCredential(w http.ResponseWriter, r *http.Request) {
	credential, _, err := h.findCredential(r)
	if err != nil {
		switch {
		case errors.Is(err, errInvalidCredentialID):
			RespondError(w, http.StatusBadRequest, "无效凭证 ID")
		case errors.Is(err, pgx.ErrNoRows):
			RespondError(w, http.StatusNotFound, "凭证不存在")
		default:
			observability.Logger(r.Context()).Error("get credential failed", "error", err)
			RespondError(w, http.StatusInternalServerError, "获取凭证详情失败")
		}
		return
	}

	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"credential": newCredentialDetail(credential),
	})
}

// DownloadCredential handles GET /api/admin/credentials/{id}/download.
func (h *CredentialsHandler) DownloadCredential(w http.ResponseWriter, r *http.Request) {
	credential, id, err := h.findCredential(r)
	if err != nil {
		switch {
		case errors.Is(err, errInvalidCredentialID):
			RespondError(w, http.StatusBadRequest, "无效凭证 ID")
		case errors.Is(err, pgx.ErrNoRows):
			RespondError(w, http.StatusNotFound, "凭证不存在")
		default:
			observability.Logger(r.Context()).Error("download credential failed", "error", err)
			RespondError(w, http.StatusInternalServerError, "下载凭证失败")
		}
		return
	}
	if !json.Valid(credential.Data) {
		observability.Logger(r.Context()).Error("credential contains invalid JSON", "credential_id", id)
		RespondError(w, http.StatusInternalServerError, "下载凭证失败")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="credential-%d.json"`, id))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(credential.Data)
}

// DeleteCredential handles DELETE /api/admin/credentials/{id}.
func (h *CredentialsHandler) DeleteCredential(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "无效凭证 ID")
		return
	}
	if err := h.queries.DeleteCredential(ctx, id); err != nil {
		observability.Logger(ctx).Error("delete credential failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "删除失败")
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "删除成功",
	})
}

type batchDeleteCredentialsRequest struct {
	IDs []int64 `json:"ids"`
}

// BatchDeleteCredentials handles POST /api/admin/credentials/batch-delete.
func (h *CredentialsHandler) BatchDeleteCredentials(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req batchDeleteCredentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "请求格式错误", err.Error())
		return
	}
	if len(req.IDs) == 0 {
		RespondError(w, http.StatusBadRequest, "请选择要删除的凭证")
		return
	}

	deleted, err := h.queries.DeleteCredentialsByIDs(ctx, req.IDs)
	if err != nil {
		observability.Logger(ctx).Error("batch delete credentials failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "批量删除失败")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"deleted": deleted,
		"message": fmt.Sprintf("已删除 %d 条凭证", deleted),
	})
}
