package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/officecli/officecli/engine"
	llmprovider "github.com/officecli/officecli/internal/providers/llm"
)

func newHostedLLMClient(cfg LicenseConfig, job GenerateJob) (GeneratorLLMClient, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("platform service URL is missing, so hosted generation is unavailable")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("platform access token is missing, so hosted generation is unavailable")
	}
	modelName := hostedModelName(job)
	return llmprovider.NewClient(llmprovider.Config{
		Provider:   "internal",
		BaseURL:    baseURL + "/api/llm",
		APIKey:     strings.TrimSpace(cfg.APIKey),
		Model:      modelName,
		ImageModel: modelName,
		TimeoutSec: cfg.TimeoutSec,
	})
}

func hostedModelName(job GenerateJob) string {
	profile := "docx-xlsx"
	switch job.DocumentType {
	case engine.DocumentTypeIMG:
		profile = "img"
	case engine.DocumentTypePPTX:
		if job.EnableImages {
			profile = "pptx-with-image"
		} else {
			profile = "pptx-no-image"
		}
	case engine.DocumentTypeDOCX, engine.DocumentTypeXLSX, engine.DocumentTypeReport:
		profile = "docx-xlsx"
	}
	return "hosted/" + profile
}

func (cfg Config) hostedRuntimeMode() RuntimeMode {
	return cfg.RuntimeModeOrDefault()
}

func runtimeModeLabel(mode RuntimeMode) string {
	if mode == "" {
		return string(RuntimeModeExternal)
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
