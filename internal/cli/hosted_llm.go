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
		return nil, fmt.Errorf("缺少平台服务地址，当前无法使用平台托管生成")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("缺少平台访问凭证，当前无法使用平台托管生成")
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
	case engine.DocumentTypePPTX:
		if job.EnableImages {
			profile = "pptx-with-image"
		} else {
			profile = "pptx-no-image"
		}
	case engine.DocumentTypeDOCX, engine.DocumentTypeXLSX:
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
	return fmt.Errorf("平台托管生成请求失败：%w", err)
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
