package issuereports

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/officecli/officecli/platform/internal/model"
)

// --- mocks ---

type mockStore struct {
	createFn func(ctx context.Context, r *model.IssueReport, requestIDs []string) (*model.IssueReport, bool, error)
	findFn   func(ctx context.Context, clientEventID string) (*model.IssueReport, error)
}

func (m *mockStore) CreateIssueReport(ctx context.Context, r *model.IssueReport, requestIDs []string) (*model.IssueReport, bool, error) {
	if m.createFn != nil {
		return m.createFn(ctx, r, requestIDs)
	}
	r.ID = 1
	return r, false, nil
}

func (m *mockStore) FindIssueReportByClientEventID(ctx context.Context, clientEventID string) (*model.IssueReport, error) {
	if m.findFn != nil {
		return m.findFn(ctx, clientEventID)
	}
	return nil, nil
}

type mockCLISession struct {
	resolveFn func(ctx context.Context, token string) (*model.CLISession, error)
}

func (m *mockCLISession) Resolve(ctx context.Context, token string) (*model.CLISession, error) {
	if m.resolveFn != nil {
		return m.resolveFn(ctx, token)
	}
	return nil, nil
}

// --- helpers ---

func handlerTestNow() time.Time {
	return time.Date(2025, 5, 24, 12, 0, 0, 0, time.UTC)
}

func newTestHandler(store *mockStore, cliSession *mockCLISession) *Handler {
	metrics := NewMetrics(nil)
	svc := NewService(store, metrics, handlerTestNow)
	return NewHandler(svc, cliSession)
}

func setupRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/issue-reports", h.Authenticated)
	r.POST("/api/issue-reports/anonymous", h.Anonymous)
	return r
}

func validPayload() submitRequest {
	return submitRequest{
		SchemaVersion: 1,
		RequestID:     "req-123",
		TaskID:        "task-456",
		RuntimeMode:   "hosted",
		ErrorCode:     "ERR_001",
		ErrorMessage:  "something broke",
		Description:   "This is a valid description with enough characters",
		ContactEmail:  "user@example.com",
		Timestamp:     handlerTestNow().Format(time.RFC3339),
		Via:           "http",
	}
}

func doRequest(router *gin.Engine, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	var bodyReader *bytes.Buffer
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewBuffer(data)
	} else {
		bodyReader = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// --- tests ---

func TestAuthenticatedHappyPath(t *testing.T) {
	store := &mockStore{}
	userID := uint64(42)
	cliSession := &mockCLISession{
		resolveFn: func(_ context.Context, token string) (*model.CLISession, error) {
			require.Equal(t, "valid-token", token)
			return &model.CLISession{ID: 1, UserID: userID}, nil
		},
	}

	var capturedReport *model.IssueReport
	store.createFn = func(_ context.Context, r *model.IssueReport, requestIDs []string) (*model.IssueReport, bool, error) {
		capturedReport = r
		r.ID = 1
		return r, false, nil
	}

	h := newTestHandler(store, cliSession)
	router := setupRouter(h)

	rec := doRequest(router, http.MethodPost, "/api/issue-reports", validPayload(), map[string]string{
		"Authorization": "Bearer valid-token",
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	require.NotEmpty(t, data["ticketId"])
	require.Equal(t, float64(1), data["schema_version"])
	require.NotEmpty(t, data["accepted_at"])

	// Verify the report was created with correct user ID.
	require.NotNil(t, capturedReport)
	require.NotNil(t, capturedReport.UserID)
	require.Equal(t, userID, *capturedReport.UserID)
}

func TestIdempotencyReplay(t *testing.T) {
	store := &mockStore{}
	userID := uint64(42)
	cliSession := &mockCLISession{
		resolveFn: func(_ context.Context, _ string) (*model.CLISession, error) {
			return &model.CLISession{ID: 1, UserID: userID}, nil
		},
	}

	callCount := 0
	store.createFn = func(_ context.Context, r *model.IssueReport, _ []string) (*model.IssueReport, bool, error) {
		callCount++
		if callCount == 1 {
			r.ID = 1
			return r, false, nil
		}
		// Second call: simulate replay (return original ticket_id, isReplay=true)
		r.ID = 1
		r.TicketID = "original-ticket-id"
		return r, true, nil
	}

	h := newTestHandler(store, cliSession)
	router := setupRouter(h)
	headers := map[string]string{"Authorization": "Bearer valid-token"}
	payload := validPayload()

	// First submission.
	rec1 := doRequest(router, http.MethodPost, "/api/issue-reports", payload, headers)
	require.Equal(t, http.StatusOK, rec1.Code)

	// Second submission with same data (simulates replay).
	rec2 := doRequest(router, http.MethodPost, "/api/issue-reports", payload, headers)
	require.Equal(t, http.StatusOK, rec2.Code)

	var resp2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp2))
	data2 := resp2["data"].(map[string]any)
	require.Equal(t, "original-ticket-id", data2["ticketId"])
}

func TestUnsupportedSchema(t *testing.T) {
	store := &mockStore{}
	cliSession := &mockCLISession{
		resolveFn: func(_ context.Context, _ string) (*model.CLISession, error) {
			return &model.CLISession{ID: 1, UserID: 42}, nil
		},
	}

	h := newTestHandler(store, cliSession)
	router := setupRouter(h)

	payload := validPayload()
	payload.SchemaVersion = 999

	rec := doRequest(router, http.MethodPost, "/api/issue-reports", payload, map[string]string{
		"Authorization": "Bearer valid-token",
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "unsupported_schema")
}

func TestMissingAuth(t *testing.T) {
	store := &mockStore{}
	cliSession := &mockCLISession{
		resolveFn: func(_ context.Context, token string) (*model.CLISession, error) {
			return nil, nil // no session found
		},
	}

	h := newTestHandler(store, cliSession)
	router := setupRouter(h)

	rec := doRequest(router, http.MethodPost, "/api/issue-reports", validPayload(), nil)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAnonymousHappyPath(t *testing.T) {
	store := &mockStore{}
	cliSession := &mockCLISession{}

	var capturedReport *model.IssueReport
	store.createFn = func(_ context.Context, r *model.IssueReport, _ []string) (*model.IssueReport, bool, error) {
		capturedReport = r
		r.ID = 1
		return r, false, nil
	}

	h := newTestHandler(store, cliSession)
	router := setupRouter(h)

	rec := doRequest(router, http.MethodPost, "/api/issue-reports/anonymous", validPayload(), nil)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	require.NotEmpty(t, data["ticketId"])

	// Anonymous: userID should be nil.
	require.NotNil(t, capturedReport)
	require.Nil(t, capturedReport.UserID)
}

func TestBodyTooLarge(t *testing.T) {
	store := &mockStore{}
	cliSession := &mockCLISession{
		resolveFn: func(_ context.Context, _ string) (*model.CLISession, error) {
			return &model.CLISession{ID: 1, UserID: 42}, nil
		},
	}

	h := newTestHandler(store, cliSession)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/issue-reports", h.Authenticated)

	// Build a body > 4KB.
	bigBody := `{"schema_version":1,"description":"` + strings.Repeat("x", 5000) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/issue-reports", strings.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// MaxBytesReader triggers 413 or 400.
	require.True(t, rec.Code == http.StatusRequestEntityTooLarge || rec.Code == http.StatusBadRequest,
		"expected 413 or 400, got %d", rec.Code)
}

func TestDescriptionEmpty(t *testing.T) {
	store := &mockStore{}
	cliSession := &mockCLISession{
		resolveFn: func(_ context.Context, _ string) (*model.CLISession, error) {
			return &model.CLISession{ID: 1, UserID: 42}, nil
		},
	}

	h := newTestHandler(store, cliSession)
	router := setupRouter(h)

	payload := validPayload()
	payload.Description = ""

	rec := doRequest(router, http.MethodPost, "/api/issue-reports", payload, map[string]string{
		"Authorization": "Bearer valid-token",
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "description required")
}

func TestContactEmailTooLong(t *testing.T) {
	store := &mockStore{}
	cliSession := &mockCLISession{
		resolveFn: func(_ context.Context, _ string) (*model.CLISession, error) {
			return &model.CLISession{ID: 1, UserID: 42}, nil
		},
	}

	h := newTestHandler(store, cliSession)
	router := setupRouter(h)

	payload := validPayload()
	payload.ContactEmail = strings.Repeat("a", 255) + "@example.com" // > 254 chars

	rec := doRequest(router, http.MethodPost, "/api/issue-reports", payload, map[string]string{
		"Authorization": "Bearer valid-token",
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "contact_email invalid")
}
