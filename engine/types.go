package engine

import "context"

type DocumentType string

const (
	DocumentTypePPTX   DocumentType = "pptx"
	DocumentTypeDOCX   DocumentType = "docx"
	DocumentTypeXLSX   DocumentType = "xlsx"
	DocumentTypeReport DocumentType = "report"
	DocumentTypeIMG    DocumentType = "img"
	DocumentTypeGIF    DocumentType = "gif"
)

type GenerateIssue struct {
	Code       string
	Message    string
	Field      string
	ReasonCode string
	Retryable  bool
}

type SDKParams struct {
	FileID      string
	Token       string
	Endpoint    string
	QueryToken  string
	FileVersion uint32
}

type GeneratedDocument struct {
	RequestID   string
	Status      string
	Mode        string
	Name        string
	Type        string
	DownloadURL string
	AccessURL   string
	FileID      string
	SDKParams   *SDKParams
	StorageKey  string
	Warnings    []GenerateIssue
	Errors      []GenerateIssue
}

type Document struct {
	ID             string
	ConversationID string
	DocumentType   string
	Status         string
	Version        int
	ArtifactKey    string
	FileID         string
	Name           string
	Warnings       []GenerateIssue
}

type GenerateDocumentRequest struct {
	Description    string
	DocumentType   DocumentType
	ExistingFileID string
	BaseURL        string
}

type ModifyDocumentRequest struct {
	DocumentID     string
	Intent         string
	Description    string
	BaseVersion    int
	RequestID      string
	TargetMetadata TargetMetadata
}

type ClassifyIntentRequest struct {
	DocumentID       string
	UserPrompt       string
	EditTarget       string
	SelectionContext *SelectionContext
}

type PrepareExecutionPlanRequest struct {
	ConversationID   string
	DocumentID       string
	UserPrompt       string
	DocumentType     string
	RequestID        string
	GenerationMode   string
	EditTarget       string
	SelectionContext *SelectionContext
}

type AnswerExecutionPlanQuestionRequest struct {
	PlanID     string
	QuestionID string
	OptionID   string
	Answer     string
	RequestID  string
}

type ReviseExecutionPlanRequest struct {
	PlanID      string
	Instruction string
	RequestID   string
}

type ApproveExecutionPlanRequest struct {
	PlanID    string
	RequestID string
}

type SelectionContext struct {
	SelectionText    string
	HasTextSelection bool
	SlideIndex       int
	ShapeType        string
	ShapeID          string
	RangeRef         string
	WorksheetIndex   int
}

type TargetMetadata struct {
	Scope          string
	SlideIndex     int
	ElementType    string
	TableRow       int
	TableCol       int
	ParagraphIndex int
	WorksheetIndex int
	WorksheetName  string
	RangeRef       string
}

type PlanQuestionOption struct {
	ID          string
	Label       string
	Description string
	Recommended bool
}

type PlanQuestion struct {
	ID            string
	Question      string
	Options       []PlanQuestionOption
	AllowFreeform bool
}

type PlanAnswer struct {
	QuestionID string
	OptionID   string
	Answer     string
	Source     string
}

type PlanSession struct {
	PlanID                   string
	ConversationID           string
	DocumentID               string
	DocumentType             string
	EditTarget               string
	GenerationMode           string
	IntentType               string
	Status                   string
	UserPrompt               string
	Questions                []PlanQuestion
	CurrentQuestion          *PlanQuestion
	QuestionSource           string
	QuestionErrorKind        string
	QuestionFallbackReason   string
	QuestionValidationRule   string
	QuestionValidationDetail string
	QuestionRawPreview       string
	QuestionLLMProvider      string
	QuestionLLMModel         string
	QuestionLLMBaseURL       string
	Answers                  []PlanAnswer
	ExecutionPrompt          string
	PlanMarkdown             string
	FrameworkBlueprint       string
	Revision                 int
}

type ClassifyIntentResult struct {
	Action               string
	ModifyIntent         string
	EditTarget           string
	TargetMetadata       TargetMetadata
	Confidence           float64
	Reason               string
	OfficeOperationCount int
	OfficePlanSource     string
}

type GenerateDocumentService interface {
	GenerateDocument(ctx context.Context, req GenerateDocumentRequest) (*GeneratedDocument, error)
}

type DocumentWorkflowService interface {
	GenerateDocumentService
	ModifyDocument(ctx context.Context, req ModifyDocumentRequest) (*Document, error)
	ClassifyIntent(ctx context.Context, req ClassifyIntentRequest) (*ClassifyIntentResult, error)
	PrepareExecutionPlan(ctx context.Context, req PrepareExecutionPlanRequest) (*PlanSession, error)
	AnswerExecutionPlanQuestion(ctx context.Context, req AnswerExecutionPlanQuestionRequest) (*PlanSession, error)
	ReviseExecutionPlan(ctx context.Context, req ReviseExecutionPlanRequest) (*PlanSession, error)
	ApproveExecutionPlan(ctx context.Context, req ApproveExecutionPlanRequest) (*PlanSession, error)
}

type DocumentEngine interface {
	DocumentWorkflowService
}
