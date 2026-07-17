package webhook

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Deliverer sends webhook events.
type Deliverer struct {
	client *http.Client
	config Config
}

// NewDeliverer creates a new webhook deliverer.
func NewDeliverer(config Config) *Deliverer {
	transport := SafeTransport(config.RequestTimeout)
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &Deliverer{
		client: &http.Client{
			Transport:     transport,
			Timeout:       config.RequestTimeout,
			CheckRedirect: NoRedirect,
		},
		config: config,
	}
}

// EventBody is the JSON payload sent to webhook targets.
type EventBody struct {
	Event        string          `json:"event"`
	RedemptionID int64           `json:"redemption_id"`
	Code         string          `json:"code"`
	UserID       string          `json:"user_id"`
	Batch        BatchInfo       `json:"batch"`
	Payload      json.RawMessage `json:"payload"`
	RedeemedAt   string          `json:"redeemed_at"`
	EventID      string          `json:"event_id"`
}

// BatchInfo describes the batch in a webhook payload.
type BatchInfo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Deliver sends a single webhook event.
func (d *Deliverer) Deliver(ctx context.Context, targetURL, secret string, body EventBody) (bool, string, error) {
	if err := ValidateURL(targetURL, d.config.AllowedDomains); err != nil {
		return false, fmt.Sprintf("url rejected: %v", err), nil
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return false, "", fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payload))
	if err != nil {
		return false, "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "cdk-system/1.0")
	if secret != "" {
		req.Header.Set("X-CDK-Signature", Sign(payload, secret))
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("request failed: %v", err), nil
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, d.config.MaxResponseBytes)
	respBody, _ := io.ReadAll(limited)
	detail := truncate(string(respBody), 500)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, detail, nil
	}
	return false, fmt.Sprintf("http %d: %s", resp.StatusCode, detail), nil
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
