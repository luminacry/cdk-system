package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"cdk-system/internal/observability"
	"cdk-system/internal/store"
)

const (
	maxSiteTitleCharacters   = 100
	maxSiteLogoBytes         = 2 << 20
	maxSiteLogoDimension     = 2048
	maxSiteSettingsBodyBytes = maxSiteLogoBytes + (64 << 10)
)

var errSiteLogoTooLarge = errors.New("Logo 文件不能超过 2 MiB")

type siteSettingsStore interface {
	GetSiteSettings(context.Context) (store.GetSiteSettingsRow, error)
	GetSiteLogo(context.Context) (store.GetSiteLogoRow, error)
	UpdateSiteSettings(context.Context, store.UpdateSiteSettingsParams) (store.UpdateSiteSettingsRow, error)
}

// SiteSettingsHandler serves the public branding and its administrative update.
type SiteSettingsHandler struct {
	store siteSettingsStore
}

// NewSiteSettingsHandler creates a SiteSettingsHandler.
func NewSiteSettingsHandler(queries *store.Queries) *SiteSettingsHandler {
	return &SiteSettingsHandler{store: queries}
}

type siteSettingsResponse struct {
	SiteTitle string `json:"site_title"`
	HasLogo   bool   `json:"has_logo"`
	LogoURL   string `json:"logo_url"`
	UpdatedAt string `json:"updated_at"`
}

// Get handles GET /api/site-settings.
func (h *SiteSettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.GetSiteSettings(r.Context())
	if err != nil {
		observability.Logger(r.Context()).Error("get site settings failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "获取站点设置失败")
		return
	}

	w.Header().Set("Cache-Control", "no-cache")
	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"settings": makeSiteSettingsResponse(settings.SiteTitle, settings.LogoContentType != nil, settings.UpdatedAt),
	})
}

// Logo handles GET /api/site-logo.
func (h *SiteSettingsHandler) Logo(w http.ResponseWriter, r *http.Request) {
	logo, err := h.store.GetSiteLogo(r.Context())
	if errors.Is(err, pgx.ErrNoRows) {
		RespondError(w, http.StatusNotFound, "尚未设置站点 Logo")
		return
	}
	if err != nil {
		observability.Logger(r.Context()).Error("get site logo failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "获取站点 Logo 失败")
		return
	}

	digest := sha256.Sum256(logo.LogoData)
	w.Header().Set("Content-Type", logo.LogoContentType)
	w.Header().Set("Cache-Control", "public, max-age=300, must-revalidate")
	w.Header().Set("ETag", fmt.Sprintf("\"%x\"", digest))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", logo.UpdatedAt, bytes.NewReader(logo.LogoData))
}

// Update handles POST /api/admin/site-settings.
func (h *SiteSettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		RespondError(w, http.StatusUnsupportedMediaType, "请求必须使用 multipart/form-data")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxSiteSettingsBodyBytes)
	if err := r.ParseMultipartForm(maxSiteSettingsBodyBytes); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			RespondError(w, http.StatusRequestEntityTooLarge, "上传内容过大")
			return
		}
		RespondError(w, http.StatusBadRequest, "解析站点设置失败")
		return
	}
	defer r.MultipartForm.RemoveAll()

	title, err := validateSiteTitle(r.FormValue("site_title"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	removeLogo := false
	if value := strings.TrimSpace(r.FormValue("remove_logo")); value != "" {
		removeLogo, err = strconv.ParseBool(value)
		if err != nil {
			RespondError(w, http.StatusBadRequest, "remove_logo 必须是布尔值")
			return
		}
	}

	logoFiles := r.MultipartForm.File["logo"]
	if len(logoFiles) > 1 {
		RespondError(w, http.StatusBadRequest, "每次只能上传一个 Logo")
		return
	}
	if removeLogo && len(logoFiles) == 1 {
		RespondError(w, http.StatusBadRequest, "不能同时上传和移除 Logo")
		return
	}

	params := store.UpdateSiteSettingsParams{SiteTitle: title}
	if removeLogo {
		params.UpdateLogo = true
	}
	if len(logoFiles) == 1 {
		data, contentType, err := readAndValidateSiteLogo(logoFiles[0])
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errSiteLogoTooLarge) {
				status = http.StatusRequestEntityTooLarge
			}
			RespondError(w, status, err.Error())
			return
		}
		params.UpdateLogo = true
		params.LogoData = data
		params.LogoContentType = contentType
	}

	settings, err := h.store.UpdateSiteSettings(r.Context(), params)
	if err != nil {
		observability.Logger(r.Context()).Error("update site settings failed", "error", err)
		RespondError(w, http.StatusInternalServerError, "保存站点设置失败")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "站点设置已保存",
		"settings": makeSiteSettingsResponse(
			settings.SiteTitle,
			settings.LogoContentType != nil,
			settings.UpdatedAt,
		),
	})
}

func validateSiteTitle(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", errors.New("网站标题必须是有效文本")
	}
	title := strings.TrimSpace(value)
	if title == "" {
		return "", errors.New("网站标题不能为空")
	}
	if utf8.RuneCountInString(title) > maxSiteTitleCharacters {
		return "", fmt.Errorf("网站标题不能超过 %d 个字符", maxSiteTitleCharacters)
	}
	if strings.IndexFunc(title, unicode.IsControl) >= 0 {
		return "", errors.New("网站标题不能包含控制字符")
	}
	return title, nil
}

func readAndValidateSiteLogo(header *multipart.FileHeader) ([]byte, string, error) {
	file, err := header.Open()
	if err != nil {
		return nil, "", errors.New("读取 Logo 失败")
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxSiteLogoBytes+1))
	if err != nil {
		return nil, "", errors.New("读取 Logo 失败")
	}
	if len(data) == 0 {
		return nil, "", errors.New("Logo 文件不能为空")
	}
	if len(data) > maxSiteLogoBytes {
		return nil, "", errSiteLogoTooLarge
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") {
		return nil, "", errors.New("Logo 必须是有效的 PNG 或 JPEG 图片")
	}
	if config.Width < 1 || config.Height < 1 ||
		config.Width > maxSiteLogoDimension || config.Height > maxSiteLogoDimension {
		return nil, "", fmt.Errorf("Logo 尺寸不能超过 %d×%d 像素", maxSiteLogoDimension, maxSiteLogoDimension)
	}

	decoded, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil || decodedFormat != format || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
		return nil, "", errors.New("Logo 图片数据不完整或已损坏")
	}

	contentType := "image/png"
	if format == "jpeg" {
		contentType = "image/jpeg"
	}
	return data, contentType, nil
}

func makeSiteSettingsResponse(title string, hasLogo bool, updatedAt time.Time) siteSettingsResponse {
	logoURL := ""
	if hasLogo {
		logoURL = "/api/site-logo?v=" + strconv.FormatInt(updatedAt.UnixNano(), 10)
	}
	return siteSettingsResponse{
		SiteTitle: title,
		HasLogo:   hasLogo,
		LogoURL:   logoURL,
		UpdatedAt: formatTime(updatedAt),
	}
}
