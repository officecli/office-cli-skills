package officesdk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	sdkoffice "github.com/officesdk/go-sdk/officesdk"

	"github.com/officecli/officecli-internal/platform/internal/previewshare"
)

type fileObjectStore interface {
	PutObject(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, key string) error
}

type FileProvider struct {
	store   *FileStore
	objects fileObjectStore
	shares  *previewshare.Service
}

func NewFileProvider(store *FileStore, objects fileObjectStore, shares *previewshare.Service) *FileProvider {
	return &FileProvider{store: store, objects: objects, shares: shares}
}

func (p *FileProvider) AuthorizeShareFile(c *gin.Context, fileID string) error {
	if p == nil || p.shares == nil {
		return nil
	}
	_, _, err := p.shares.RequireShareAccess(c, fileID)
	return err
}

func (p *FileProvider) VerifyFile(c *gin.Context, fileID string) (*sdkoffice.VerifyResponse, error) {
	if err := p.AuthorizeShareFile(c, fileID); err != nil {
		return nil, err
	}
	return &sdkoffice.VerifyResponse{
		CurrentUserInfo: sdkoffice.UserInfo{
			ID:   "preview_user",
			Name: "OfficeCLI Preview",
			Permissions: map[string]bool{
				"read": true, "download": false, "print": false, "update": false,
			},
		},
	}, nil
}

func (p *FileProvider) GetFile(c *gin.Context, fileID string) (*sdkoffice.FileResponse, error) {
	if err := p.AuthorizeShareFile(c, fileID); err != nil {
		return nil, err
	}
	meta, err := p.ResolveFileMeta(c.Request.Context(), fileID)
	if err != nil {
		return nil, fmt.Errorf("file not found: %s", fileID)
	}
	return &sdkoffice.FileResponse{
		ID:         meta.ID,
		Name:       meta.Name,
		Version:    meta.Version,
		FromSDK:    meta.FromSDK,
		CreateTime: meta.CreateTime,
		ModifyTime: meta.ModifyTime,
		CreatorID:  meta.CreatorID,
		ModifierID: meta.ModifierID,
	}, nil
}

func (p *FileProvider) GetFileDownload(c *gin.Context, fileID string) (*sdkoffice.DownloadResponse, error) {
	if err := p.AuthorizeShareFile(c, fileID); err != nil {
		return nil, err
	}
	meta, err := p.ResolveFileMeta(c.Request.Context(), fileID)
	if err != nil {
		return nil, fmt.Errorf("file not found: %s", fileID)
	}
	return &sdkoffice.DownloadResponse{URL: p.proxyDownloadURL(c, meta.StorageKey)}, nil
}

func (p *FileProvider) GetFileWatermark(_ *gin.Context, _ string) (*sdkoffice.WatermarkResponse, error) {
	return &sdkoffice.WatermarkResponse{Type: 0}, nil
}

func (p *FileProvider) GetUploadURL(c *gin.Context, fileID string) (*sdkoffice.UploadURLResponse, error) {
	if err := p.AuthorizeShareFile(c, fileID); err != nil {
		return nil, err
	}
	var body struct {
		ObjectName  string `json:"object_name"`
		ContentType string `json:"content_type"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.ObjectName) == "" {
		return nil, fmt.Errorf("object_name is required")
	}
	storageKey := fmt.Sprintf("officesdk/%s/content/%s", fileID, path.Clean(strings.TrimPrefix(body.ObjectName, "/")))
	contentType := strings.TrimSpace(body.ContentType)
	if contentType == "" {
		contentType = "application/json"
	}
	if err := p.store.SetUploadInfo(c.Request.Context(), storageKey, &UploadInfo{ContentType: contentType}); err != nil {
		return nil, err
	}
	return &sdkoffice.UploadURLResponse{
		URL:              p.proxyUploadURL(c, storageKey),
		Method:           http.MethodPut,
		Headers:          map[string]string{"Content-Type": contentType},
		CompletionParams: map[string]string{"storage_key": storageKey},
	}, nil
}

func (p *FileProvider) CompleteUpload(c *gin.Context, fileID string) (*sdkoffice.UploadCompletionResponse, error) {
	if err := p.AuthorizeShareFile(c, fileID); err != nil {
		return nil, err
	}
	meta, err := p.ResolveFileMeta(c.Request.Context(), fileID)
	if err != nil {
		return nil, fmt.Errorf("file not found: %s", fileID)
	}
	now := time.Now().Unix()
	meta.ModifyTime = now
	pendingKey, _ := p.pendingContentStorageKey(c.Request.Context(), fileID)
	initialImport := pendingKey != "" && !isSDKContentStorageKey(fileID, meta.StorageKey)
	if !initialImport {
		meta.FromSDK = true
		meta.Version++
	}
	if err := p.store.SetFileMeta(c.Request.Context(), meta); err != nil {
		return nil, err
	}
	return &sdkoffice.UploadCompletionResponse{
		ID:         meta.ID,
		Version:    int(meta.Version),
		CreateTime: meta.CreateTime,
		ModifyTime: meta.ModifyTime,
		CreatorID:  meta.CreatorID,
		ModifierID: meta.ModifierID,
	}, nil
}

func (p *FileProvider) GetDownloadURL(c *gin.Context, fileID string) (*sdkoffice.DownloadResponse, error) {
	if err := p.AuthorizeShareFile(c, fileID); err != nil {
		return nil, err
	}
	objectName := strings.TrimSpace(c.Query("object_name"))
	if objectName == "" {
		return nil, fmt.Errorf("object_name is required")
	}
	if objectName == "content" {
		if pendingKey, err := p.pendingContentStorageKey(c.Request.Context(), fileID); err == nil && pendingKey != "" {
			return &sdkoffice.DownloadResponse{URL: p.proxyDownloadURL(c, pendingKey)}, nil
		}
		meta, err := p.ResolveFileMeta(c.Request.Context(), fileID)
		if err != nil {
			return nil, fmt.Errorf("file not found: %s", fileID)
		}
		return &sdkoffice.DownloadResponse{URL: p.proxyDownloadURL(c, meta.StorageKey)}, nil
	}
	storageKey := fmt.Sprintf("officesdk/%s/content/%s", fileID, path.Clean(strings.TrimPrefix(objectName, "/")))
	return &sdkoffice.DownloadResponse{URL: p.proxyDownloadURL(c, storageKey)}, nil
}

func (p *FileProvider) GetAssetUploadURL(c *gin.Context, fileID string) (*sdkoffice.AssetUploadURLResponse, error) {
	if err := p.AuthorizeShareFile(c, fileID); err != nil {
		return nil, err
	}
	var body struct {
		ObjectName  string `json:"object_name"`
		ContentType string `json:"content_type"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.ObjectName) == "" {
		return nil, fmt.Errorf("object_name is required")
	}
	storageKey := fmt.Sprintf("officesdk/%s/assets/%s", fileID, path.Clean(strings.TrimPrefix(body.ObjectName, "/")))
	contentType := strings.TrimSpace(body.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if err := p.store.SetUploadInfo(c.Request.Context(), storageKey, &UploadInfo{ContentType: contentType}); err != nil {
		return nil, err
	}
	return &sdkoffice.AssetUploadURLResponse{
		URL:              p.proxyUploadURL(c, storageKey),
		Method:           http.MethodPut,
		FileFieldKey:     "file",
		Headers:          map[string]string{"Content-Type": contentType},
		CompletionParams: map[string]string{"storage_key": storageKey},
	}, nil
}

func (p *FileProvider) AssetCompleteUpload(c *gin.Context, fileID string) (*sdkoffice.UploadCompletionResponse, error) {
	return p.CompleteUpload(c, fileID)
}

func (p *FileProvider) GetAssetDownloadURL(c *gin.Context, fileID string) (*sdkoffice.DownloadResponse, error) {
	if err := p.AuthorizeShareFile(c, fileID); err != nil {
		return nil, err
	}
	objectName := strings.TrimSpace(c.Query("object_name"))
	if objectName == "" {
		return nil, fmt.Errorf("object_name is required")
	}
	return &sdkoffice.DownloadResponse{
		URL: p.proxyDownloadURL(c, fmt.Sprintf("officesdk/%s/assets/%s", fileID, path.Clean(strings.TrimPrefix(objectName, "/")))),
	}, nil
}

func (p *FileProvider) CreateAssetsFile(_ *gin.Context, fileID string) (*sdkoffice.CreateAssetsResponse, error) {
	return &sdkoffice.CreateAssetsResponse{ID: fileID, Size: 1}, nil
}

func (p *FileProvider) ReadObject(ctx context.Context, storageKey string) ([]byte, error) {
	if p == nil || p.objects == nil {
		return nil, fmt.Errorf("preview object store unavailable")
	}
	obj, err := p.objects.GetObject(ctx, strings.TrimSpace(storageKey))
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(obj)
}

func (p *FileProvider) pendingContentStorageKey(ctx context.Context, fileID string) (string, error) {
	if p != nil && p.store != nil {
		if pendingKey, err := p.store.GetPendingContentStorageKey(ctx, fileID); err == nil && pendingKey != "" {
			return pendingKey, nil
		}
	}
	derivedKey := sdkContentStorageKey(fileID)
	if !p.hasObject(ctx, derivedKey) {
		return "", fmt.Errorf("pending content not found: %s", fileID)
	}
	if p != nil && p.store != nil {
		_ = p.store.SetPendingContentStorageKey(ctx, fileID, derivedKey)
	}
	return derivedKey, nil
}

func (p *FileProvider) ResolveFileMeta(ctx context.Context, fileID string) (*FileMeta, error) {
	normalizedFileID := strings.TrimSpace(fileID)
	if normalizedFileID == "" {
		return nil, fmt.Errorf("file not found: %s", fileID)
	}
	if p != nil && p.store != nil {
		meta, err := p.store.GetFileMeta(ctx, normalizedFileID)
		if err == nil {
			return meta, nil
		}
		if !isFileMetaNotFound(err) {
			return nil, err
		}
	}
	if p != nil && p.shares != nil {
		share, err := p.shares.GetByFileID(ctx, normalizedFileID)
		if err == nil {
			return previewShareToFileMeta(share), nil
		}
		if err != nil && !errors.Is(err, previewshare.ErrNotFound) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("file not found: %s", normalizedFileID)
}

func (p *FileProvider) HandleProxyUpload(c *gin.Context) {
	storageKey := strings.TrimSpace(c.Query("key"))
	if storageKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}
	if fileID, ok := parseFileIDFromStorageKey(storageKey); ok {
		if err := p.AuthorizeShareFile(c, fileID); err != nil {
			c.JSON(previewAccessHTTPStatus(err), gin.H{"error": err.Error()})
			return
		}
	}
	body, err := extractUploadBody(c.Request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}
	info, err := p.store.GetUploadInfo(c.Request.Context(), storageKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "upload info not found"})
		return
	}
	if err := p.objects.PutObject(c.Request.Context(), storageKey, bytes.NewReader(body), int64(len(body)), info.ContentType); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "upload failed"})
		return
	}
	if fileID, ok := parseFileIDFromStorageKey(storageKey); ok && strings.Contains(storageKey, "/content/") {
		_ = p.store.SetPendingContentStorageKey(c.Request.Context(), fileID, storageKey)
	}
	c.Status(http.StatusOK)
}

func (p *FileProvider) HandleProxyDownload(c *gin.Context) {
	storageKey := strings.TrimSpace(c.Query("key"))
	if storageKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}
	if fileID, ok := parseFileIDFromStorageKey(storageKey); ok {
		if p != nil && p.shares != nil {
			if _, status, err := p.shares.RequireShareDownloadAccess(c, fileID, storageKey); err != nil {
				c.JSON(status, gin.H{"error": err.Error()})
				return
			}
		} else {
			if err := p.AuthorizeShareFile(c, fileID); err != nil {
				c.JSON(previewAccessHTTPStatus(err), gin.H{"error": err.Error()})
				return
			}
		}
	}
	obj, err := p.objects.GetObject(c.Request.Context(), storageKey)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "failed to get file"})
		return
	}
	defer obj.Close()

	filename, contentType := p.downloadMetadata(c.Request.Context(), storageKey)
	if contentType != "" {
		c.Header("Content-Type", contentType)
	}
	if filename != "" {
		c.Header("Content-Disposition", buildAttachmentDisposition(filename))
	}
	c.Header("Cache-Control", "private, max-age=60")
	c.Header("X-Content-Type-Options", "nosniff")
	c.DataFromReader(http.StatusOK, -1, contentType, obj, nil)
}

func (p *FileProvider) downloadMetadata(ctx context.Context, storageKey string) (string, string) {
	if fileID, ok := parseFileIDFromStorageKey(storageKey); ok {
		if meta, err := p.ResolveFileMeta(ctx, fileID); err == nil && meta != nil && meta.StorageKey == storageKey {
			return meta.Name, normalizeDownloadContentType(meta.Name, storageKey)
		}
	}
	name := path.Base(storageKey)
	return name, normalizeDownloadContentType(name, storageKey)
}

func isFileMetaNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "file not found")
}

func previewShareToFileMeta(share *previewshare.PreviewShare) *FileMeta {
	if share == nil {
		return nil
	}
	return &FileMeta{
		ID:         share.FileID,
		Name:       share.FileName,
		StorageKey: share.StorageKey,
		Version:    1,
		FromSDK:    false,
		CreateTime: share.CreatedAt.UTC().Unix(),
		ModifyTime: share.UpdatedAt.UTC().Unix(),
		CreatorID:  "publish",
		ModifierID: "publish",
	}
}

func (p *FileProvider) hasObject(ctx context.Context, storageKey string) bool {
	if p == nil || p.objects == nil || strings.TrimSpace(storageKey) == "" {
		return false
	}
	obj, err := p.objects.GetObject(ctx, storageKey)
	if err != nil {
		return false
	}
	_ = obj.Close()
	return true
}

func sdkContentStorageKey(fileID string) string {
	return fmt.Sprintf("officesdk/%s/content/content", strings.TrimSpace(fileID))
}

func isSDKContentStorageKey(fileID, storageKey string) bool {
	return strings.TrimSpace(storageKey) == sdkContentStorageKey(fileID)
}

func normalizeDownloadContentType(name, storageKey string) string {
	if strings.HasSuffix(storageKey, "/content/content") || name == "content" {
		return "application/json"
	}
	if guessed := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); guessed != "" {
		return guessed
	}
	return "application/octet-stream"
}

func extractUploadBody(req *http.Request) ([]byte, error) {
	contentType := req.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		reader, err := req.MultipartReader()
		if err != nil {
			return nil, err
		}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if part.FileName() == "" {
				_, _ = io.Copy(io.Discard, part)
				_ = part.Close()
				continue
			}
			data, readErr := io.ReadAll(part)
			_ = part.Close()
			if readErr != nil {
				return nil, readErr
			}
			return data, nil
		}
		return nil, fmt.Errorf("multipart upload missing file part")
	}
	return io.ReadAll(req.Body)
}

func parseFileIDFromStorageKey(storageKey string) (string, bool) {
	parts := strings.Split(strings.Trim(strings.TrimSpace(storageKey), "/"), "/")
	if len(parts) < 2 {
		return "", false
	}
	switch parts[0] {
	case "preview", "officesdk":
		return parts[1], parts[1] != ""
	default:
		return "", false
	}
}

func (p *FileProvider) proxyUploadURL(c *gin.Context, storageKey string) string {
	return requestBaseURL(c.Request) + "/officesdk/storage/upload?key=" + urlQueryEscape(storageKey)
}

func (p *FileProvider) proxyDownloadURL(c *gin.Context, storageKey string) string {
	downloadURL := requestBaseURL(c.Request) + "/officesdk/proxy/download?key=" + urlQueryEscape(storageKey)
	if p == nil || p.shares == nil {
		return downloadURL
	}
	if fileID, ok := parseFileIDFromStorageKey(storageKey); ok {
		if token, err := p.shares.IssueDownloadToken(c.Request.Context(), fileID, storageKey); err == nil && token != "" {
			return downloadURL + "&access_token=" + urlQueryEscape(token)
		}
	}
	return downloadURL
}

func buildAttachmentDisposition(filename string) string {
	name := strings.ReplaceAll(filename, "\"", "")
	return fmt.Sprintf("attachment; filename=\"%s\"", name)
}
