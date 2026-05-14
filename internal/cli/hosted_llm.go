package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/officecli/officecli-internal/engine"
	llmprovider "github.com/officecli/officecli-internal/internal/providers/llm"
)

const hostedGenerationTimeoutSec = 1200

func newHostedLLMClient(cfg LicenseConfig, job GenerateJob) (GeneratorLLMClient, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("platform service URL is missing, so hosted generation is unavailable")
	}
	textModelName := hostedTextModelName(job)
	imageModelName := hostedImageModelName(job)
	var imageAccess *llmprovider.InternalImageAccess
	if job.LicenseCheck != nil && job.LicenseCheck.AccessMode != LicenseAccessModeHosted {
		tokenBytes, err := json.Marshal(job.LicenseCheck.CommitToken)
		if err != nil {
			return nil, fmt.Errorf("marshal hosted access token: %w", err)
		}
		imageAccess = &llmprovider.InternalImageAccess{
			FingerprintHash: job.LicenseCheck.CommitToken.FingerprintHash,
			UserID:          job.LicenseCheck.CommitToken.UserID,
			APIKey:          strings.TrimSpace(cfg.APIKey),
			AccessMode:      string(job.LicenseCheck.AccessMode),
			CommitToken:     json.RawMessage(tokenBytes),
		}
	}
	return llmprovider.NewClient(llmprovider.Config{
		Provider:    "internal",
		BaseURL:     baseURL + "/api/llm",
		APIKey:      strings.TrimSpace(cfg.APIKey),
		Model:       textModelName,
		ImageModel:  imageModelName,
		TimeoutSec:  hostedTimeoutSec(cfg.TimeoutSec, job),
		ImageAccess: imageAccess,
	})
}

func hostedTimeoutSec(configured int, job GenerateJob) int {
	if configured < hostedGenerationTimeoutSec {
		return hostedGenerationTimeoutSec
	}
	return configured
}

func newHostedImageLLMClient(cfg LicenseConfig) (GeneratorLLMClient, error) {
	return newHostedLLMClient(cfg, GenerateJob{
		DocumentType: engine.DocumentTypeIMG,
		RuntimeMode:  RuntimeModeHosted,
		Mode:         "fast",
	})
}

func hostedTextModelName(job GenerateJob) string {
	if job.DocumentType == engine.DocumentTypeIMG {
		return "hosted/image"
	}
	return "hosted/text"
}

func hostedImageModelName(_ GenerateJob) string {
	return "hosted/image"
}

func (cfg Config) hostedRuntimeMode() RuntimeMode {
	return cfg.RuntimeModeOrDefault()
}

func runtimeModeLabel(mode RuntimeMode) string {
	if mode == "" {
		return string(RuntimeModeHosted)
	}
	return string(mode)
}

func buildHostedModeError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("hosted generation request failed: %w", err)
}

func shouldSkipLocalLLM(mode RuntimeMode) bool {
	return mode == RuntimeModeHosted
}

func resultRuntimeMode(job GenerateJob, result *LicenseCheckResult) string {
	if result != nil && strings.TrimSpace(result.SelectedRuntimeMode) != "" {
		return result.SelectedRuntimeMode
	}
	return runtimeModeLabel(job.RuntimeMode)
}

func hostedContext(ctx context.Context) context.Context {
	return ctx
}
