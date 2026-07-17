package batch

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"cdk-system/internal/store"
)

const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// CreateBatchRequest is the input for creating a batch with codes.
type CreateBatchRequest struct {
	Name               string
	Description        string
	PayloadJSON        string
	Prefix             string
	CodeLength         int32
	Count              int32
	MaxUsesPerCode     int32
	MaxRedeemsPerUser  int32
	ExpiresAt          *time.Time
	WebhookURL         string
	WebhookSecret      string
	AssignCredential   bool
	CredentialPlanType string
}

// CreateBatch creates a batch and generates codes in a single transaction.
func CreateBatch(ctx context.Context, pool store.DBTX, queries *store.Queries, req CreateBatchRequest) (int64, error) {
	if req.PayloadJSON == "" {
		req.PayloadJSON = "{}"
	}
	if req.Description == "" {
		req.Description = ""
	}
	prefix := strings.ToUpper(req.Prefix)

	batch, err := queries.CreateBatch(ctx, store.CreateBatchParams{
		Name:               req.Name,
		Description:        req.Description,
		PayloadJson:        json.RawMessage(req.PayloadJSON),
		Prefix:             prefix,
		CodeLength:         req.CodeLength,
		ExpiresAt:          pgTime(req.ExpiresAt),
		MaxUsesPerCode:     req.MaxUsesPerCode,
		MaxRedeemsPerUser:  req.MaxRedeemsPerUser,
		WebhookUrl:         req.WebhookURL,
		WebhookSecret:      req.WebhookSecret,
		AssignCredential:   req.AssignCredential,
		CredentialPlanType: strings.TrimSpace(req.CredentialPlanType),
		Status:             store.BatchStatusActive,
	})
	if err != nil {
		return 0, fmt.Errorf("create batch: %w", err)
	}

	codes := generateCodes(prefix, int(req.CodeLength), int(req.Count))
	params := make([]store.CreateCodesParams, len(codes))
	for i, code := range codes {
		params[i] = store.CreateCodesParams{BatchID: batch.ID, Code: code}
	}
	inserted, err := queries.CreateCodes(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("create codes: %w", err)
	}
	if inserted != int64(len(params)) {
		return 0, fmt.Errorf("create codes: inserted %d of %d", inserted, len(params))
	}

	return batch.ID, nil
}

func generateCodes(prefix string, length, count int) []string {
	codes := make([]string, 0, count)
	seen := make(map[string]struct{}, count)
	for len(codes) < count {
		code := prefix + randomCode(length)
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	return codes
}

func randomCode(length int) string {
	var sb strings.Builder
	max := big.NewInt(int64(len(alphabet)))
	for i := 0; i < length; i++ {
		n, _ := rand.Int(rand.Reader, max)
		sb.WriteByte(alphabet[n.Int64()])
	}
	return sb.String()
}

func pgTime(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
