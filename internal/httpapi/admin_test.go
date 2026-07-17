package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cdk-system/internal/batch"
)

type availableCredentialCounterStub struct {
	available int64
	err       error
	planType  string
	calls     int
}

func (s *availableCredentialCounterStub) CountAvailableCredentialsByPlan(_ context.Context, planType string) (int64, error) {
	s.calls++
	s.planType = planType
	return s.available, s.err
}

func TestValidateCreateBatchRejectsDatabaseConstraintViolations(t *testing.T) {
	req := createBatchRequest{
		Name:              strings.Repeat("名", 201),
		Description:       strings.Repeat("x", 1001),
		PayloadJSON:       "{}",
		Prefix:            "BAD-",
		CodeLength:        12,
		Count:             1,
		MaxUsesPerCode:    1,
		MaxRedeemsPerUser: 1,
		WebhookURL:        "http://127.0.0.1/hook",
		WebhookSecret:     strings.Repeat("s", 513),
	}

	errs := validateCreateBatch(req)
	if len(errs) < 5 {
		t.Fatalf("validation errors = %v, want all invalid fields rejected", errs)
	}
}

func TestASCIIAlphanumeric(t *testing.T) {
	if !isASCIIAlphanumeric("VIP2026") {
		t.Fatal("valid prefix was rejected")
	}
	for _, value := range []string{"VIP-", "中文", "ABC_"} {
		if isASCIIAlphanumeric(value) {
			t.Fatalf("invalid prefix %q was accepted", value)
		}
	}
}

func TestCreateBatchRejectsEmptyCredentialInventory(t *testing.T) {
	counter := &availableCredentialCounterStub{}
	createCalls := 0
	handler := &AdminHandler{
		credentialAvailability: counter,
		batchCreate: func(context.Context, batch.CreateBatchRequest) (int64, error) {
			createCalls++
			return 1, nil
		},
	}
	recorder := httptest.NewRecorder()

	handler.CreateBatch(recorder, validCreateBatchRequest(t, true, "pro"))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if counter.calls != 1 || counter.planType != "pro" {
		t.Fatalf("inventory query calls = %d, plan = %q", counter.calls, counter.planType)
	}
	if createCalls != 0 {
		t.Fatalf("batch create calls = %d, want 0", createCalls)
	}
}

func TestCreateBatchAllowsAvailableCredentialInventory(t *testing.T) {
	counter := &availableCredentialCounterStub{available: 1}
	createCalls := 0
	var created batch.CreateBatchRequest
	handler := &AdminHandler{
		credentialAvailability: counter,
		batchCreate: func(_ context.Context, req batch.CreateBatchRequest) (int64, error) {
			createCalls++
			created = req
			return 42, nil
		},
	}
	recorder := httptest.NewRecorder()

	handler.CreateBatch(recorder, validCreateBatchRequest(t, true, " pro "))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if counter.calls != 1 || counter.planType != "pro" || createCalls != 1 {
		t.Fatalf("inventory calls = %d, plan = %q, create calls = %d", counter.calls, counter.planType, createCalls)
	}
	if !created.AssignCredential || created.CredentialPlanType != "pro" {
		t.Fatalf("created request = %+v", created)
	}
}

func TestCreateBatchWithoutCredentialAssignmentSkipsInventoryCheck(t *testing.T) {
	counter := &availableCredentialCounterStub{}
	createCalls := 0
	handler := &AdminHandler{
		credentialAvailability: counter,
		batchCreate: func(context.Context, batch.CreateBatchRequest) (int64, error) {
			createCalls++
			return 7, nil
		},
	}
	recorder := httptest.NewRecorder()

	handler.CreateBatch(recorder, validCreateBatchRequest(t, false, ""))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if counter.calls != 0 || createCalls != 1 {
		t.Fatalf("inventory calls = %d, create calls = %d", counter.calls, createCalls)
	}
}

func validCreateBatchRequest(t *testing.T, assignCredential bool, planType string) *http.Request {
	t.Helper()
	body, err := json.Marshal(createBatchRequest{
		Name:               "test batch",
		CodeLength:         12,
		Count:              1,
		MaxUsesPerCode:     1,
		MaxRedeemsPerUser:  1,
		AssignCredential:   assignCredential,
		CredentialPlanType: planType,
	})
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewRequest(http.MethodPost, "/api/admin/batches", strings.NewReader(string(body)))
}
