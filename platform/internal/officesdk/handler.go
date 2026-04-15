package officesdk

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	store     *FileStore
	provider  *FileProvider
	endpoint  string
	jwtSecret string
}

func NewHandler(store *FileStore, provider *FileProvider, endpoint, jwtSecret string) *Handler {
	return &Handler{
		store:     store,
		provider:  provider,
		endpoint:  strings.TrimSpace(endpoint),
		jwtSecret: strings.TrimSpace(jwtSecret),
	}
}

func (h *Handler) GetSDKParams(c *gin.Context) {
	fileID := strings.TrimSpace(c.Query("file_id"))
	if fileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "file_id is required"})
		return
	}
	if err := h.provider.AuthorizeShareFile(c, fileID); err != nil {
		c.JSON(previewAccessHTTPStatus(err), gin.H{"code": previewAccessHTTPStatus(err), "message": err.Error()})
		return
	}
	meta, err := h.provider.ResolveFileMeta(c.Request.Context(), fileID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "file not found"})
		return
	}
	fileType := GetFileTypeEnum(meta.Name)
	if fileType < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "unsupported file type"})
		return
	}
	endpoint := h.effectiveEndpoint(c.Request)
	appID := meta.AppID
	if appID == "" {
		appID = ExtractSDKAppID(endpoint)
	}
	params := SDKParams{
		Endpoint: endpoint,
		Path:     ResolveSDKFilePagePath(endpoint),
		Token:    SignToken(h.jwtSecret, appID, fileID),
		TokenURL: requestBaseURL(c.Request) + "/officesdk/sdk-params?file_id=" + url.QueryEscape(fileID),
		FileID:   fileID,
		FileType: fileType,
		Version:  meta.Version,
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": params})
}

func (h *Handler) ServePage(c *gin.Context) {
	fileID := strings.TrimSpace(c.Query("file_id"))
	if fileID == "" {
		c.String(http.StatusBadRequest, "file_id is required")
		return
	}
	if err := h.provider.AuthorizeShareFile(c, fileID); err != nil {
		c.String(previewAccessHTTPStatus(err), err.Error())
		return
	}
	h.ServePageForFile(c, fileID)
}

func (h *Handler) ServePageForFile(c *gin.Context, fileID string) {
	meta, err := h.provider.ResolveFileMeta(c.Request.Context(), fileID)
	if err != nil {
		c.String(http.StatusNotFound, "file not found")
		return
	}
	fileType := GetFileTypeEnum(meta.Name)
	if fileType < 0 {
		c.String(http.StatusBadRequest, "unsupported file type")
		return
	}
	endpoint := h.effectiveEndpoint(c.Request)
	appID := meta.AppID
	if appID == "" {
		appID = ExtractSDKAppID(endpoint)
	}
	html := RenderOfficePage(endpoint, fileID, SignToken(h.jwtSecret, appID, fileID), fileType, true)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

func (h *Handler) effectiveEndpoint(r *http.Request) string {
	if h.endpoint != "" {
		return h.endpoint
	}
	return requestBaseURL(r) + defaultSDKPathPrefix
}

func requestBaseURL(r *http.Request) string {
	if r == nil {
		return ""
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		return ""
	}
	hostName := host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		hostName = parsedHost
	} else {
		hostName = strings.Trim(host, "[]")
	}
	if hostName == "" {
		return ""
	}
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	return proto + "://" + host
}

func previewAccessHTTPStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not found"):
		return http.StatusNotFound
	case strings.Contains(msg, "expired"):
		return http.StatusGone
	case strings.Contains(msg, "disabled"):
		return http.StatusForbidden
	case strings.Contains(msg, "login is required"), strings.Contains(msg, "unauthorized"):
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

func urlQueryEscape(value string) string {
	return url.QueryEscape(strings.TrimSpace(value))
}
