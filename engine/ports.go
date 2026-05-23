package engine

import (
	"context"
	"io"
	"time"
)

type Logger interface {
	Debug(msg string, kv ...any)
	Info(msg string, kv ...any)
	Warn(msg string, kv ...any)
	Error(msg string, kv ...any)
}

type ProgressEvent struct {
	Step            string
	Status          string
	RequestID       string
	ConversationID  string
	DocumentID      string
	FileID          string
	NewVersion      int
	DurationMs      int64
	ElapsedMs       int64
	Content         string
	ActiveContent   string
	Mode            string
	ImageSlideCount int
	FailedStep      string
	Error           string
}

type ProgressEmitter interface {
	Emit(ctx context.Context, event ProgressEvent)
}

type LLMMessage struct {
	Role    string
	Content string
}

type StructuredSchema struct {
	Name        string
	Description string
	JSONSchema  []byte
	Strict      bool
}

type StructuredCompletionRequest struct {
	Messages []LLMMessage
	Schema   StructuredSchema
}

type ImageGenerationRequest struct {
	Prompt            string
	TargetAspectRatio float64
	Size              string
	ReferenceImages   []ImageReference
}

type ImageReference struct {
	Filename string `json:"filename,omitempty"`
	MIME     string `json:"mime"`
	Data     string `json:"data"`
}

type ImageGenerationResult struct {
	Data               []byte
	MIME               string
	CreditBalance      *int
	AccessMode         string
	Remaining          int
	RewardRemaining    int
	PaidQuotaRemaining int
}

type LLMClient interface {
	CompleteText(ctx context.Context, messages []LLMMessage) (string, error)
	CompleteJSON(ctx context.Context, messages []LLMMessage) (string, error)
	CompleteStructured(ctx context.Context, req StructuredCompletionRequest) (string, error)
	GenerateImage(ctx context.Context, req ImageGenerationRequest) (*ImageGenerationResult, error)
}

type ArtifactInfo struct {
	StorageKey  string
	ContentType string
	SizeBytes   int64
}

type ArtifactStore interface {
	Put(ctx context.Context, key string, contentType string, body io.Reader, size int64) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error)
}

type DocumentRecord struct {
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

type DocumentStore interface {
	Create(ctx context.Context, doc *DocumentRecord) error
	GetByID(ctx context.Context, id string) (*DocumentRecord, error)
	UpdateStatus(ctx context.Context, id string, status string) error
	UpdateName(ctx context.Context, id string, name string) error
	UpdateArtifactKey(ctx context.Context, id string, artifactKey string) error
	UpdateFileID(ctx context.Context, id string, fileID string) error
	BumpVersion(ctx context.Context, id string, baseVersion int, newArtifactKey string) (int, error)
}

type PlanStore interface {
	Load(ctx context.Context, planID string) (*PlanSession, error)
	Save(ctx context.Context, session *PlanSession, ttl time.Duration) error
}

type OfficeFileMeta struct {
	FileID      string
	Name        string
	DownloadURL string
	StorageKey  string
	Version     uint32
	FromSDK     bool
}

type OfficeBridge interface {
	GetConversationFileID(ctx context.Context, conversationID string) (string, error)
	SetConversationFileID(ctx context.Context, conversationID string, fileID string) error
	RegisterGeneratedFile(ctx context.Context, fileName string, downloadURL string, storageKey string, existingFileID string) (accessURL string, fileID string, params *SDKParams, err error)
	GetFileMeta(ctx context.Context, fileID string) (*OfficeFileMeta, error)
	SetFileMeta(ctx context.Context, meta *OfficeFileMeta) error
	SetPendingContentStorageKey(ctx context.Context, fileID string, storageKey string) error
	GetPendingContentStorageKey(ctx context.Context, fileID string) (string, error)
	DeletePendingContentStorageKey(ctx context.Context, fileID string) error
	SetContentSyncVersion(ctx context.Context, fileID string, version string) error
	GetContentSyncVersion(ctx context.Context, fileID string) (string, error)
	DeleteContentSyncVersion(ctx context.Context, fileID string) error
	SetSDKBackedVersion(ctx context.Context, fileID string, version string) error
	GetSDKBackedVersion(ctx context.Context, fileID string) (string, error)
	DeleteSDKBackedVersion(ctx context.Context, fileID string) error
}

type Clock interface {
	Now() time.Time
}

type IDGenerator interface {
	NewID() string
}
