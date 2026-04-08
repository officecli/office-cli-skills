package app

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	publishsvc "github.com/officecli/officecli/platform/internal/publish"
)

type fakePublishService struct {
	result *publishsvc.Result
	err    error
	req    publishsvc.Request
	auth   string
}

func (f *fakePublishService) Publish(_ context.Context, bearer string, req publishsvc.Request) (*publishsvc.Result, error) {
	f.auth = bearer
	f.req = req
	if req.Reader != nil {
		data, _ := io.ReadAll(req.Reader)
		f.req.Reader = bytes.NewReader(data)
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestRegisterPublishRoutesRejectsMissingAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	registerPublishRoutes(api, Config{PublishMaxFileBytes: 1024, PublishRateLimitPerMinute: 10}, &fakePublishService{})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "demo.docx")
	_, _ = part.Write([]byte("hello"))
	_ = writer.WriteField("document_type", "docx")
	_ = writer.WriteField("document_name", "demo.docx")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/publish", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestRegisterPublishRoutesReturnsPublishPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	expiresAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	fake := &fakePublishService{result: &publishsvc.Result{
		AccessURL: "https://preview.example.com/share/1",
		Password:  "123456",
		FileID:    "file-1",
		ExpiresAt: &expiresAt,
	}}
	registerPublishRoutes(api, Config{PublishMaxFileBytes: 1024, PublishRateLimitPerMinute: 10}, fake)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "demo.docx")
	_, _ = part.Write([]byte("hello"))
	_ = writer.WriteField("document_type", "docx")
	_ = writer.WriteField("document_name", "demo.docx")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/publish", &body)
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"access_url":"https://preview.example.com/share/1"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if fake.auth != "Bearer test-key" {
		t.Fatalf("auth = %q", fake.auth)
	}
	if fake.req.DocumentType != "docx" {
		t.Fatalf("document type = %q", fake.req.DocumentType)
	}
	if fake.req.DocumentName != "demo.docx" {
		t.Fatalf("document name = %q", fake.req.DocumentName)
	}
	data, _ := io.ReadAll(fake.req.Reader)
	if string(data) != "hello" {
		t.Fatalf("file body = %q", string(data))
	}
}

func TestRegisterPublishRoutesAcceptsFileBeforeMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	fake := &fakePublishService{result: &publishsvc.Result{
		AccessURL: "https://preview.example.com/share/2",
		Password:  "654321",
		FileID:    "file-2",
	}}
	registerPublishRoutes(api, Config{PublishMaxFileBytes: 1024, PublishRateLimitPerMinute: 10}, fake)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "deck.pptx")
	_, _ = part.Write([]byte("ppt-bytes"))
	_ = writer.WriteField("document_type", "pptx")
	_ = writer.WriteField("document_name", "deck.pptx")
	_ = writer.WriteField("expires_in_seconds", "3600")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/publish", &body)
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if fake.req.DocumentType != "pptx" {
		t.Fatalf("document type = %q", fake.req.DocumentType)
	}
	if fake.req.DocumentName != "deck.pptx" {
		t.Fatalf("document name = %q", fake.req.DocumentName)
	}
	if fake.req.ExpiresInSeconds != 3600 {
		t.Fatalf("expires_in_seconds = %d", fake.req.ExpiresInSeconds)
	}
	data, _ := io.ReadAll(fake.req.Reader)
	if string(data) != "ppt-bytes" {
		t.Fatalf("file body = %q", string(data))
	}
}
