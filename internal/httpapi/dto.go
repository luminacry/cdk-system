package httpapi

import (
	"encoding/json"
	"time"

	"cdk-system/internal/redeem"
	"cdk-system/internal/store"
)

type batchResponse struct {
	ID                 int64           `json:"id"`
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	PayloadJSON        json.RawMessage `json:"payload_json"`
	Prefix             string          `json:"prefix"`
	CodeLength         int32           `json:"code_length"`
	ExpiresAt          *string         `json:"expires_at"`
	MaxUsesPerCode     int32           `json:"max_uses_per_code"`
	MaxRedeemsPerUser  int32           `json:"max_redeems_per_user"`
	WebhookURL         string          `json:"webhook_url"`
	WebhookSecret      string          `json:"webhook_secret"`
	Status             string          `json:"status"`
	CreatedAt          string          `json:"created_at"`
	UpdatedAt          string          `json:"updated_at"`
	AssignCredential   bool            `json:"assign_credential"`
	CredentialPlanType string          `json:"credential_plan_type"`
	CodeCount          int64           `json:"code_count"`
	UsedCount          int64           `json:"used_count"`
}

func batchModelResponse(batch store.Batch) batchResponse {
	return batchResponse{
		ID:                 batch.ID,
		Name:               batch.Name,
		Description:        batch.Description,
		PayloadJSON:        batch.PayloadJson,
		Prefix:             batch.Prefix,
		CodeLength:         batch.CodeLength,
		ExpiresAt:          nullableTime(batch.ExpiresAt.Valid, batch.ExpiresAt.Time),
		MaxUsesPerCode:     batch.MaxUsesPerCode,
		MaxRedeemsPerUser:  batch.MaxRedeemsPerUser,
		WebhookURL:         batch.WebhookUrl,
		WebhookSecret:      maskSecret(batch.WebhookSecret),
		Status:             string(batch.Status),
		CreatedAt:          formatTime(batch.CreatedAt),
		UpdatedAt:          formatTime(batch.UpdatedAt),
		AssignCredential:   batch.AssignCredential,
		CredentialPlanType: batch.CredentialPlanType,
	}
}

func batchListResponses(rows []store.ListBatchesRow) []batchResponse {
	items := make([]batchResponse, 0, len(rows))
	for _, batch := range rows {
		items = append(items, batchResponse{
			ID:                 batch.ID,
			Name:               batch.Name,
			Description:        batch.Description,
			PayloadJSON:        batch.PayloadJson,
			Prefix:             batch.Prefix,
			CodeLength:         batch.CodeLength,
			ExpiresAt:          nullableTime(batch.ExpiresAt.Valid, batch.ExpiresAt.Time),
			MaxUsesPerCode:     batch.MaxUsesPerCode,
			MaxRedeemsPerUser:  batch.MaxRedeemsPerUser,
			WebhookURL:         batch.WebhookUrl,
			WebhookSecret:      maskSecret(batch.WebhookSecret),
			Status:             string(batch.Status),
			CreatedAt:          formatTime(batch.CreatedAt),
			UpdatedAt:          formatTime(batch.UpdatedAt),
			AssignCredential:   batch.AssignCredential,
			CredentialPlanType: batch.CredentialPlanType,
			CodeCount:          batch.CodeCount,
			UsedCount:          batch.UsedCount,
		})
	}
	return items
}

type codeResponse struct {
	ID        int64  `json:"id"`
	Code      string `json:"code"`
	Display   string `json:"display"`
	Status    string `json:"status"`
	UseCount  int32  `json:"use_count"`
	State     string `json:"state"`
	CreatedAt string `json:"created_at"`
}

func codeResponses(rows []store.ListCodesByBatchRow, prefix string) []codeResponse {
	items := make([]codeResponse, 0, len(rows))
	for _, code := range rows {
		items = append(items, codeResponse{
			ID:        code.ID,
			Code:      code.Code,
			Display:   redeem.DisplayCode(code.Code, prefix),
			Status:    string(code.Status),
			UseCount:  code.UseCount,
			State:     code.State,
			CreatedAt: formatTime(code.CreatedAt),
		})
	}
	return items
}

type dailyResponse struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

func dailyResponses(rows []store.GetDailyRedemptionsRow) []dailyResponse {
	items := make([]dailyResponse, 0, len(rows))
	for _, row := range rows {
		if !row.Date.Valid {
			continue
		}
		items = append(items, dailyResponse{Date: row.Date.Time.Format(time.DateOnly), Count: row.Count})
	}
	return items
}

type recentRedemptionResponse struct {
	ID            int64   `json:"id"`
	CreatedAt     string  `json:"created_at"`
	BatchName     *string `json:"batch_name"`
	Code          string  `json:"code"`
	UserID        string  `json:"user_id"`
	Result        string  `json:"result"`
	Message       string  `json:"message"`
	WebhookStatus string  `json:"webhook_status"`
}

func recentRedemptionResponses(rows []store.GetRecentRedemptionsRow) []recentRedemptionResponse {
	items := make([]recentRedemptionResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, recentRedemptionResponse{
			ID:            row.ID,
			CreatedAt:     formatTime(row.CreatedAt),
			BatchName:     row.BatchName,
			Code:          row.Code,
			UserID:        row.UserID,
			Result:        string(row.Result),
			Message:       row.Message,
			WebhookStatus: string(row.WebhookStatus),
		})
	}
	return items
}

type redemptionLogResponse struct {
	ID              int64   `json:"id"`
	CreatedAt       string  `json:"created_at"`
	BatchName       *string `json:"batch_name"`
	Code            string  `json:"code"`
	UserID          string  `json:"user_id"`
	Result          string  `json:"result"`
	Message         string  `json:"message"`
	WebhookStatus   string  `json:"webhook_status"`
	WebhookResponse string  `json:"webhook_response"`
	IP              string  `json:"ip"`
}

func redemptionLogResponses(rows []store.ListRedemptionsRow) []redemptionLogResponse {
	items := make([]redemptionLogResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, redemptionLogResponse{
			ID:              row.ID,
			CreatedAt:       formatTime(row.CreatedAt),
			BatchName:       row.BatchName,
			Code:            row.Code,
			UserID:          row.UserID,
			Result:          string(row.Result),
			Message:         row.Message,
			WebhookStatus:   string(row.WebhookStatus),
			WebhookResponse: row.WebhookResponse,
			IP:              row.Ip,
		})
	}
	return items
}

type batchOptionResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func batchOptionResponses(rows []store.ListBatchesRow) []batchOptionResponse {
	items := make([]batchOptionResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, batchOptionResponse{ID: row.ID, Name: row.Name})
	}
	return items
}

func nullableTime(valid bool, value time.Time) *string {
	if !valid {
		return nil
	}
	formatted := formatTime(value)
	return &formatted
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}
