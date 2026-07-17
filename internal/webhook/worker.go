package webhook

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cdk-system/internal/store"
)

// Worker polls the webhook_outbox table and delivers events.
type Worker struct {
	pool      *pgxpool.Pool
	queries   workerStore
	deliverer *Deliverer
	config    Config
	wake      chan struct{}
	stop      chan struct{}
	wg        sync.WaitGroup
	started   bool
	mu        sync.Mutex
}

type workerStore interface {
	LeaseWebhookOutboxEvent(context.Context, *string) (store.WebhookOutbox, error)
	GetRedemptionByID(context.Context, int64) (store.Redemption, error)
	GetBatchByID(context.Context, int64) (store.Batch, error)
	UpdateRedemptionWebhookStatus(context.Context, store.UpdateRedemptionWebhookStatusParams) (int64, error)
	UpdateWebhookOutboxSuccess(context.Context, store.UpdateWebhookOutboxSuccessParams) (int64, error)
	UpdateWebhookOutboxFailed(context.Context, store.UpdateWebhookOutboxFailedParams) (int64, error)
}

// NewWorker creates a new webhook worker.
func NewWorker(pool *pgxpool.Pool, queries *store.Queries, deliverer *Deliverer, config Config) *Worker {
	return &Worker{
		pool:      pool,
		queries:   queries,
		deliverer: deliverer,
		config:    config,
		wake:      make(chan struct{}, 1),
		stop:      make(chan struct{}),
	}
}

// Run starts the worker loop.
func (w *Worker) Run() {
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	w.started = true
	w.mu.Unlock()

	w.wg.Add(1)
	go w.loop()
}

// Stop gracefully stops the worker.
func (w *Worker) Stop() {
	close(w.stop)
	w.wg.Wait()
}

// Wake signals the worker to poll immediately.
func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *Worker) loop() {
	defer w.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-w.stop:
			return
		case <-w.wake:
			w.processBatch()
		case <-ticker.C:
			w.processBatch()
		}
	}
}

func (w *Worker) processBatch() {
	ctx := context.Background()
	for {
		event, err := w.leaseEvent(ctx)
		if err != nil {
			slog.Error("lease webhook event failed", "error", err)
			return
		}
		if event == nil {
			return
		}
		w.deliverEvent(ctx, event)
	}
}

func (w *Worker) leaseEvent(ctx context.Context) (*store.WebhookOutbox, error) {
	token := generateLeaseToken()
	event, err := w.queries.LeaseWebhookOutboxEvent(ctx, &token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &event, nil
}

func (w *Worker) deliverEvent(ctx context.Context, event *store.WebhookOutbox) {
	redemption, err := w.queries.GetRedemptionByID(ctx, event.RedemptionID)
	if err != nil {
		slog.Error("get redemption for webhook failed", "error", err, "event_id", event.EventID)
		w.markFailed(ctx, event, "failed to load redemption", true)
		return
	}
	if redemption.BatchID == nil {
		w.markFailed(ctx, event, "redemption has no batch", false)
		return
	}

	batch, err := w.queries.GetBatchByID(ctx, *redemption.BatchID)
	if err != nil {
		slog.Error("get batch for webhook failed", "error", err, "event_id", event.EventID)
		w.markFailed(ctx, event, "failed to load batch", true)
		return
	}

	body := EventBody{
		Event:        "cdk.redeemed",
		RedemptionID: redemption.ID,
		Code:         redemption.CodeText,
		UserID:       redemption.UserID,
		Batch:        BatchInfo{ID: batch.ID, Name: batch.Name},
		Payload:      redemption.PayloadSnapshot,
		RedeemedAt:   redemption.CreatedAt.Format(time.RFC3339),
		EventID:      event.EventID,
	}

	deliverCtx, cancel := context.WithTimeout(ctx, w.config.RequestTimeout)
	defer cancel()

	ok, detail, err := w.deliverer.Deliver(deliverCtx, batch.WebhookUrl, batch.WebhookSecret, body)
	if err != nil {
		slog.Error("webhook deliver error", "error", err, "event_id", event.EventID)
		w.markFailed(ctx, event, detail, true)
		return
	}
	if ok {
		w.markSuccess(ctx, event, detail)
		return
	}
	w.markFailed(ctx, event, detail, true)
}

func (w *Worker) markSuccess(ctx context.Context, event *store.WebhookOutbox, detail string) {
	affected, err := w.queries.UpdateWebhookOutboxSuccess(ctx, store.UpdateWebhookOutboxSuccessParams{
		LastResponse: detail,
		ID:           event.ID,
		LeaseToken:   event.LeaseToken,
	})
	if err != nil {
		slog.Error("update outbox status failed", "error", err, "event_id", event.EventID)
		return
	}
	if affected != 1 {
		slog.Warn("webhook outbox lease lost before success update", "event_id", event.EventID, "rows_affected", affected)
		return
	}

	affected, err = w.queries.UpdateRedemptionWebhookStatus(ctx, store.UpdateRedemptionWebhookStatusParams{
		ID:              event.RedemptionID,
		WebhookStatus:   store.WebhookStatusSuccess,
		WebhookResponse: detail,
		EventID:         &event.EventID,
	})
	if err != nil {
		slog.Error("update redemption webhook status failed", "error", err, "event_id", event.EventID)
		return
	}
	if affected != 1 {
		slog.Warn("redemption webhook status was not advanced", "event_id", event.EventID, "rows_affected", affected)
	}
}

func (w *Worker) markFailed(ctx context.Context, event *store.WebhookOutbox, detail string, retry bool) {
	status := store.WebhookStatusFailed
	scheduledAt := time.Now().UTC()
	if retry && int(event.Attempts) < w.config.MaxAttempts {
		delay := time.Duration(float64(w.config.InitialDelay) * math.Pow(w.config.BackoffFactor, float64(event.Attempts-1)))
		if delay > w.config.MaxDelay {
			delay = w.config.MaxDelay
		}
		scheduledAt = time.Now().UTC().Add(delay)
	} else {
		status = store.WebhookStatusDeadLetter
	}

	affected, err := w.queries.UpdateWebhookOutboxFailed(ctx, store.UpdateWebhookOutboxFailedParams{
		Status:       status,
		LastResponse: detail,
		ScheduledAt:  scheduledAt,
		ID:           event.ID,
		LeaseToken:   event.LeaseToken,
	})
	if err != nil {
		slog.Error("update outbox status failed", "error", err, "event_id", event.EventID)
		return
	}
	if affected != 1 {
		slog.Warn("webhook outbox lease lost before failure update", "event_id", event.EventID, "rows_affected", affected)
		return
	}

	redemptionStatus := status
	if status == store.WebhookStatusFailed {
		redemptionStatus = store.WebhookStatusPending
	}
	affected, err = w.queries.UpdateRedemptionWebhookStatus(ctx, store.UpdateRedemptionWebhookStatusParams{
		ID:              event.RedemptionID,
		WebhookStatus:   redemptionStatus,
		WebhookResponse: detail,
		EventID:         &event.EventID,
	})
	if err != nil {
		slog.Error("update redemption webhook status failed", "error", err, "event_id", event.EventID)
		return
	}
	if affected != 1 {
		slog.Warn("redemption webhook status was not advanced", "event_id", event.EventID, "rows_affected", affected)
	}
}

func generateLeaseToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
