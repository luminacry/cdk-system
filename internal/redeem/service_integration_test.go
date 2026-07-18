package redeem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cdk-system/internal/store"
)

type integrationFixture struct {
	pool      *pgxpool.Pool
	service   *Service
	batchID   int64
	code      string
	plan      string
	idemKeys  []string
	createdAt string
}

func newIntegrationFixture(t *testing.T, assignCredential bool, maxUses, maxPerUser int32) *integrationFixture {
	t.Helper()
	databaseURL := os.Getenv("CDK_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CDK_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping test database: %v", err)
	}

	token := fmt.Sprintf("%d", time.Now().UnixNano())
	fixture := &integrationFixture{
		pool:      pool,
		service:   NewService(pool, store.NewQueries(pool)),
		code:      "IT" + token,
		plan:      "integration-" + token,
		createdAt: token,
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO batches (
			name, description, payload_json, prefix, code_length, expires_at,
			max_uses_per_code, max_redeems_per_user, webhook_url, webhook_secret,
			assign_credential, credential_plan_type, status
		) VALUES ($1, '', '{"source":"integration"}', 'IT', 12, NULL, $2, $3, '', '', $4, $5, 'active')
		RETURNING id`, "integration-"+token, maxUses, maxPerUser, assignCredential, fixture.plan).Scan(&fixture.batchID)
	if err != nil {
		pool.Close()
		t.Fatalf("create integration batch: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO codes (batch_id, code) VALUES ($1, $2)`, fixture.batchID, fixture.code); err != nil {
		pool.Close()
		t.Fatalf("create integration code: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM redemptions WHERE batch_id = $1`, fixture.batchID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM credentials WHERE plan_type IN ($1, $2)`, fixture.plan, fixture.plan+"-other")
		for _, key := range fixture.idemKeys {
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM idempotency_keys WHERE key = $1`, key)
		}
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM batches WHERE id = $1`, fixture.batchID)
		pool.Close()
	})
	return fixture
}

func (f *integrationFixture) idempotencyKey(suffix string) string {
	key := "integration-" + f.createdAt + "-" + suffix
	f.idemKeys = append(f.idemKeys, key)
	return key
}

func (f *integrationFixture) addCode(t *testing.T, suffix string) string {
	t.Helper()
	code := f.code + suffix
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO codes (batch_id, code) VALUES ($1, $2)`, f.batchID, code); err != nil {
		t.Fatalf("insert integration code: %v", err)
	}
	return code
}

func TestRedeemCredentialInventoryIntegration(t *testing.T) {
	fixture := newIntegrationFixture(t, true, 1, 1)
	ctx := context.Background()

	first, err := fixture.service.Redeem(ctx, RedeemRequest{
		UserID:         "inventory-user@example.com",
		Code:           fixture.code,
		IdempotencyKey: fixture.idempotencyKey("empty"),
	})
	if err != nil {
		t.Fatalf("redeem without inventory: %v", err)
	}
	if first.Result != ResultCredentialUnavailable {
		t.Fatalf("result = %q, want %q", first.Result, ResultCredentialUnavailable)
	}
	assertCodeUsage(t, fixture, 0)

	expiredAt := time.Now().UTC().Add(-time.Hour)
	if _, err := fixture.pool.Exec(ctx, `
		INSERT INTO credentials (email, plan_type, expired, data)
		VALUES ('expired@example.com', $1, $2, '{"marker":"expired"}')`, fixture.plan, expiredAt); err != nil {
		t.Fatalf("insert expired credential: %v", err)
	}
	if _, err := fixture.pool.Exec(ctx, `
		INSERT INTO credentials (email, plan_type, expired, data)
		VALUES ('other@example.com', $1, NULL, '{"marker":"other"}')`, fixture.plan+"-other"); err != nil {
		t.Fatalf("insert other-plan credential: %v", err)
	}

	second, err := fixture.service.Redeem(ctx, RedeemRequest{
		UserID:         "inventory-user@example.com",
		Code:           fixture.code,
		IdempotencyKey: fixture.idempotencyKey("unavailable"),
	})
	if err != nil {
		t.Fatalf("redeem with unusable inventory: %v", err)
	}
	if second.Result != ResultCredentialUnavailable {
		t.Fatalf("result = %q, want %q", second.Result, ResultCredentialUnavailable)
	}
	assertCodeUsage(t, fixture, 0)

	var credentialID int64
	if err := fixture.pool.QueryRow(ctx, `
		INSERT INTO credentials (email, plan_type, expired, data)
		VALUES ('valid@example.com', $1, NULL, '{"marker":"valid"}') RETURNING id`, fixture.plan).Scan(&credentialID); err != nil {
		t.Fatalf("insert valid credential: %v", err)
	}

	success, err := fixture.service.Redeem(ctx, RedeemRequest{
		UserID:         "inventory-user@example.com",
		Code:           fixture.code,
		IdempotencyKey: fixture.idempotencyKey("success"),
	})
	if err != nil {
		t.Fatalf("redeem with valid inventory: %v", err)
	}
	if !success.OK || success.Result != ResultSuccess {
		t.Fatalf("unexpected success response: %+v", success)
	}
	var credential map[string]any
	if err := json.Unmarshal(success.Credential, &credential); err != nil {
		t.Fatalf("decode assigned credential: %v", err)
	}
	if credential["marker"] != "valid" {
		t.Fatalf("assigned credential marker = %v, want valid", credential["marker"])
	}
	assertCodeUsage(t, fixture, 1)

	var used bool
	if err := fixture.pool.QueryRow(ctx, `SELECT used FROM credentials WHERE id = $1`, credentialID).Scan(&used); err != nil {
		t.Fatalf("read assigned credential: %v", err)
	}
	if !used {
		t.Fatal("assigned credential was not marked used")
	}
}

func TestRedeemConcurrentIdempotencyIntegration(t *testing.T) {
	fixture := newIntegrationFixture(t, false, 10, 10)
	ctx := context.Background()
	key := fixture.idempotencyKey("concurrent")
	req := RedeemRequest{UserID: "concurrent-user@example.com", Code: fixture.code, IdempotencyKey: key}

	const workers = 16
	start := make(chan struct{})
	var wg sync.WaitGroup
	var failures atomic.Int32
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resp, err := fixture.service.Redeem(ctx, req)
			if err != nil || resp == nil || !resp.OK {
				failures.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if failures.Load() != 0 {
		t.Fatalf("%d concurrent idempotent requests failed", failures.Load())
	}
	assertCodeUsage(t, fixture, 1)

	var successCount int
	if err := fixture.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM redemptions WHERE batch_id = $1 AND result = 'success'`, fixture.batchID).Scan(&successCount); err != nil {
		t.Fatalf("count successful redemptions: %v", err)
	}
	if successCount != 1 {
		t.Fatalf("successful redemption rows = %d, want 1", successCount)
	}

	_, err := fixture.service.Redeem(ctx, RedeemRequest{
		UserID:         "different-user@example.com",
		Code:           fixture.code,
		IdempotencyKey: key,
	})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("different request error = %v, want ErrIdempotencyConflict", err)
	}
	assertCodeUsage(t, fixture, 1)
}

func TestBatchRedeemPartialResultsIntegration(t *testing.T) {
	fixture := newIntegrationFixture(t, false, 1, 2)
	ctx := context.Background()
	secondCode := fixture.addCode(t, "B")
	thirdCode := fixture.addCode(t, "C")

	resp, err := fixture.service.BatchRedeem(ctx, BatchRedeemRequest{
		UserID: "batch-user@example.com",
		Codes:  []string{fixture.code, "missing-code", secondCode, thirdCode, secondCode},
	})
	if err != nil {
		t.Fatalf("batch redeem: %v", err)
	}
	if !resp.OK || resp.Total != 5 || resp.Succeeded != 3 || resp.Failed != 2 {
		t.Fatalf("unexpected batch response: %+v", resp)
	}
	wantResults := []string{ResultSuccess, ResultInvalidCode, ResultSuccess, ResultSuccess, ResultInvalidInput}
	for i, want := range wantResults {
		if resp.Results[i].Result != want {
			t.Fatalf("result[%d] = %q, want %q", i, resp.Results[i].Result, want)
		}
	}
	if resp.Results[0].Redemption == nil || resp.Results[1].Redemption != nil {
		t.Fatalf("success/failure redemption details are incorrect: %+v", resp.Results)
	}
	var thirdUses int
	if err := fixture.pool.QueryRow(ctx, `SELECT use_count FROM codes WHERE code = $1`, thirdCode).Scan(&thirdUses); err != nil {
		t.Fatal(err)
	}
	if thirdUses != 1 {
		t.Fatalf("third code use_count = %d, want 1", thirdUses)
	}
}

func TestBatchRedeemIdempotencyIntegration(t *testing.T) {
	fixture := newIntegrationFixture(t, false, 2, 2)
	ctx := context.Background()
	secondCode := fixture.addCode(t, "B")
	key := fixture.idempotencyKey("batch")
	req := BatchRedeemRequest{
		UserID:         "batch-idempotent@example.com",
		Codes:          []string{fixture.code, secondCode},
		IdempotencyKey: key,
	}

	first, err := fixture.service.BatchRedeem(ctx, req)
	if err != nil || first.Succeeded != 2 {
		t.Fatalf("first batch redeem = %+v, %v", first, err)
	}
	second, err := fixture.service.BatchRedeem(ctx, req)
	if err != nil || second.Succeeded != 2 {
		t.Fatalf("replayed batch redeem = %+v, %v", second, err)
	}
	var totalUses int
	if err := fixture.pool.QueryRow(ctx, `SELECT SUM(use_count) FROM codes WHERE batch_id = $1`, fixture.batchID).Scan(&totalUses); err != nil {
		t.Fatal(err)
	}
	if totalUses != 2 {
		t.Fatalf("total uses after replay = %d, want 2", totalUses)
	}

	req.Codes = []string{fixture.code}
	if _, err := fixture.service.BatchRedeem(ctx, req); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed batch request error = %v, want ErrIdempotencyConflict", err)
	}
}

func TestBatchRedeemConcurrentIdempotencyIntegration(t *testing.T) {
	fixture := newIntegrationFixture(t, false, 2, 2)
	ctx := context.Background()
	secondCode := fixture.addCode(t, "B")
	req := BatchRedeemRequest{
		UserID:         "batch-concurrent@example.com",
		Codes:          []string{fixture.code, secondCode},
		IdempotencyKey: fixture.idempotencyKey("batch-concurrent"),
	}

	const workers = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	var failures atomic.Int32
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resp, err := fixture.service.BatchRedeem(ctx, req)
			if err != nil || resp == nil || resp.Succeeded != 2 || resp.Failed != 0 {
				failures.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if failures.Load() != 0 {
		t.Fatalf("%d concurrent idempotent batch requests failed", failures.Load())
	}
	var totalUses int
	if err := fixture.pool.QueryRow(ctx, `SELECT SUM(use_count) FROM codes WHERE batch_id = $1`, fixture.batchID).Scan(&totalUses); err != nil {
		t.Fatal(err)
	}
	if totalUses != 2 {
		t.Fatalf("total uses after concurrent replay = %d, want 2", totalUses)
	}
}

func TestRedeemUsedCodeDoesNotIncrementCodeIntegration(t *testing.T) {
	fixture := newIntegrationFixture(t, false, 1, 1)
	ctx := context.Background()
	if _, err := fixture.pool.Exec(ctx, `UPDATE codes SET use_count = 1 WHERE batch_id = $1`, fixture.batchID); err != nil {
		t.Fatalf("mark code used: %v", err)
	}

	resp, err := fixture.service.Redeem(ctx, RedeemRequest{UserID: "quota-user@example.com", Code: fixture.code})
	if err != nil {
		t.Fatalf("redeem used code: %v", err)
	}
	if resp.Result != ResultCodeUsedUp {
		t.Fatalf("result = %q, want %q", resp.Result, ResultCodeUsedUp)
	}
	assertCodeUsage(t, fixture, 1)
}

func TestWebhookStatusUsesCurrentEventIntegration(t *testing.T) {
	fixture := newIntegrationFixture(t, false, 1, 1)
	ctx := context.Background()
	if _, err := fixture.pool.Exec(ctx, `
		UPDATE batches SET webhook_url = 'https://example.com/hook', webhook_secret = 'integration-secret'
		WHERE id = $1`, fixture.batchID); err != nil {
		t.Fatalf("configure webhook: %v", err)
	}

	resp, err := fixture.service.Redeem(ctx, RedeemRequest{UserID: "webhook-user@example.com", Code: fixture.code})
	if err != nil || resp == nil || !resp.OK || !resp.WebhookPending {
		t.Fatalf("webhook redeem = %+v, %v", resp, err)
	}
	var redemptionID int64
	var eventID string
	var attempts int32
	if err := fixture.pool.QueryRow(ctx, `
		SELECT id, event_id, webhook_attempt FROM redemptions
		WHERE batch_id = $1 AND result = 'success'`, fixture.batchID).Scan(&redemptionID, &eventID, &attempts); err != nil {
		t.Fatalf("read webhook redemption: %v", err)
	}
	if attempts != 0 {
		t.Fatalf("initial webhook attempts = %d, want 0", attempts)
	}

	queries := store.NewQueries(fixture.pool)
	wrongEventID := "wrong-event"
	rows, err := queries.UpdateRedemptionWebhookStatus(ctx, store.UpdateRedemptionWebhookStatusParams{
		ID:              redemptionID,
		WebhookStatus:   store.WebhookStatusSuccess,
		WebhookResponse: "wrong",
		EventID:         &wrongEventID,
	})
	if err != nil || rows != 0 {
		t.Fatalf("stale event update = %d, %v; want 0, nil", rows, err)
	}
	rows, err = queries.UpdateRedemptionWebhookStatus(ctx, store.UpdateRedemptionWebhookStatusParams{
		ID:              redemptionID,
		WebhookStatus:   store.WebhookStatusSuccess,
		WebhookResponse: "delivered",
		EventID:         &eventID,
	})
	if err != nil || rows != 1 {
		t.Fatalf("owned event update = %d, %v; want 1, nil", rows, err)
	}

	newEventID := "resend-" + fixture.createdAt
	rows, err = queries.PrepareRedemptionWebhookResend(ctx, store.PrepareRedemptionWebhookResendParams{
		ID:      redemptionID,
		EventID: &newEventID,
	})
	if err != nil || rows != 1 {
		t.Fatalf("prepare resend = %d, %v; want 1, nil", rows, err)
	}
	rows, err = queries.PrepareRedemptionWebhookResend(ctx, store.PrepareRedemptionWebhookResendParams{
		ID:      redemptionID,
		EventID: &wrongEventID,
	})
	if err != nil || rows != 0 {
		t.Fatalf("duplicate resend = %d, %v; want 0, nil", rows, err)
	}
	rows, err = queries.UpdateRedemptionWebhookStatus(ctx, store.UpdateRedemptionWebhookStatusParams{
		ID:              redemptionID,
		WebhookStatus:   store.WebhookStatusFailed,
		WebhookResponse: "stale",
		EventID:         &eventID,
	})
	if err != nil || rows != 0 {
		t.Fatalf("old event after resend = %d, %v; want 0, nil", rows, err)
	}
}

func assertCodeUsage(t *testing.T, fixture *integrationFixture, wantCode int) {
	t.Helper()
	ctx := context.Background()
	var codeUses int
	if err := fixture.pool.QueryRow(ctx, `SELECT use_count FROM codes WHERE batch_id = $1`, fixture.batchID).Scan(&codeUses); err != nil {
		t.Fatalf("read code usage: %v", err)
	}
	if codeUses != wantCode {
		t.Fatalf("code use_count = %d, want %d", codeUses, wantCode)
	}
}
