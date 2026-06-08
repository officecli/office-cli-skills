package cli

import (
	"context"
	"io"
	"strings"

	"github.com/officecli/officecli/engine"
	licenseprovider "github.com/officecli/officecli/internal/license"
	publishprovider "github.com/officecli/officecli/internal/providers/publish"
	reviewprovider "github.com/officecli/officecli/internal/review"
	"github.com/officecli/officecli/internal/runtime"
)

type Config struct {
	Defaults DefaultsConfig         `json:"defaults"`
	Runtime  RuntimeConfig          `json:"runtime"`
	LLM      LLMConfig              `json:"llm"`
	License  LicenseConfig          `json:"license"`
	Publish  publishprovider.Config `json:"publish"`
}

type DefaultsConfig struct {
	OutputDir       string `json:"output_dir"`
	Mode            string `json:"mode"`
	Publish         bool   `json:"publish"`
	PPTXStylePreset string `json:"pptx_style_preset,omitempty"`
}

type RuntimeMode string

const (
	RuntimeModeExternal RuntimeMode = "external"
	RuntimeModeHosted   RuntimeMode = "hosted"
)

type RuntimeConfig struct {
	Mode                   RuntimeMode `json:"mode"`
	DefaultDocumentProfile string      `json:"default_document_profile,omitempty"`
}

func (cfg Config) RuntimeModeOrDefault() RuntimeMode {
	if mode, ok := normalizeConfiguredRuntimeMode(cfg.Runtime.Mode, hasGenerationConfig(cfg)); ok {
		return mode
	}
	if hasGenerationConfig(cfg) {
		return RuntimeModeExternal
	}
	return RuntimeModeHosted
}

func normalizeConfiguredRuntimeMode(mode RuntimeMode, hasGeneration bool) (RuntimeMode, bool) {
	normalized := RuntimeMode(strings.ToLower(strings.TrimSpace(string(mode))))
	switch normalized {
	case RuntimeModeExternal, RuntimeModeHosted:
		return normalized, true
	case "custom":
		return RuntimeModeExternal, true
	case "":
		return "", false
	default:
		if hasGeneration {
			return RuntimeModeExternal, true
		}
		return RuntimeModeHosted, true
	}
}

func runtimeModeConfigWarning(mode RuntimeMode) string {
	normalized := RuntimeMode(strings.ToLower(strings.TrimSpace(string(mode))))
	switch normalized {
	case "", RuntimeModeExternal, RuntimeModeHosted:
		return ""
	case "custom":
		return "Warning: config runtime.mode=custom is deprecated and will be treated as external. Run `officecli config set-runtime <external|hosted>` to update the config."
	default:
		return "Warning: config runtime.mode=" + strings.TrimSpace(string(mode)) + " is unsupported and will be ignored. Run `officecli config set-runtime <external|hosted>` to update the config."
	}
}

type LLMConfig struct {
	Provider     string `json:"provider"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
	Model        string `json:"model"`
	ImageBaseURL string `json:"image_base_url,omitempty"`
	ImageAPIKey  string `json:"image_api_key,omitempty"`
	ImageModel   string `json:"image_model"`
	ReviewModel  string `json:"review_model,omitempty"`
	TimeoutSec   int    `json:"timeout_sec"`
}

type InputSources struct {
	Stdin string
	IsTTY bool
	CWD   string
}

type GenerateJob struct {
	DocumentType          engine.DocumentType
	Topic                 string
	Brief                 string
	OriginalPrompt        string
	Prompt                string
	SourceFilePath        string
	RuntimeMode           RuntimeMode
	Mode                  string
	Language              string
	Style                 string
	StyleSpecified        bool
	Audience              string
	EnableImages          bool
	ImageRatio            string
	ImageSize             string
	GifFPS                int
	ImageWatermark        *ImageWatermarkOptions
	PromptTemplateID      string
	ReferenceImageSources []string
	ReferenceImages       []engine.ImageReference
	ReferenceScanEnabled  bool
	ReferenceScanRoot     string
	ReferencePPTXSources  []string
	PPTXBackend           string
	LocalPreview          bool
	OutputDir             string
	Publish               bool
	Debug                 bool
	JSONOutput            bool
	Warnings              []engine.GenerateIssue
	LicenseCheck          *LicenseCheckResult
}

type GenerateResult struct {
	Status                 string                             `json:"status"`
	FilePath               string                             `json:"file_path"`
	DocumentType           string                             `json:"document_type"`
	DocumentName           string                             `json:"document_name"`
	Published              bool                               `json:"published"`
	AccessURL              string                             `json:"access_url,omitempty"`
	Password               string                             `json:"password,omitempty"`
	ExpiresAt              string                             `json:"expires_at,omitempty"`
	Warnings               []string                           `json:"warnings,omitempty"`
	LocalPreviewPath       string                             `json:"local_preview_path,omitempty"`
	LocalPreviewDataPath   string                             `json:"local_preview_data_path,omitempty"`
	AccessMode             string                             `json:"access_mode,omitempty"`
	RuntimeMode            string                             `json:"runtime_mode,omitempty"`
	AllowedModes           []string                           `json:"allowed_modes,omitempty"`
	HostedEnabled          bool                               `json:"hosted_enabled,omitempty"`
	CreditBalance          int                                `json:"credit_balance,omitempty"`
	CreditsCharged         int                                `json:"credits_charged"`
	CreditMode             string                             `json:"credit_mode,omitempty"`
	RewardRemaining        int                                `json:"reward_remaining,omitempty"`
	PaidQuotaRemaining     int                                `json:"paid_quota_remaining,omitempty"`
	Remaining              int                                `json:"remaining,omitempty"`
	ConfigPath             string                             `json:"config_path,omitempty"`
	LicenseEnabled         bool                               `json:"license_enabled"`
	PublishedSkippedReason string                             `json:"published_skipped_reason,omitempty"`
	ReferenceStyle         *runtime.ReferenceStyleMetadata    `json:"reference_style,omitempty"`
	PPTXArtifactDebug      *runtime.PPTXArtifactDebugMetadata `json:"pptx_artifact_debug,omitempty"`
	PPTXReview             *PPTXReviewMetadata                `json:"pptx_review,omitempty"`
	PPTXBackend            string                             `json:"pptx_backend,omitempty"`
	ImageWatermark         *ImageWatermarkResult              `json:"image_watermark,omitempty"`
}

type ImageWatermarkOptions struct {
	Apply           bool `json:"apply"`
	PaidEntitlement bool `json:"paidEntitlement"`
	CanDisable      bool `json:"canDisable"`
}

type ImageWatermarkResult struct {
	Applied         bool `json:"applied"`
	PaidEntitlement bool `json:"paidEntitlement"`
	CanDisable      bool `json:"canDisable"`
}

type PPTXReviewMetadata struct {
	Status         string   `json:"status"`
	OverallScore   int      `json:"overall_score"`
	StructureScore int      `json:"structure_score"`
	Summary        string   `json:"summary,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
}

type ImagePromptSlot struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	Example      string `json:"example,omitempty"`
	DefaultValue string `json:"default_value,omitempty"`
	HelpText     string `json:"help_text,omitempty"`
	Required     bool   `json:"required,omitempty"`
	Multiline    bool   `json:"multiline,omitempty"`
}

type ImagePromptTemplate struct {
	ID           uint64            `json:"id"`
	OwnerUserID  uint64            `json:"owner_user_id,omitempty"`
	Visibility   string            `json:"visibility,omitempty"`
	Slug         string            `json:"slug"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	PromptPreset string            `json:"prompt_preset"`
	ThumbnailURL string            `json:"thumbnail_url,omitempty"`
	SortOrder    int               `json:"sort_order"`
	Enabled      bool              `json:"enabled"`
	Slots        []ImagePromptSlot `json:"slots,omitempty"`
}

type CreateUserImagePromptTemplateRequest struct {
	SourceTemplateID uint64            `json:"source_template_id,omitempty"`
	Slug             string            `json:"slug"`
	Title            string            `json:"title"`
	Description      string            `json:"description"`
	PromptPreset     string            `json:"prompt_preset,omitempty"`
	Slots            []ImagePromptSlot `json:"slots,omitempty"`
	SortOrder        int               `json:"sort_order,omitempty"`
}

type CreateImageTemplatePublishRequest struct {
	PrivateTemplateID uint64 `json:"private_template_id"`
	ProvenanceID      uint64 `json:"provenance_id,omitempty"`
	RequestID         string `json:"request_id,omitempty"`
	SubmitterNote     string `json:"submitter_note,omitempty"`
}

type ImageTemplatePublishRequest struct {
	ID                uint64 `json:"id"`
	PrivateTemplateID uint64 `json:"private_template_id"`
	RequesterUserID   uint64 `json:"requester_user_id"`
	ProvenanceID      uint64 `json:"provenance_id"`
	Status            string `json:"status"`
	SubmitterNote     string `json:"submitter_note,omitempty"`
	PublicTemplateID  uint64 `json:"public_template_id,omitempty"`
	CreatedAt         string `json:"created_at,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

type ReviewJob struct {
	DocumentType string
	FilePath     string
	EnableVisual bool
	FailBelow    int
	JSONOutput   bool
}

type GenerateParams = runtime.GenerateParams
type GeneratedArtifact = runtime.GeneratedArtifact
type GeneratedSidecar = runtime.GeneratedSidecar
type ReviewRequest = reviewprovider.Request
type ReviewResult = reviewprovider.Result
type ReviewIssue = reviewprovider.Issue
type LicenseConfig = licenseprovider.Config
type LicenseCheckRequest = licenseprovider.CheckRequest
type LicenseCheckResult = licenseprovider.CheckResult
type LicenseAccessMode = licenseprovider.AccessMode
type UsageCommitToken = licenseprovider.CommitToken
type UsageConsumeResult = licenseprovider.ConsumeResult
type PublishRequest = publishprovider.PublishRequest
type PublishResult = publishprovider.PublishResult

const (
	LicenseAccessModeDisabled = licenseprovider.AccessModeDisabled
	LicenseAccessModeFree     = licenseprovider.AccessModeFree
	LicenseAccessModeReward   = licenseprovider.AccessModeReward
	LicenseAccessModePaid     = licenseprovider.AccessModePaid
	LicenseAccessModeHosted   = licenseprovider.AccessModeHosted
	LicenseAccessModeBlocked  = licenseprovider.AccessModeBlocked
)

type Generator interface {
	Generate(ctx context.Context, params GenerateParams) (*GeneratedArtifact, error)
}

type GeneratorLLMClient = engine.LLMClient

type Reviewer interface {
	Review(ctx context.Context, req ReviewRequest) (*ReviewResult, error)
}

type LicenseManager interface {
	Check(ctx context.Context, req LicenseCheckRequest) (*LicenseCheckResult, error)
	Consume(ctx context.Context, token UsageCommitToken) (*UsageConsumeResult, error)
}

type Publisher interface {
	Publish(ctx context.Context, req PublishRequest) (*PublishResult, error)
}

type Prompter interface {
	Ask(question string, options []string, allowFreeform bool) (string, string, error)
}

type Output interface {
	Println(w io.Writer, line string) error
}
