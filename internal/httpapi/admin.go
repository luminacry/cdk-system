package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cdk-system/internal/batch"
	"cdk-system/internal/observability"
	"cdk-system/internal/redeem"
	"cdk-system/internal/store"
	"cdk-system/internal/webhook"
)

// AdminHandler holds dependencies for admin endpoints.
type AdminHandler struct {
	pool                   *pgxpool.Pool
	queries                *store.Queries
	worker                 *webhook.Worker
	credentialAvailability availableCredentialCounter
	batchCreate            batchCreateFunc
}

type availableCredentialCounter interface {
	CountAvailableCredentialsByPlan(context.Context, string) (int64, error)
}

type batchCreateFunc func(context.Context, batch.CreateBatchRequest) (int64, error)

// NewAdminHandler creates a new AdminHandler.
func NewAdminHandler(pool *pgxpool.Pool, queries *store.Queries, worker *webhook.Worker) *AdminHandler {
	handler := &AdminHandler{
		pool:                   pool,
		queries:                queries,
		worker:                 worker,
		credentialAvailability: queries,
	}
	handler.batchCreate = handler.createBatch
	return handler
}

// Stats handles GET /api/admin/stats.
func (h *AdminHandler) Stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats, err := h.queries.GetStats(ctx)
	if err != nil {
		observability.Logger(ctx).Error("get stats failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "获取统计失败")
		return
	}

	daily, err := h.queries.GetDailyRedemptions(ctx)
	if err != nil {
		observability.Logger(ctx).Error("get daily stats failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "获取统计失败")
		return
	}

	recent, err := h.queries.GetRecentRedemptions(ctx)
	if err != nil {
		observability.Logger(ctx).Error("get recent redemptions failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "获取统计失败")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"stats":  stats,
		"daily":  dailyResponses(daily),
		"recent": recentRedemptionResponses(recent),
	})
}

// ListBatches handles GET /api/admin/batches.
func (h *AdminHandler) ListBatches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	batches, err := h.queries.ListBatches(ctx)
	if err != nil {
		observability.Logger(ctx).Error("list batches failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "获取批次失败")
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"ok": true, "batches": batchListResponses(batches)})
}

type createBatchRequest struct {
	Name               string  `json:"name"`
	Description        string  `json:"description"`
	PayloadJSON        string  `json:"payload_json"`
	Prefix             string  `json:"prefix"`
	CodeLength         int32   `json:"code_length"`
	Count              int32   `json:"count"`
	MaxUsesPerCode     int32   `json:"max_uses_per_code"`
	MaxRedeemsPerUser  int32   `json:"max_redeems_per_user"`
	ExpiresAt          *string `json:"expires_at"`
	WebhookURL         string  `json:"webhook_url"`
	WebhookSecret      string  `json:"webhook_secret"`
	AssignCredential   bool    `json:"assign_credential"`
	CredentialPlanType string  `json:"credential_plan_type"`
}

// CreateBatch handles POST /api/admin/batches.
func (h *AdminHandler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req createBatchRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "请求格式错误", err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Prefix = strings.ToUpper(strings.TrimSpace(req.Prefix))
	req.WebhookURL = strings.TrimSpace(req.WebhookURL)
	req.CredentialPlanType = strings.TrimSpace(req.CredentialPlanType)

	errors := validateCreateBatch(req)
	if len(errors) > 0 {
		RespondError(w, http.StatusBadRequest, "输入校验失败", errors...)
		return
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			RespondError(w, http.StatusBadRequest, "过期时间格式错误")
			return
		}
		expiresAt = &t
	}

	// Auto-generate webhook secret if URL is provided without secret.
	webhookSecret := req.WebhookSecret
	if req.WebhookURL != "" && webhookSecret == "" {
		webhookSecret = generateHex(16)
	}

	batchRequest := batch.CreateBatchRequest{
		Name:               req.Name,
		Description:        req.Description,
		PayloadJSON:        req.PayloadJSON,
		Prefix:             req.Prefix,
		CodeLength:         req.CodeLength,
		Count:              req.Count,
		MaxUsesPerCode:     req.MaxUsesPerCode,
		MaxRedeemsPerUser:  req.MaxRedeemsPerUser,
		ExpiresAt:          expiresAt,
		WebhookURL:         req.WebhookURL,
		WebhookSecret:      webhookSecret,
		AssignCredential:   req.AssignCredential,
		CredentialPlanType: req.CredentialPlanType,
	}

	if req.AssignCredential {
		available, err := h.credentialAvailability.CountAvailableCredentialsByPlan(ctx, req.CredentialPlanType)
		if err != nil {
			observability.Logger(ctx).Error("count available credentials failed", "error", err)
			RespondError(w, http.StatusInternalServerError, "检查凭证库存失败")
			return
		}
		if available < 1 {
			RespondError(w, http.StatusBadRequest, "所选套餐没有可用凭证")
			return
		}
	}

	batchID, err := h.batchCreate(ctx, batchRequest)
	if err != nil {
		observability.Logger(ctx).Error("create batch failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "创建批次失败")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"batch_id": batchID,
		"message":  "批次创建成功",
	})
}

func (h *AdminHandler) createBatch(ctx context.Context, req batch.CreateBatchRequest) (int64, error) {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	batchID, err := batch.CreateBatch(ctx, tx, h.queries.WithTx(tx), req)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}
	return batchID, nil
}

func validateCreateBatch(req createBatchRequest) []string {
	var errs []string
	if strings.TrimSpace(req.Name) == "" {
		errs = append(errs, "批次名称不能为空")
	}
	if utf8.RuneCountInString(req.Name) > 200 {
		errs = append(errs, "批次名称不能超过 200 个字符")
	}
	if utf8.RuneCountInString(req.Description) > 1000 {
		errs = append(errs, "备注说明不能超过 1000 个字符")
	}
	if req.CodeLength < 8 || req.CodeLength > 20 {
		errs = append(errs, "码长度必须在 8-20 之间")
	}
	if req.Count < 1 || req.Count > 10000 {
		errs = append(errs, "生成数量必须在 1-10000 之间")
	}
	if req.MaxUsesPerCode < 1 || req.MaxUsesPerCode > 100000 {
		errs = append(errs, "单码可用次数必须在 1-100000 之间")
	}
	if req.MaxRedeemsPerUser < 1 || req.MaxRedeemsPerUser > 100000 {
		errs = append(errs, "用户限兑次数必须在 1-100000 之间")
	}
	if len(req.Prefix) > 8 {
		errs = append(errs, "前缀长度不能超过 8")
	}
	if req.Prefix != "" && !isASCIIAlphanumeric(req.Prefix) {
		errs = append(errs, "前缀只能包含英文字母和数字")
	}
	if req.WebhookURL != "" {
		if len(req.WebhookURL) > 2000 {
			errs = append(errs, "Webhook URL 过长")
		} else if err := webhook.ValidateURL(req.WebhookURL, nil); err != nil {
			errs = append(errs, "Webhook 地址必须是可访问的公网 HTTPS 地址")
		}
	}
	if len(req.WebhookSecret) > 512 {
		errs = append(errs, "Webhook 密钥不能超过 512 个字符")
	}
	if req.AssignCredential {
		if _, err := normalizeCredentialPlanType(req.CredentialPlanType); err != nil {
			errs = append(errs, err.Error())
		}
	} else if utf8.RuneCountInString(req.CredentialPlanType) > 100 {
		errs = append(errs, "凭证套餐类型过长")
	}
	if req.PayloadJSON != "" {
		var payload any
		if err := json.Unmarshal([]byte(req.PayloadJSON), &payload); err != nil {
			errs = append(errs, "奖励内容必须是合法 JSON")
		}
	}
	return errs
}

func isASCIIAlphanumeric(value string) bool {
	for _, char := range value {
		if (char < 'A' || char > 'Z') && (char < '0' || char > '9') {
			return false
		}
	}
	return true
}

// GetBatch handles GET /api/admin/batches/:id.
func (h *AdminHandler) GetBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := parseBatchID(r)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "无效批次 ID")
		return
	}

	batch, err := h.queries.GetBatchByID(ctx, id)
	if err != nil {
		RespondError(w, http.StatusNotFound, "批次不存在")
		return
	}

	counts, err := h.queries.GetBatchCounts(ctx, id)
	if err != nil {
		observability.Logger(ctx).Error("get batch counts failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "获取批次统计失败")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"batch":  batchModelResponse(batch),
		"counts": counts,
	})
}

// ListCodes handles GET /api/admin/batches/:id/codes.
func (h *AdminHandler) ListCodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := parseBatchID(r)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "无效批次 ID")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage := 50
	offset := int32((page - 1) * perPage)

	state := r.URL.Query().Get("state")
	if state == "" {
		state = "all"
	}

	var codes []store.ListCodesByBatchRow
	var total int64

	if state == "all" {
		codes, err = h.queries.ListCodesByBatch(ctx, store.ListCodesByBatchParams{
			BatchID: id,
			Limit:   int32(perPage),
			Offset:  offset,
		})
		if err != nil {
			observability.Logger(ctx).Error("list codes failed", "error", err)
			RespondError(w, http.StatusInternalServerError, "获取兑换码失败")
			return
		}
		total, err = h.queries.CountCodesByBatch(ctx, id)
	} else {
		allowed := map[string]bool{"active": true, "partused": true, "used": true, "disabled": true}
		if !allowed[state] {
			RespondError(w, http.StatusBadRequest, "无效的状态筛选")
			return
		}
		codesFiltered, err := h.queries.ListCodesByBatchAndState(ctx, store.ListCodesByBatchAndStateParams{
			BatchID:     id,
			State:       state,
			LimitCount:  int32(perPage),
			OffsetCount: offset,
		})
		if err != nil {
			observability.Logger(ctx).Error("list codes failed", "error", err)
			RespondError(w, http.StatusInternalServerError, "获取兑换码失败")
			return
		}
		// Adapt the typed result to the generic row shape used by the handler.
		codes = make([]store.ListCodesByBatchRow, len(codesFiltered))
		for i, c := range codesFiltered {
			codes[i] = store.ListCodesByBatchRow(c)
		}
		total, err = h.queries.CountCodesByBatchAndState(ctx, store.CountCodesByBatchAndStateParams{
			BatchID: id,
			State:   state,
		})
	}
	if err != nil {
		observability.Logger(ctx).Error("count codes failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "获取兑换码失败")
		return
	}

	pages := int(total) / perPage
	if int(total)%perPage > 0 {
		pages++
	}

	batch, err := h.queries.GetBatchByID(ctx, id)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "获取批次失败")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"codes":    codeResponses(codes, batch.Prefix),
		"max_uses": batch.MaxUsesPerCode,
		"total":    total,
		"page":     page,
		"pages":    pages,
	})
}

// ToggleBatch handles POST /api/admin/batches/:id/toggle.
func (h *AdminHandler) ToggleBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := parseBatchID(r)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "无效批次 ID")
		return
	}

	batch, err := h.queries.ToggleBatchStatus(ctx, id)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "切换批次状态失败")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"status":  batch.Status,
		"message": fmt.Sprintf("批次已%s", statusText(batch.Status)),
	})
}

// ToggleCode handles POST /api/admin/codes/:id/toggle.
func (h *AdminHandler) ToggleCode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "无效码 ID")
		return
	}

	code, err := h.queries.ToggleCodeStatus(ctx, id)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "切换码状态失败")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"status":  code.Status,
		"message": fmt.Sprintf("兑换码已%s", codeStatusText(code.Status)),
	})
}

// ExportBatch handles GET /api/admin/batches/:id/export.
func (h *AdminHandler) ExportBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := parseBatchID(r)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "无效批次 ID")
		return
	}

	fmtParam := r.URL.Query().Get("fmt")
	if fmtParam != "csv" {
		fmtParam = "txt"
	}

	batch, err := h.queries.GetBatchByID(ctx, id)
	if err != nil {
		RespondError(w, http.StatusNotFound, "批次不存在")
		return
	}

	codes, err := h.queries.ListCodesByBatch(ctx, store.ListCodesByBatchParams{
		BatchID: id,
		Limit:   100000,
		Offset:  0,
	})
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "获取兑换码失败")
		return
	}

	switch fmtParam {
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="batch-%d.csv"`, id))
		w.Write([]byte("\xEF\xBB\xBF")) // UTF-8 BOM for Excel
		writer := csv.NewWriter(w)
		writer.Write([]string{"兑换码"})
		for _, c := range codes {
			writer.Write([]string{redeem.DisplayCode(c.Code, batch.Prefix)})
		}
		writer.Flush()
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="batch-%d.txt"`, id))
		for _, c := range codes {
			w.Write([]byte(redeem.DisplayCode(c.Code, batch.Prefix) + "\n"))
		}
	}
}

// ListLogs handles GET /api/admin/logs.
func (h *AdminHandler) ListLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	batchIDStr := r.URL.Query().Get("batch_id")
	result := r.URL.Query().Get("result")
	webhook := r.URL.Query().Get("webhook")
	userID := r.URL.Query().Get("user_id")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage := 50
	offset := int32((page - 1) * perPage)

	batchID := int64(0)
	if batchIDStr != "" {
		id, err := strconv.ParseInt(batchIDStr, 10, 64)
		if err == nil {
			batchID = id
		}
	}

	resultFilter := ""
	if result != "" {
		resultFilter = result
	}
	webhookFilter := ""
	if webhook != "" {
		webhookFilter = webhook
	}
	userFilter := ""
	if userID != "" {
		userFilter = userID
	}

	logs, err := h.queries.ListRedemptions(ctx, store.ListRedemptionsParams{
		BatchID:       batchID,
		ResultFilter:  resultFilter,
		WebhookFilter: webhookFilter,
		UserFilter:    userFilter,
		LimitCount:    int32(perPage),
		OffsetCount:   offset,
	})
	if err != nil {
		observability.Logger(ctx).Error("list logs failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "获取日志失败")
		return
	}

	total, err := h.queries.CountRedemptions(ctx, store.CountRedemptionsParams{
		BatchID:       batchID,
		ResultFilter:  resultFilter,
		WebhookFilter: webhookFilter,
		UserFilter:    userFilter,
	})
	if err != nil {
		observability.Logger(ctx).Error("count logs failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "获取日志失败")
		return
	}

	batches, err := h.queries.ListBatches(ctx)
	if err != nil {
		observability.Logger(ctx).Error("list batches for logs failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "获取日志失败")
		return
	}

	pages := int(total) / perPage
	if int(total)%perPage > 0 {
		pages++
	}

	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"logs":    redemptionLogResponses(logs),
		"batches": batchOptionResponses(batches),
		"total":   total,
		"page":    page,
		"pages":   pages,
	})
}

// ResendWebhook handles POST /api/admin/logs/:id/resend.
func (h *AdminHandler) ResendWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "无效记录 ID")
		return
	}

	redemption, err := h.queries.GetRedemptionByID(ctx, id)
	if err != nil {
		RespondError(w, http.StatusNotFound, "记录不存在")
		return
	}
	if redemption.Result != store.RedemptionResultSuccess {
		RespondError(w, http.StatusBadRequest, "只能重推成功的兑换记录")
		return
	}
	if redemption.BatchID == nil || redemption.WebhookStatus == store.WebhookStatusNone {
		RespondError(w, http.StatusBadRequest, "该记录没有 Webhook 配置")
		return
	}

	batch, err := h.queries.GetBatchByID(ctx, *redemption.BatchID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "获取批次失败")
		return
	}
	if batch.WebhookUrl == "" {
		RespondError(w, http.StatusBadRequest, "批次没有 Webhook 配置")
		return
	}

	eventID := generateHex(16)
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		observability.Logger(ctx).Error("begin webhook resend transaction failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "创建重推任务失败")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.queries.WithTx(tx)

	affected, err := qtx.PrepareRedemptionWebhookResend(ctx, store.PrepareRedemptionWebhookResendParams{
		ID:      id,
		EventID: &eventID,
	})
	if err != nil {
		observability.Logger(ctx).Error("prepare webhook resend failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "创建重推任务失败")
		return
	}
	if affected != 1 {
		RespondError(w, http.StatusConflict, "Webhook 正在投递中，请稍后重试")
		return
	}

	_, err = qtx.CreateWebhookOutboxEvent(ctx, store.CreateWebhookOutboxEventParams{
		EventID:      eventID,
		RedemptionID: id,
	})
	if err != nil {
		observability.Logger(ctx).Error("create resend outbox event failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "创建重推任务失败")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		observability.Logger(ctx).Error("commit webhook resend failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "创建重推任务失败")
		return
	}

	if h.worker != nil {
		h.worker.Wake()
	}

	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Webhook 重推任务已创建",
	})
}

// ListFAQ handles GET /api/admin/faq.
func (h *AdminHandler) ListFAQ(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := h.queries.ListFAQ(ctx)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "获取 FAQ 失败")
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items})
}

type faqRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// CreateFAQ handles POST /api/admin/faq.
func (h *AdminHandler) CreateFAQ(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req faqRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Content) == "" {
		RespondError(w, http.StatusBadRequest, "标题和内容不能为空")
		return
	}
	item, err := h.queries.CreateFAQ(ctx, store.CreateFAQParams{
		Title:   req.Title,
		Content: req.Content,
	})
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "创建 FAQ 失败")
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"ok": true, "id": item.ID})
}

// UpdateFAQ handles PUT /api/admin/faq/:id.
func (h *AdminHandler) UpdateFAQ(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "无效 FAQ ID")
		return
	}
	var req faqRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Content) == "" {
		RespondError(w, http.StatusBadRequest, "标题和内容不能为空")
		return
	}
	_, err = h.queries.UpdateFAQ(ctx, store.UpdateFAQParams{
		ID:      id,
		Title:   req.Title,
		Content: req.Content,
	})
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "更新 FAQ 失败")
		return
	}
	OK(w)
}

// DeleteFAQ handles DELETE /api/admin/faq/:id.
func (h *AdminHandler) DeleteFAQ(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "无效 FAQ ID")
		return
	}
	if err := h.queries.DeleteFAQ(ctx, id); err != nil {
		RespondError(w, http.StatusInternalServerError, "删除 FAQ 失败")
		return
	}
	OK(w)
}

type reorderFAQRequest struct {
	IDs []int64 `json:"ids"`
}

// ReorderFAQ handles POST /api/admin/faq/reorder.
func (h *AdminHandler) ReorderFAQ(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req reorderFAQRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if len(req.IDs) == 0 {
		RespondError(w, http.StatusBadRequest, "排序 ID 列表不能为空")
		return
	}
	if err := h.queries.ReorderFAQ(ctx, req.IDs); err != nil {
		RespondError(w, http.StatusInternalServerError, "排序 FAQ 失败")
		return
	}
	OK(w)
}

func parseBatchID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func statusText(status store.BatchStatus) string {
	if status == store.BatchStatusActive {
		return "启用"
	}
	return "停用"
}

func codeStatusText(status store.CodeStatus) string {
	if status == store.CodeStatusUnused {
		return "启用"
	}
	return "禁用"
}

func maskSecret(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 8 {
		return "****"
	}
	return secret[:4] + "****" + secret[len(secret)-4:]
}

func generateHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
