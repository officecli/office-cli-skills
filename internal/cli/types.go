package cli

import (
	"context"
	"io"

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
	if cfg.Runtime.Mode != "" {
		return cfg.Runtime.Mode
	}
	return RuntimeModeExternal
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
	DocumentType engine.DocumentType
	Topic        string
	Brief        string
	Prompt       string
	RuntimeMode  RuntimeMode
	Mode         string
	Language     string
	Style        string
	Audience     string
	EnableImages bool
	LocalPreview bool
	OutputDir    string
	Publish      bool
	JSONOutput   bool
	LicenseCheck *LicenseCheckResult
}

type GenerateResult struct {
	Status                 string   `json:"status"`
	FilePath               string   `json:"file_path"`
	DocumentType           string   `json:"document_type"`
	DocumentName           string   `json:"document_name"`
	Published              bool     `json:"published"`
	AccessURL              string   `json:"access_url,omitempty"`
	Password               string   `json:"password,omitempty"`
	ExpiresAt              string   `json:"expires_at,omitempty"`
	Warnings               []string `json:"warnings,omitempty"`
	LocalPreviewPath       string   `json:"local_preview_path,omitempty"`
	LocalPreviewDataPath   string   `json:"local_preview_data_path,omitempty"`
	AccessMode             string   `json:"access_mode,omitempty"`
	RuntimeMode            string   `json:"runtime_mode,omitempty"`
	AllowedModes           []string `json:"allowed_modes,omitempty"`
	HostedEnabled          bool     `json:"hosted_enabled,omitempty"`
	CreditBalance          int      `json:"credit_balance,omitempty"`
	FreeRemaining          int      `json:"free_remaining,omitempty"`
	RewardRemaining        int      `json:"reward_remaining,omitempty"`
	PaidQuotaRemaining     int      `json:"paid_quota_remaining,omitempty"`
	Remaining              int      `json:"remaining,omitempty"`
	ConfigPath             string   `json:"config_path,omitempty"`
	LicenseEnabled         bool     `json:"license_enabled"`
	PublishedSkippedReason string   `json:"published_skipped_reason,omitempty"`
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
