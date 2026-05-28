package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/officecli/officecli/platform/internal/admin"
	"github.com/officecli/officecli/platform/internal/model"
	sqlstore "github.com/officecli/officecli/platform/internal/store/sqlstore"
)

type routeImageTemplateObjectStore struct {
	objects map[string][]byte
}

func (f *routeImageTemplateObjectStore) PutObject(_ context.Context, key string, reader io.Reader, _ int64, _ string) error {
	if f.objects == nil {
		f.objects = map[string][]byte{}
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	f.objects[key] = data
	return nil
}

func (f *routeImageTemplateObjectStore) GetObject(_ context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.objects[key])), nil
}

func testImageTemplateRouter(t *testing.T) (*gin.Engine, *admin.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:image_prompt_template_routes?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ImagePromptTemplate{}, &model.AdminAuditLog{}))
	store := sqlstore.NewWithDB(db)
	svc := admin.NewService(store, nil, "secret", time.Hour, "cookie", testRouteCodec{}, "salt", nil, nil, nil)
	svc.SetImageTemplateObjectStore(&routeImageTemplateObjectStore{})

	router := gin.New()
	registerImageTemplateRoutes(router.Group("/api"), svc)
	return router, svc
}

type testRouteCodec struct{}

func (testRouteCodec) Encode(v string) (string, error) { return v, nil }
func (testRouteCodec) Decode(v string) (string, error) { return v, nil }

func TestImageTemplatePublicRoutesListThumbnailAndCompose(t *testing.T) {
	router, svc := testImageTemplateRouter(t)

	enabled, err := svc.CreateImagePromptTemplate(context.Background(), admin.UpsertImagePromptTemplateRequest{
		Slug:         "poster",
		Title:        "Poster",
		Description:  "Poster template",
		PromptPreset: "cinematic poster preset",
		SortOrder:    10,
		Enabled:      true,
	})
	require.NoError(t, err)
	disabled, err := svc.CreateImagePromptTemplate(context.Background(), admin.UpsertImagePromptTemplateRequest{
		Slug:         "disabled",
		Title:        "Disabled",
		PromptPreset: "disabled preset",
		SortOrder:    1,
		Enabled:      false,
	})
	require.NoError(t, err)
	_, err = svc.UploadImagePromptTemplateThumbnail(context.Background(), enabled.ID, "thumb.png", "image/png", bytes.NewReader([]byte("png")), 3)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/image-templates", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotContains(t, rec.Body.String(), "prompt_preset")
	require.Contains(t, rec.Body.String(), "Poster")
	require.NotContains(t, rec.Body.String(), "Disabled")
	require.Contains(t, rec.Body.String(), "/api/image-templates/")

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/image-templates/%d/thumbnail", enabled.ID), nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "image/png", rec.Header().Get("Content-Type"))
	require.Equal(t, "png", rec.Body.String())

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/image-templates/%d/compose", enabled.ID), bytes.NewReader([]byte(`{"prompt":"red bicycle"}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var envelope struct {
		Data admin.ComposeImagePromptTemplateResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Contains(t, envelope.Data.Prompt, "cinematic poster preset")
	require.Contains(t, envelope.Data.Prompt, "red bicycle")

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/image-templates/%d/compose", disabled.ID), bytes.NewReader([]byte(`{"prompt":"red bicycle"}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "disabled")
}
