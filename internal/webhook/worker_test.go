package webhook

import (
	"context"
	"testing"
	"time"

	"cdk-system/internal/store"
)

type fakeWorkerStore struct {
	successRows     int64
	failureRows     int64
	redemptionRows  int64
	successCalls    int
	failureCalls    int
	redemptionCalls int
	successArg      store.UpdateWebhookOutboxSuccessParams
	failureArg      store.UpdateWebhookOutboxFailedParams
	redemptionArg   store.UpdateRedemptionWebhookStatusParams
}

func (f *fakeWorkerStore) LeaseWebhookOutboxEvent(context.Context, *string) (store.WebhookOutbox, error) {
	return store.WebhookOutbox{}, nil
}

func (f *fakeWorkerStore) GetRedemptionByID(context.Context, int64) (store.Redemption, error) {
	return store.Redemption{}, nil
}

func (f *fakeWorkerStore) GetBatchByID(context.Context, int64) (store.Batch, error) {
	return store.Batch{}, nil
}

func (f *fakeWorkerStore) UpdateRedemptionWebhookStatus(_ context.Context, arg store.UpdateRedemptionWebhookStatusParams) (int64, error) {
	f.redemptionCalls++
	f.redemptionArg = arg
	return f.redemptionRows, nil
}

func (f *fakeWorkerStore) UpdateWebhookOutboxSuccess(_ context.Context, arg store.UpdateWebhookOutboxSuccessParams) (int64, error) {
	f.successCalls++
	f.successArg = arg
	return f.successRows, nil
}

func (f *fakeWorkerStore) UpdateWebhookOutboxFailed(_ context.Context, arg store.UpdateWebhookOutboxFailedParams) (int64, error) {
	f.failureCalls++
	f.failureArg = arg
	return f.failureRows, nil
}

func TestMarkSuccessStopsAfterLeaseLoss(t *testing.T) {
	token := "current-token"
	fake := &fakeWorkerStore{successRows: 0, redemptionRows: 1}
	worker := &Worker{queries: fake}
	event := &store.WebhookOutbox{ID: 11, RedemptionID: 22, EventID: "event-1", LeaseToken: &token, Attempts: 1}

	worker.markSuccess(context.Background(), event, "ok")

	if fake.successCalls != 1 {
		t.Fatalf("success update calls = %d, want 1", fake.successCalls)
	}
	if fake.redemptionCalls != 0 {
		t.Fatalf("redemption update calls = %d, want 0 after lease loss", fake.redemptionCalls)
	}
	if fake.successArg.LeaseToken == nil || *fake.successArg.LeaseToken != token {
		t.Fatalf("success update did not use the event lease token")
	}
}

func TestMarkSuccessUpdatesRedemptionAfterOwnedCompletion(t *testing.T) {
	token := "owned-token"
	fake := &fakeWorkerStore{successRows: 1, redemptionRows: 1}
	worker := &Worker{queries: fake}
	event := &store.WebhookOutbox{ID: 11, RedemptionID: 22, EventID: "event-2", LeaseToken: &token, Attempts: 2}

	worker.markSuccess(context.Background(), event, "delivered")

	if fake.redemptionCalls != 1 {
		t.Fatalf("redemption update calls = %d, want 1", fake.redemptionCalls)
	}
}

func TestMarkFailedUsesLeaseTokenAndSchedulesRetry(t *testing.T) {
	token := "failed-token"
	fake := &fakeWorkerStore{failureRows: 1, redemptionRows: 1}
	worker := &Worker{
		queries: fake,
		config: Config{
			MaxAttempts:   3,
			InitialDelay:  time.Second,
			BackoffFactor: 2,
			MaxDelay:      time.Minute,
		},
	}
	event := &store.WebhookOutbox{ID: 31, RedemptionID: 41, EventID: "event-3", LeaseToken: &token, Attempts: 1}

	before := time.Now().UTC()
	worker.markFailed(context.Background(), event, "temporary failure", true)

	if fake.failureCalls != 1 || fake.redemptionCalls != 1 {
		t.Fatalf("failure calls = %d, redemption calls = %d; want 1 each", fake.failureCalls, fake.redemptionCalls)
	}
	if fake.failureArg.LeaseToken == nil || *fake.failureArg.LeaseToken != token {
		t.Fatalf("failure update did not use the event lease token")
	}
	if fake.failureArg.Status != store.WebhookStatusFailed {
		t.Fatalf("failure status = %q, want %q", fake.failureArg.Status, store.WebhookStatusFailed)
	}
	if !fake.failureArg.ScheduledAt.After(before) {
		t.Fatalf("retry was not scheduled in the future")
	}
	if fake.redemptionArg.WebhookStatus != store.WebhookStatusPending {
		t.Fatalf("redemption status = %q, want pending during retry", fake.redemptionArg.WebhookStatus)
	}
	if fake.redemptionArg.EventID == nil || *fake.redemptionArg.EventID != event.EventID {
		t.Fatal("redemption update did not use the leased event ID")
	}
}
