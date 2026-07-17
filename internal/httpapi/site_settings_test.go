package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"cdk-system/internal/store"
)

type siteSettingsStoreStub struct {
	getSettingsRow store.GetSiteSettingsRow
	getSettingsErr error
	getLogoRow     store.GetSiteLogoRow
	getLogoErr     error
	updateRow      store.UpdateSiteSettingsRow
	updateErr      error
	updateParams   store.UpdateSiteSettingsParams
	updateCalls    int
}

func (s *siteSettingsStoreStub) GetSiteSettings(context.Context) (store.GetSiteSettingsRow, error) {
	return s.getSettingsRow, s.getSettingsErr
}

func (s *siteSettingsStoreStub) GetSiteLogo(context.Context) (store.GetSiteLogoRow, error) {
	return s.getLogoRow, s.getLogoErr
}

func (s *siteSettingsStoreStub) UpdateSiteSettings(_ context.Context, params store.UpdateSiteSettingsParams) (store.UpdateSiteSettingsRow, error) {
	s.updateCalls++
	s.updateParams = params
	return s.updateRow, s.updateErr
}

func TestGetSiteSettingsReturnsPublicBranding(t *testing.T) {
	now := time.Date(2026, time.July, 18, 2, 3, 4, 500, time.UTC)
	contentType := "image/png"
	backend := &siteSettingsStoreStub{getSettingsRow: store.GetSiteSettingsRow{
		SiteTitle:       "Example",
		LogoContentType: &contentType,
		UpdatedAt:       now,
	}}
	handler := &SiteSettingsHandler{store: backend}
	recorder := httptest.NewRecorder()

	handler.Get(recorder, httptest.NewRequest(http.MethodGet, "/api/site-settings", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q", got)
	}
	var response struct {
		OK       bool                 `json:"ok"`
		Settings siteSettingsResponse `json:"settings"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Settings.SiteTitle != "Example" || !response.Settings.HasLogo {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.Settings.LogoURL != "/api/site-logo?v="+strconv.FormatInt(now.UnixNano(), 10) {
		t.Fatalf("logo_url = %q", response.Settings.LogoURL)
	}
	if response.Settings.UpdatedAt != now.Format(time.RFC3339) {
		t.Fatalf("updated_at = %q", response.Settings.UpdatedAt)
	}
}

func TestGetSiteLogoSupportsSecureConditionalCaching(t *testing.T) {
	now := time.Date(2026, time.July, 18, 2, 3, 4, 0, time.UTC)
	data := encodePNG(t, 2, 2)
	backend := &siteSettingsStoreStub{getLogoRow: store.GetSiteLogoRow{
		LogoData:        data,
		LogoContentType: "image/png",
		UpdatedAt:       now,
	}}
	handler := &SiteSettingsHandler{store: backend}
	recorder := httptest.NewRecorder()

	handler.Logo(recorder, httptest.NewRequest(http.MethodGet, "/api/site-logo", nil))

	if recorder.Code != http.StatusOK || !bytes.Equal(recorder.Body.Bytes(), data) {
		t.Fatalf("status/body mismatch: status=%d body=%d bytes", recorder.Code, recorder.Body.Len())
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=300, must-revalidate" {
		t.Fatalf("Cache-Control = %q", got)
	}
	etag := recorder.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag was not set")
	}

	conditionalRequest := httptest.NewRequest(http.MethodGet, "/api/site-logo", nil)
	conditionalRequest.Header.Set("If-None-Match", etag)
	conditionalRecorder := httptest.NewRecorder()
	handler.Logo(conditionalRecorder, conditionalRequest)
	if conditionalRecorder.Code != http.StatusNotModified {
		t.Fatalf("conditional status = %d, want %d", conditionalRecorder.Code, http.StatusNotModified)
	}
	if conditionalRecorder.Body.Len() != 0 {
		t.Fatalf("304 body length = %d, want 0", conditionalRecorder.Body.Len())
	}
}

func TestGetSiteLogoNotFound(t *testing.T) {
	handler := &SiteSettingsHandler{store: &siteSettingsStoreStub{getLogoErr: pgx.ErrNoRows}}
	recorder := httptest.NewRecorder()
	handler.Logo(recorder, httptest.NewRequest(http.MethodGet, "/api/site-logo", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestUpdateSiteSettingsTitleOnly(t *testing.T) {
	now := time.Date(2026, time.July, 18, 2, 3, 4, 0, time.UTC)
	contentType := "image/jpeg"
	backend := &siteSettingsStoreStub{updateRow: store.UpdateSiteSettingsRow{
		SiteTitle:       "New title",
		LogoContentType: &contentType,
		UpdatedAt:       now,
	}}
	handler := &SiteSettingsHandler{store: backend}
	request := newSiteSettingsRequest(t, map[string]string{"site_title": "  New title  "}, nil)
	recorder := httptest.NewRecorder()

	handler.Update(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if backend.updateCalls != 1 || backend.updateParams.SiteTitle != "New title" {
		t.Fatalf("update call = %d, params = %+v", backend.updateCalls, backend.updateParams)
	}
	if backend.updateParams.UpdateLogo {
		t.Fatal("title-only request unexpectedly changed the logo")
	}
	if !strings.Contains(recorder.Body.String(), `"message":"站点设置已保存"`) {
		t.Fatalf("response does not contain success message: %s", recorder.Body.String())
	}
}

func TestUpdateSiteSettingsUploadsValidatedPNG(t *testing.T) {
	now := time.Date(2026, time.July, 18, 2, 3, 4, 0, time.UTC)
	contentType := "image/png"
	data := encodePNG(t, 4, 3)
	backend := &siteSettingsStoreStub{updateRow: store.UpdateSiteSettingsRow{
		SiteTitle:       "Brand",
		LogoContentType: &contentType,
		UpdatedAt:       now,
	}}
	handler := &SiteSettingsHandler{store: backend}
	request := newSiteSettingsRequest(t, map[string]string{"site_title": "Brand"}, &testUpload{
		filename: "logo.txt",
		data:     data,
	})
	recorder := httptest.NewRecorder()

	handler.Update(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !backend.updateParams.UpdateLogo || backend.updateParams.LogoContentType != "image/png" {
		t.Fatalf("params = %+v", backend.updateParams)
	}
	if !bytes.Equal(backend.updateParams.LogoData, data) {
		t.Fatal("stored logo differs from uploaded bytes")
	}
}

func TestUpdateSiteSettingsAcceptsJPEGAndIgnoresDeclaredType(t *testing.T) {
	var data bytes.Buffer
	if err := jpeg.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	contentType := "image/jpeg"
	backend := &siteSettingsStoreStub{updateRow: store.UpdateSiteSettingsRow{
		SiteTitle:       "Brand",
		LogoContentType: &contentType,
		UpdatedAt:       time.Now(),
	}}
	handler := &SiteSettingsHandler{store: backend}
	recorder := httptest.NewRecorder()
	handler.Update(recorder, newSiteSettingsRequest(t, map[string]string{"site_title": "Brand"}, &testUpload{
		filename: "actually-a-jpeg.png",
		data:     data.Bytes(),
	}))

	if recorder.Code != http.StatusOK || backend.updateParams.LogoContentType != "image/jpeg" {
		t.Fatalf("status = %d, content type = %q; body = %s", recorder.Code, backend.updateParams.LogoContentType, recorder.Body.String())
	}
}

func TestUpdateSiteSettingsRemovesLogo(t *testing.T) {
	backend := &siteSettingsStoreStub{updateRow: store.UpdateSiteSettingsRow{
		SiteTitle: "Brand",
		UpdatedAt: time.Now(),
	}}
	handler := &SiteSettingsHandler{store: backend}
	recorder := httptest.NewRecorder()
	handler.Update(recorder, newSiteSettingsRequest(t, map[string]string{
		"site_title":  "Brand",
		"remove_logo": "true",
	}, nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !backend.updateParams.UpdateLogo || len(backend.updateParams.LogoData) != 0 || backend.updateParams.LogoContentType != "" {
		t.Fatalf("remove params = %+v", backend.updateParams)
	}
}

func TestUpdateSiteSettingsRejectsInvalidInputBeforeWriting(t *testing.T) {
	validPNG := encodePNG(t, 1, 1)
	truncatedPNG := validPNG[:len(validPNG)/2]
	if _, format, err := image.DecodeConfig(bytes.NewReader(truncatedPNG)); err != nil || format != "png" {
		t.Fatalf("truncated PNG must retain a valid header for this test: format=%q err=%v", format, err)
	}
	oversizedDimensions := encodePNG(t, maxSiteLogoDimension+1, 1)
	var gifData bytes.Buffer
	if err := gif.Encode(&gifData, image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black}), nil); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		fields     map[string]string
		upload     *testUpload
		wantStatus int
	}{
		{name: "blank title", fields: map[string]string{"site_title": "   "}},
		{name: "title too long", fields: map[string]string{"site_title": strings.Repeat("界", maxSiteTitleCharacters+1)}},
		{name: "control character", fields: map[string]string{"site_title": "bad\ntitle"}},
		{name: "invalid remove flag", fields: map[string]string{"site_title": "Brand", "remove_logo": "sometimes"}},
		{name: "upload and remove", fields: map[string]string{"site_title": "Brand", "remove_logo": "true"}, upload: &testUpload{filename: "logo.png", data: validPNG}},
		{name: "not an image", fields: map[string]string{"site_title": "Brand"}, upload: &testUpload{filename: "logo.png", data: []byte("not an image")}},
		{name: "unsupported GIF", fields: map[string]string{"site_title": "Brand"}, upload: &testUpload{filename: "logo.gif", data: gifData.Bytes()}},
		{name: "truncated image", fields: map[string]string{"site_title": "Brand"}, upload: &testUpload{filename: "logo.png", data: truncatedPNG}},
		{name: "dimensions too large", fields: map[string]string{"site_title": "Brand"}, upload: &testUpload{filename: "logo.png", data: oversizedDimensions}},
		{name: "file too large", fields: map[string]string{"site_title": "Brand"}, upload: &testUpload{filename: "logo.png", data: make([]byte, maxSiteLogoBytes+1)}, wantStatus: http.StatusRequestEntityTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &siteSettingsStoreStub{}
			handler := &SiteSettingsHandler{store: backend}
			recorder := httptest.NewRecorder()
			handler.Update(recorder, newSiteSettingsRequest(t, tt.fields, tt.upload))
			wantStatus := tt.wantStatus
			if wantStatus == 0 {
				wantStatus = http.StatusBadRequest
			}
			if recorder.Code != wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, wantStatus, recorder.Body.String())
			}
			if backend.updateCalls != 0 {
				t.Fatalf("update calls = %d, want 0", backend.updateCalls)
			}
		})
	}
}

func TestUpdateSiteSettingsRequiresMultipartAndCapsTotalBody(t *testing.T) {
	backend := &siteSettingsStoreStub{}
	handler := &SiteSettingsHandler{store: backend}

	nonMultipart := httptest.NewRequest(http.MethodPost, "/api/admin/site-settings", strings.NewReader(`{"site_title":"Brand"}`))
	nonMultipart.Header.Set("Content-Type", "application/json")
	nonMultipartRecorder := httptest.NewRecorder()
	handler.Update(nonMultipartRecorder, nonMultipart)
	if nonMultipartRecorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("non-multipart status = %d, want %d", nonMultipartRecorder.Code, http.StatusUnsupportedMediaType)
	}

	largeRequest := newSiteSettingsRequest(t, map[string]string{
		"site_title": "Brand",
		"padding":    strings.Repeat("x", maxSiteSettingsBodyBytes),
	}, nil)
	largeRecorder := httptest.NewRecorder()
	handler.Update(largeRecorder, largeRequest)
	if largeRecorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large request status = %d, want %d; body = %s", largeRecorder.Code, http.StatusRequestEntityTooLarge, largeRecorder.Body.String())
	}
	if backend.updateCalls != 0 {
		t.Fatalf("update calls = %d, want 0", backend.updateCalls)
	}
}

type testUpload struct {
	filename string
	data     []byte
}

func newSiteSettingsRequest(t *testing.T, fields map[string]string, upload *testUpload) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if upload != nil {
		part, err := writer.CreateFormFile("logo", upload.filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(upload.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/admin/site-settings", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func encodePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.NRGBA{R: 12, G: 34, B: 56, A: 255})
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
