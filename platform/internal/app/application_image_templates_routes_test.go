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
	"github.com/officecli/officecli/platform/internal/auth"
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
	require.Contains(t, rec.Body.String(), `"prompt_preset":"cinematic poster preset"`)
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

func TestImageTemplatePublicRouteSerializesSlots(t *testing.T) {
	router, svc := testImageTemplateRouter(t)

	_, err := svc.CreateImagePromptTemplate(context.Background(), admin.UpsertImagePromptTemplateRequest{
		Slug:         "poster-slots",
		Title:        "Poster",
		Description:  "Poster template",
		PromptPreset: "A {{product}} poster",
		SortOrder:    10,
		Enabled:      true,
		Slots: []admin.ImagePromptSlot{
			{Key: "product", Label: "Product", Example: "running shoes", DefaultValue: "a product", Required: true, Multiline: true},
		},
	})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/image-templates", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var envelope struct {
		Data []admin.ImagePromptTemplateResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))

	var slotted *admin.ImagePromptTemplateResponse
	for i := range envelope.Data {
		if envelope.Data[i].Slug == "poster-slots" {
			slotted = &envelope.Data[i]
			break
		}
	}
	require.NotNil(t, slotted, "poster-slots template must be present")
	require.Len(t, slotted.Slots, 1)
	require.Equal(t, "product", slotted.Slots[0].Key)
	require.Equal(t, "a product", slotted.Slots[0].DefaultValue)
	require.True(t, slotted.Slots[0].Required)
	require.True(t, slotted.Slots[0].Multiline)

	// snake_case wire shape (must align with officedex bridge mapping).
	body := rec.Body.String()
	require.Contains(t, body, `"slots":[`)
	require.Contains(t, body, `"default_value":"a product"`)
}

func TestImageTemplateUserRoutesCopyPublicTemplateIntoPrivateLibrary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:image_prompt_template_user_routes?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ImagePromptTemplate{}, &model.ImageGenerationProvenance{}, &model.ImageTemplatePublishRequest{}, &model.AdminAuditLog{}))
	store := sqlstore.NewWithDB(db)
	svc := admin.NewService(store, nil, "secret", time.Hour, "cookie", testRouteCodec{}, "salt", nil, nil, nil)
	svc.SetImageTemplateObjectStore(&routeImageTemplateObjectStore{})
	authSvc := auth.NewService(nil, nil, overviewSessionStore{payload: auth.SessionPayload{SessionID: "session-1", UserID: 42}}, "cop_app_session", time.Hour, testRouteCodec{}, nil, nil)

	public, err := svc.CreateImagePromptTemplate(context.Background(), admin.UpsertImagePromptTemplateRequest{
		Slug:         "poster",
		Title:        "Poster",
		Description:  "Poster template",
		PromptPreset: "A {{product}} poster",
		SortOrder:    10,
		Enabled:      true,
		Slots:        []admin.ImagePromptSlot{{Key: "product", Label: "Product", Required: true}},
	})
	require.NoError(t, err)

	router := gin.New()
	registerImageTemplateRoutes(router.Group("/api"), svc, authSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/app/image-templates", bytes.NewReader([]byte(fmt.Sprintf(`{"source_template_id":%d,"slug":"my-poster","title":"My Poster"}`, public.ID))))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "session-1"})
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"visibility":"user_private"`)
	require.Contains(t, rec.Body.String(), `"owner_user_id":42`)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/app/image-templates", nil)
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "session-1"})
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"public":[`)
	require.Contains(t, rec.Body.String(), `"private":[`)
	require.Contains(t, rec.Body.String(), `"title":"My Poster"`)
	require.Contains(t, rec.Body.String(), `"prompt_preset":"A {{product}} poster"`)
}
