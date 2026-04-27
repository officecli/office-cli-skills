package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/officecli/officecli/engine"
)

type renderOnlyLLMClient struct {
	imageCalls     int
	completeJSON   int
	completeStruct int
}

func (c *renderOnlyLLMClient) CompleteText(context.Context, []engine.LLMMessage) (string, error) {
	return "", nil
}

func (c *renderOnlyLLMClient) CompleteJSON(context.Context, []engine.LLMMessage) (string, error) {
	c.completeJSON++
	return "", nil
}

func (c *renderOnlyLLMClient) CompleteStructured(context.Context, engine.StructuredCompletionRequest) (string, error) {
	c.completeStruct++
	return "", nil
}

func (c *renderOnlyLLMClient) GenerateImage(context.Context, engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	c.imageCalls++
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+yF9sAAAAASUVORK5CYII=")
	if err != nil {
		return nil, err
	}
	return &engine.ImageGenerationResult{Data: data, MIME: "image/png"}, nil
}

func TestExecuteRenderJobSkipsLLMInitForDOCX(t *testing.T) {
	app := NewApp(nil, nil, nil)
	llmInitCalls := 0
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		llmInitCalls++
		return &renderOnlyLLMClient{}, nil
	}
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}

	cfg := Config{
		Defaults: DefaultsConfig{OutputDir: t.TempDir(), Publish: false, Mode: "fast"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}
	job := GenerateJob{
		DocumentType: engine.DocumentTypeDOCX,
		Topic:        "Quarterly Brief",
		Prompt:       "Quarterly Brief",
		OutputDir:    cfg.Defaults.OutputDir,
		RuntimeMode:  RuntimeModeExternal,
		JSONOutput:   true,
	}
	payload := json.RawMessage(`{"title":"Quarterly Brief","sections":[{"heading":"Summary","level":1,"paragraphs":["Delivery-ready content."]}]}`)

	result, err := app.executeRenderJob(context.Background(), cfg, job, payload, nil)
	if err != nil {
		t.Fatalf("executeRenderJob: %v", err)
	}
	if llmInitCalls != 0 {
		t.Fatalf("expected no llm init, got %d", llmInitCalls)
	}
	if filepath.Ext(result.FilePath) != ".docx" {
		t.Fatalf("unexpected file path: %s", result.FilePath)
	}
}

func TestExecuteRenderJobSkipsTextLLMForPPTXTextOnly(t *testing.T) {
	app := NewApp(nil, nil, nil)
	llmInitCalls := 0
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		llmInitCalls++
		return &renderOnlyLLMClient{}, nil
	}
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}

	cfg := Config{
		Defaults: DefaultsConfig{OutputDir: t.TempDir(), Publish: false, Mode: "fast"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}
	job := GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Platform Overview",
		Prompt:       "Platform Overview",
		OutputDir:    cfg.Defaults.OutputDir,
		RuntimeMode:  RuntimeModeExternal,
		EnableImages: false,
		JSONOutput:   true,
	}
	payload := json.RawMessage(`{"title":"Platform Overview","stylePreset":"editorial-light","theme":null,"slides":[{"title":"Platform Overview","content":"","isTitle":true,"layout":"title","variant":"title-center","narrativeRole":"cover","sectionIndex":0,"sectionTitle":"","subtitle":"What the platform is and why it matters","points":[],"sections":[],"chart":null,"metrics":[],"source":"","bgColor":"","bgColor2":"","hasImage":false,"imagePrompt":"","imagePos":"","visuals":[]}]}`)

	if _, err := app.executeRenderJob(context.Background(), cfg, job, payload, nil); err != nil {
		t.Fatalf("executeRenderJob: %v", err)
	}
	if llmInitCalls != 0 {
		t.Fatalf("expected no llm init, got %d", llmInitCalls)
	}
}

func TestExecuteRenderJobUsesImageProviderWithoutTextCalls(t *testing.T) {
	app := NewApp(nil, nil, nil)
	llmClient := &renderOnlyLLMClient{}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return llmClient, nil
	}
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}

	cfg := Config{
		Defaults: DefaultsConfig{OutputDir: t.TempDir(), Publish: false, Mode: "fast"},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "llm-key", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}
	job := GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Platform Overview",
		Prompt:       "Platform Overview",
		OutputDir:    cfg.Defaults.OutputDir,
		RuntimeMode:  RuntimeModeExternal,
		EnableImages: true,
		JSONOutput:   true,
	}
	payload := json.RawMessage(`{"title":"Platform Overview","stylePreset":"editorial-light","theme":null,"slides":[{"title":"Platform Overview","content":"","isTitle":true,"layout":"title","variant":"title-center","narrativeRole":"cover","sectionIndex":0,"sectionTitle":"","subtitle":"What the platform is and why it matters","points":[],"sections":[],"chart":null,"metrics":[],"source":"","bgColor":"","bgColor2":"","hasImage":true,"imagePrompt":"A clean software dashboard hero visual","imagePos":"background","visuals":[]}]}`)

	if _, err := app.executeRenderJob(context.Background(), cfg, job, payload, nil); err != nil {
		t.Fatalf("executeRenderJob: %v", err)
	}
	if llmClient.imageCalls == 0 {
		t.Fatal("expected image generation call")
	}
	if llmClient.completeJSON != 0 || llmClient.completeStruct != 0 {
		t.Fatalf("unexpected text llm calls: json=%d structured=%d", llmClient.completeJSON, llmClient.completeStruct)
	}
}
