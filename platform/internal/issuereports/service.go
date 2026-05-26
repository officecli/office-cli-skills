package issuereports

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/officecli/officecli/platform/internal/issuereports/contentfilter"
	"github.com/officecli/officecli/platform/internal/model"
)

const CurrentSchemaVersion = 1
const MaxSchemaVersion = 1
const MinSchemaVersion = 1
const MaxBodyBytes = 4096 // 4KB

var (
	ErrUnsupportedSchema   = errors.New("unsupported_schema")
	ErrDescriptionRequired = errors.New("description required")
	ErrContactEmailInvalid = errors.New("contact_email invalid")
)

// Store defines the persistence interface used by the issue-reports service.
type Store interface {
	CreateIssueReport(ctx context.Context, r *model.IssueReport, requestIDs []string) (*model.IssueReport, bool, error)
	FindIssueReportByClientEventID(ctx context.Context, clientEventID string) (*model.IssueReport, error)
}

// Service orchestrates validation, content filtering, idempotency derivation,
// and persistence for issue reports.
type Service struct {
	store   Store
	metrics *Metrics
	now     func() time.Time
}

// NewService creates a new issue-reports service.
func NewService(store Store, metrics *Metrics, now func() time.Time) *Service {
	return &Service{store: store, metrics: metrics, now: now}
}

// SubmitInput holds the parameters for submitting an issue report.
type SubmitInput struct {
	UserID       *uint64
	RuntimeMode  string // "hosted" | "external"; empty -> "hosted"
	SchemaVersion int
	RequestID    string // desktop payload requestId, may be ""
	TaskID       string // desktop payload taskId, may be ""
	ErrorCode    string
	ErrorMessage string
	Description  string // desktop payload description (10-500 chars) -> summary
	ContactEmail string // may be ""
	Timestamp    string // desktop payload timestamp, RFC3339
	Via          string // desktop payload via, default "http"
	Category     string // default ""
	Severity     string // default ""
	ClientIP     string
	UserAgent    string
}

// SubmitResult holds the result of a successful issue-report submission.
type SubmitResult struct {
	TicketID      string
	SchemaVersion int
	AcceptedAt    time.Time
	IsReplay      bool
}

// Submit validates, filters, and persists a new issue report.
func (s *Service) Submit(ctx context.Context, in SubmitInput) (*SubmitResult, error) {
	// 1. Validate schema_version.
	if in.SchemaVersion < MinSchemaVersion || in.SchemaVersion > MaxSchemaVersion {
		return nil, ErrUnsupportedSchema
	}

	// 2. Description (summary) required, 10-500 chars.
	descLen := utf8.RuneCountInString(in.Description)
	if descLen < 10 || descLen > 500 {
		return nil, ErrDescriptionRequired
	}

	// 3. Content filter.
	filterOut, filterReport, err := contentfilter.Apply(contentfilter.Input{
		Summary:      in.Description,
		Detail:       "",
		Category:     in.Category,
		ErrorMessage: in.ErrorMessage,
		ContactEmail: in.ContactEmail,
	})
	if err != nil {
		if errors.Is(err, contentfilter.ErrContactEmailTooLong) {
			return nil, ErrContactEmailInvalid
		}
		return nil, err
	}
	for _, field := range filterReport.TruncatedFields {
		s.metrics.ContentTruncated(ctx, field)
	}

	// Normalize runtime mode.
	runtimeMode := in.RuntimeMode
	if runtimeMode == "" {
		runtimeMode = "hosted"
	}

	// Normalize via.
	via := in.Via
	if via == "" {
		via = "http"
	}

	// 4. Derive idempotency key.
	now := s.now()
	clientEventID, fallbackEngaged, normalizedClientTsMs, serverNowMs := DeriveClientEventID(DeriveInput{
		UserID:           in.UserID,
		PayloadTimestamp: in.Timestamp,
		PayloadRequestID: in.RequestID,
		Summary:          in.Description,
		Now:              now,
	})
	if fallbackEngaged {
		s.metrics.ClockSkewRejected(ctx, "payload_timestamp_invalid")
	}

	// 5. Assemble IssueReport.
	var contactEmail *string
	if filterOut.ContactEmail != "" {
		contactEmail = &filterOut.ContactEmail
	}

	report := &model.IssueReport{
		TicketID:          uuid.NewString(),
		UserID:            in.UserID,
		ClientEventID:     clientEventID,
		SchemaVersion:     CurrentSchemaVersion,
		RuntimeMode:       runtimeMode,
		Category:          filterOut.Category,
		Severity:          in.Severity,
		Summary:           filterOut.Summary,
		Detail:            filterOut.Detail,
		ContactEmail:      contactEmail,
		TaskID:            in.TaskID,
		ErrorCode:         in.ErrorCode,
		ErrorMessage:      filterOut.ErrorMessage,
		Via:               via,
		ClientMeta:        []byte("{}"),
		ClientTsMs:        normalizedClientTsMs,
		ServerNowMs:       serverNowMs,
		ClockSkewFallback: fallbackEngaged,
		ClientIP:          in.ClientIP,
		UserAgent:         in.UserAgent,
		Status:            "new",
	}

	// 6. Persist.
	var requestIDs []string
	if in.RequestID != "" {
		requestIDs = []string{in.RequestID}
	}
	created, isReplay, err := s.store.CreateIssueReport(ctx, report, requestIDs)
	if err != nil {
		return nil, err
	}

	// 7. Metrics.
	s.metrics.ReceivedTotal(ctx, runtimeMode, in.Category, in.Severity)
	s.metrics.AnonymousObserved(ctx, in.UserID == nil)

	// 8. Return result.
	return &SubmitResult{
		TicketID:      created.TicketID,
		SchemaVersion: CurrentSchemaVersion,
		AcceptedAt:    now,
		IsReplay:      isReplay,
	}, nil
}
