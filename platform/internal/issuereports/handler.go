package issuereports

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/officecli/officecli/platform/internal/httpapi"
	"github.com/officecli/officecli/platform/internal/model"
)

// CLISessionResolver resolves a bearer token to a CLI session.
type CLISessionResolver interface {
	Resolve(ctx context.Context, token string) (*model.CLISession, error)
}

// Handler exposes HTTP endpoints for issue-report submission.
type Handler struct {
	svc        *Service
	cliSession CLISessionResolver
}

// NewHandler creates a new issue-reports HTTP handler.
func NewHandler(svc *Service, cli CLISessionResolver) *Handler {
	return &Handler{svc: svc, cliSession: cli}
}

// submitRequest matches the desktop §3 payload schema (camelCase fields).
type submitRequest struct {
	SchemaVersion int    `json:"schema_version"`
	RequestID     string `json:"requestId"`
	TaskID        string `json:"taskId"`
	RuntimeMode   string `json:"runtimeMode"`
	ErrorCode     string `json:"errorCode"`
	ErrorMessage  string `json:"errorMessage"`
	Description   string `json:"description"`
	ContactEmail  string `json:"contactEmail"`
	Timestamp     string `json:"timestamp"`
	Via           string `json:"via"`
}

// Authenticated handles POST /api/issue-reports (requires CLI session auth).
func (h *Handler) Authenticated(c *gin.Context) {
	token := httpapi.BearerToken(c.GetHeader("Authorization"))
	if token == "" {
		httpapi.AbortUnauthorized(c)
		return
	}
	session, err := h.cliSession.Resolve(c.Request.Context(), token)
	if err != nil || session == nil {
		httpapi.AbortUnauthorized(c)
		return
	}

	userID := &session.UserID
	h.handleSubmit(c, userID)
}

// Anonymous handles POST /api/issue-reports/anonymous (no auth required).
func (h *Handler) Anonymous(c *gin.Context) {
	h.handleSubmit(c, nil)
}

func (h *Handler) handleSubmit(c *gin.Context, userID *uint64) {
	// Limit body size.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxBodyBytes)

	var req submitRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			httpapi.Error(c, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		httpapi.Error(c, http.StatusBadRequest, "invalid JSON body")
		return
	}

	result, err := h.svc.Submit(c.Request.Context(), SubmitInput{
		UserID:        userID,
		RuntimeMode:   req.RuntimeMode,
		SchemaVersion: req.SchemaVersion,
		RequestID:     req.RequestID,
		TaskID:        req.TaskID,
		ErrorCode:     req.ErrorCode,
		ErrorMessage:  req.ErrorMessage,
		Description:   req.Description,
		ContactEmail:  req.ContactEmail,
		Timestamp:     req.Timestamp,
		Via:           req.Via,
		ClientIP:      c.ClientIP(),
		UserAgent:     c.GetHeader("User-Agent"),
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrUnsupportedSchema):
			c.JSON(http.StatusBadRequest, gin.H{
				"error":      "unsupported_schema",
				"minVersion": MinSchemaVersion,
				"maxVersion": MaxSchemaVersion,
				"request_id": httpapi.RequestID(c),
			})
		case errors.Is(err, ErrDescriptionRequired):
			httpapi.Error(c, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrContactEmailInvalid):
			httpapi.Error(c, http.StatusBadRequest, err.Error())
		default:
			httpapi.Error(c, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httpapi.JSON(c, http.StatusOK, gin.H{
		"ticketId":       result.TicketID,
		"schema_version": result.SchemaVersion,
		"accepted_at":    result.AcceptedAt.Format("2006-01-02T15:04:05Z"),
	})
}
