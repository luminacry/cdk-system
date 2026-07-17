package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"cdk-system/internal/store"
)

type credentialLookupStub struct {
	credential  store.Credential
	err         error
	requestedID int64
}

type credentialListStub struct {
	items      []store.ListCredentialsRow
	total      int64
	listErr    error
	countErr   error
	params     store.ListCredentialsParams
	countState string
	listCalls  int
	countCalls int
}

type credentialPlansStub struct {
	plans []store.ListCredentialPlansRow
	err   error
	calls int
}

func (s *credentialPlansStub) ListCredentialPlans(context.Context) ([]store.ListCredentialPlansRow, error) {
	s.calls++
	return s.plans, s.err
}

func (s *credentialListStub) ListCredentials(_ context.Context, params store.ListCredentialsParams) ([]store.ListCredentialsRow, error) {
	s.params = params
	s.listCalls++
	return s.items, s.listErr
}

func (s *credentialListStub) CountCredentials(_ context.Context, state string) (int64, error) {
	s.countState = state
	s.countCalls++
	return s.total, s.countErr
}

func (s *credentialLookupStub) GetCredentialByID(_ context.Context, id int64) (store.Credential, error) {
	s.requestedID = id
	return s.credential, s.err
}

func TestListCredentialsUsesRequestedPageSizeAndOffset(t *testing.T) {
	list := &credentialListStub{total: 101}
	handler := &CredentialsHandler{credentialList: list}
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/credentials?state=unused&page=3&page_size=20", nil)

	handler.ListCredentials(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if list.params.StateFilter != "unused" || list.params.PageSize != 20 || list.params.PageOffset != 40 {
		t.Fatalf("query params = %+v, want state unused, size 20, offset 40", list.params)
	}
	if list.countState != "unused" || list.listCalls != 1 || list.countCalls != 1 {
		t.Fatalf("list calls = %d, count calls = %d, count state = %q", list.listCalls, list.countCalls, list.countState)
	}

	var response struct {
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		Pages    int   `json:"pages"`
		PageSize int32 `json:"page_size"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Total != 101 || response.Page != 3 || response.Pages != 6 || response.PageSize != 20 {
		t.Fatalf("pagination response = %+v", response)
	}
}

func TestListCredentialsDefaultsToFiftyPerPage(t *testing.T) {
	list := &credentialListStub{total: 125}
	handler := &CredentialsHandler{credentialList: list}
	recorder := httptest.NewRecorder()

	handler.ListCredentials(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/credentials", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if list.params.PageSize != 50 || list.params.PageOffset != 0 {
		t.Fatalf("query params = %+v, want size 50, offset 0", list.params)
	}
	var response struct {
		Pages    int   `json:"pages"`
		PageSize int32 `json:"page_size"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Pages != 3 || response.PageSize != 50 {
		t.Fatalf("pagination response = %+v", response)
	}
}

func TestCredentialPageSizeAllowlist(t *testing.T) {
	for _, pageSize := range []int32{20, 50, 100, 200, 500} {
		t.Run(strconv.Itoa(int(pageSize)), func(t *testing.T) {
			page, gotSize, offset, err := parseCredentialPagination("2", strconv.Itoa(int(pageSize)))
			if err != nil {
				t.Fatal(err)
			}
			if page != 2 || gotSize != pageSize || offset != pageSize {
				t.Fatalf("pagination = %d, %d, %d", page, gotSize, offset)
			}
		})
	}
}

func TestListCredentialsRejectsInvalidPageSize(t *testing.T) {
	for _, pageSize := range []string{"0", "10", "501", "-20", "abc", "20.0"} {
		t.Run(pageSize, func(t *testing.T) {
			list := &credentialListStub{}
			handler := &CredentialsHandler{credentialList: list}
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/admin/credentials?page_size="+pageSize, nil)

			handler.ListCredentials(recorder, req)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if list.listCalls != 0 || list.countCalls != 0 {
				t.Fatalf("database called for invalid page size")
			}
		})
	}
}

func TestListCredentialsRejectsOffsetOverflow(t *testing.T) {
	list := &credentialListStub{}
	handler := &CredentialsHandler{credentialList: list}
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/credentials?page=2147483648&page_size=500", nil)

	handler.ListCredentials(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if list.listCalls != 0 || list.countCalls != 0 {
		t.Fatalf("database called for overflowing offset")
	}
}

func TestListCredentialPlansReturnsCompleteAggregates(t *testing.T) {
	plans := &credentialPlansStub{plans: []store.ListCredentialPlansRow{{
		PlanType:  "pro",
		Total:     10,
		Available: 6,
		Used:      3,
		Expired:   1,
	}}}
	handler := &CredentialsHandler{credentialPlans: plans}
	recorder := httptest.NewRecorder()

	handler.ListCredentialPlans(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/credentials/plans", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response struct {
		OK    bool                           `json:"ok"`
		Plans []store.ListCredentialPlansRow `json:"plans"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || len(response.Plans) != 1 || response.Plans[0] != plans.plans[0] {
		t.Fatalf("response = %+v", response)
	}
}

func TestListCredentialPlansQueryFailure(t *testing.T) {
	plans := &credentialPlansStub{err: errors.New("database unavailable")}
	handler := &CredentialsHandler{credentialPlans: plans}
	recorder := httptest.NewRecorder()

	handler.ListCredentialPlans(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/credentials/plans", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
}

func TestUploadCredentialsContentTypeDoesNotPanic(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
	}{
		{name: "short valid content type", contentType: "application/json"},
		{name: "malformed content type", contentType: "application/json; charset"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &CredentialsHandler{}
			req := httptest.NewRequest(http.MethodPost, "/api/admin/credentials/upload", strings.NewReader("not-json"))
			req.Header.Set("Content-Type", tt.contentType)
			recorder := httptest.NewRecorder()

			handler.UploadCredentials(recorder, req)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
		})
	}
}

func TestUploadCredentialsRecognizesMultipartForm(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("description", "no file"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	handler := &CredentialsHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/credentials/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()

	handler.UploadCredentials(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "请上传文件") {
		t.Fatalf("body = %q, want missing-file error", recorder.Body.String())
	}
}

func TestUploadCredentialsRejectsMissingMultipartPlanBeforeImport(t *testing.T) {
	req := newMultipartCredentialUpload(t, nil, `[{
		"email":"owner@example.com"
	}]`)
	query := req.URL.Query()
	query.Set("plan_type", "query-must-not-be-used")
	req.URL.RawQuery = query.Encode()

	importCalls := 0
	handler := &CredentialsHandler{credentialImport: func(context.Context, []credentialItem) (int, error) {
		importCalls++
		return 0, nil
	}}
	recorder := httptest.NewRecorder()

	handler.UploadCredentials(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if importCalls != 0 {
		t.Fatalf("import calls = %d, want 0", importCalls)
	}
}

func TestUploadCredentialsRejectsInvalidPlanBeforeImport(t *testing.T) {
	for _, planType := range []string{"   ", strings.Repeat("套", 101), "pro\ncontrol"} {
		t.Run(strconv.Itoa(len(planType)), func(t *testing.T) {
			importCalls := 0
			handler := &CredentialsHandler{credentialImport: func(context.Context, []credentialItem) (int, error) {
				importCalls++
				return 0, nil
			}}
			req := httptest.NewRequest(http.MethodPost, "/api/admin/credentials/upload", strings.NewReader(`[{"email":"owner@example.com"}]`))
			req.Header.Set("Content-Type", "application/json")
			query := req.URL.Query()
			query.Set("plan_type", planType)
			req.URL.RawQuery = query.Encode()
			recorder := httptest.NewRecorder()

			handler.UploadCredentials(recorder, req)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if importCalls != 0 {
				t.Fatalf("import calls = %d, want 0", importCalls)
			}
		})
	}
}

func TestUploadCredentialsOverridesFilePlanInDatabaseAndJSON(t *testing.T) {
	selectedPlan := " enterprise "
	req := newMultipartCredentialUpload(t, &selectedPlan, `[
		{"email":"first@example.com","plan_type":"legacy","token":"secret-1"},
		{"email":"second@example.com","token":"secret-2"}
	]`)

	var importedItems []credentialItem
	handler := &CredentialsHandler{credentialImport: func(_ context.Context, items []credentialItem) (int, error) {
		importedItems = append(importedItems, items...)
		return len(items), nil
	}}
	recorder := httptest.NewRecorder()

	handler.UploadCredentials(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if len(importedItems) != 2 {
		t.Fatalf("imported items = %d, want 2", len(importedItems))
	}
	for _, item := range importedItems {
		if item.PlanType != "enterprise" {
			t.Fatalf("database plan_type = %q, want enterprise", item.PlanType)
		}
		var data map[string]any
		if err := json.Unmarshal(item.Data, &data); err != nil {
			t.Fatal(err)
		}
		if data["plan_type"] != "enterprise" {
			t.Fatalf("JSON plan_type = %#v, want enterprise", data["plan_type"])
		}
	}
}

func TestUploadCredentialsJSONCompatibilityUsesQueryPlan(t *testing.T) {
	var importedItems []credentialItem
	handler := &CredentialsHandler{credentialImport: func(_ context.Context, items []credentialItem) (int, error) {
		importedItems = append(importedItems, items...)
		return len(items), nil
	}}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/credentials/upload?plan_type=pro", strings.NewReader(`[{"email":"owner@example.com"}]`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.UploadCredentials(recorder, req)

	if recorder.Code != http.StatusOK || len(importedItems) != 1 || importedItems[0].PlanType != "pro" {
		t.Fatalf("status = %d, imported = %+v; body = %s", recorder.Code, importedItems, recorder.Body.String())
	}
}

func TestNormalizeCredentialsSkipsInvalidExpired(t *testing.T) {
	handler := &CredentialsHandler{}
	rawItems := []map[string]any{
		{"email": "invalid-string@example.com", "plan_type": "pro", "expired": "not-a-date"},
		{"email": "invalid-type@example.com", "plan_type": "pro", "expired": float64(123)},
		{"email": "empty@example.com", "plan_type": "pro", "expired": ""},
		{"email": "missing@example.com", "plan_type": "pro"},
		{"email": "valid@example.com", "plan_type": "pro", "expired": "2030-01-02"},
	}

	items, skipped := handler.normalizeCredentials(rawItems, "pro")

	if skipped != 2 {
		t.Fatalf("skipped = %d, want 2", skipped)
	}
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
	if items[0].Expired.Valid || items[1].Expired.Valid {
		t.Fatal("empty or missing expired value should remain unset")
	}
	if !items[2].Expired.Valid {
		t.Fatal("valid expired value was not parsed")
	}
}

func TestNormalizeCredentialsTrimsAndValidatesIdentityFields(t *testing.T) {
	handler := &CredentialsHandler{}
	rawItems := []map[string]any{
		{"email": "  owner@example.com ", "plan_type": " pro ", "token": "secret"},
		{"email": " ", "plan_type": "pro"},
		{"email": strings.Repeat("x", 321), "plan_type": "pro"},
	}

	items, skipped := handler.normalizeCredentials(rawItems, "enterprise")
	if skipped != 2 || len(items) != 1 {
		t.Fatalf("items = %d, skipped = %d; want 1, 2", len(items), skipped)
	}
	if items[0].Email != "owner@example.com" || items[0].PlanType != "enterprise" {
		t.Fatalf("normalized identity = %q/%q", items[0].Email, items[0].PlanType)
	}
	if !bytes.Contains(items[0].Data, []byte(`"email":"owner@example.com"`)) {
		t.Fatalf("raw data did not receive normalized fields: %s", items[0].Data)
	}
	if !bytes.Contains(items[0].Data, []byte(`"plan_type":"enterprise"`)) {
		t.Fatalf("raw data did not receive selected plan: %s", items[0].Data)
	}
}

func TestNormalizeCredentialPlanTypeRejectsInvalidUTF8(t *testing.T) {
	if _, err := normalizeCredentialPlanType(string([]byte{0xff})); err == nil {
		t.Fatal("invalid UTF-8 plan type was accepted")
	}
}

func newMultipartCredentialUpload(t *testing.T, planType *string, body string) *http.Request {
	t.Helper()
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	if planType != nil {
		if err := writer.WriteField("plan_type", *planType); err != nil {
			t.Fatal(err)
		}
	}
	file, err := writer.CreateFormFile("file", "credentials.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/credentials/upload", &requestBody)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestGetCredentialDoesNotExposeSecretData(t *testing.T) {
	lookup := &credentialLookupStub{credential: testCredential()}
	handler := &CredentialsHandler{credentialLookup: lookup}
	router := chi.NewRouter()
	router.Get("/credentials/{id}", handler.GetCredential)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/credentials/42", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if lookup.requestedID != 42 {
		t.Fatalf("requested ID = %d, want 42", lookup.requestedID)
	}
	var response struct {
		OK         bool                       `json:"ok"`
		Credential map[string]json.RawMessage `json:"credential"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK {
		t.Fatal("response ok = false, want true")
	}
	if _, exists := response.Credential["data"]; exists {
		t.Fatal("credential detail exposed the raw data field")
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte("top-secret")) {
		t.Fatal("credential detail exposed secret data")
	}
}

func TestGetCredentialNotFound(t *testing.T) {
	handler := &CredentialsHandler{credentialLookup: &credentialLookupStub{err: pgx.ErrNoRows}}
	router := chi.NewRouter()
	router.Get("/credentials/{id}", handler.GetCredential)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/credentials/999", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestDownloadCredentialReturnsRawJSONWithoutCaching(t *testing.T) {
	credential := testCredential()
	lookup := &credentialLookupStub{credential: credential}
	handler := &CredentialsHandler{credentialLookup: lookup}
	router := chi.NewRouter()
	router.Get("/credentials/{id}/download", handler.DownloadCredential)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/credentials/42/download", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Disposition"); got != `attachment; filename="credential-42.json"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if !bytes.Equal(recorder.Body.Bytes(), credential.Data) {
		t.Fatalf("body = %q, want raw credential JSON %q", recorder.Body.Bytes(), credential.Data)
	}
}

func testCredential() store.Credential {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	return store.Credential{
		ID:       42,
		Email:    "owner@example.com",
		PlanType: "pro",
		Expired: pgtype.Timestamptz{
			Time:  now.Add(24 * time.Hour),
			Valid: true,
		},
		Data:      json.RawMessage(`{"email":"owner@example.com","token":"top-secret"}`),
		CreatedAt: now,
		UpdatedAt: now,
	}
}
