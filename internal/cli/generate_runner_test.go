package cli

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/officecli/officecli/engine"
)

func TestExecuteGenerateJobRefreshesAccessAfterBestModeQuestions(t *testing.T) {
	app := NewApp(nil, nil, nil)
	llm := &fakeBestModeLLMClient{
		structuredResponses: []string{
			`{"questions":[{"id":"audience","question":"Who is the main audience for this deck?","allowFreeform":true,"options":[{"id":"management","label":"Leadership","description":"Emphasize conclusions and judgment.","recommended":true},{"id":"team","label":"Internal team","description":"Emphasize execution detail.","recommended":false}]}]}`,
			`{"plan_markdown":"# Execution Plan\n\n## Summary\n- Conclusion-first for leadership.","execution_prompt":"Generate the PPT in 6 slides or fewer, for leadership, with a conclusion-first structure."}`,
		},
		jsonResponses: []string{
			`{"presentationType":"Overview deck","targetAudience":"Leadership","presentationPurpose":"Explain OfficeCLI","pageCount":6,"contentStyle":"Conclusion-first","visualEffect":"Clean and credible","slideOutline":[{"slideIndex":1,"purpose":"Cover","contentFormat":"paragraph","suggestedLayout":"title","maxItems":1,"contentRequirements":"State the topic and audience","visualSuggestion":"hero"}],"contentGuideline":"Keep one core point per slide"}`,
		},
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return llm, nil
	}

	var actions []string
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return dynamicLicenseManager{
			check: func(req LicenseCheckRequest) (*LicenseCheckResult, error) {
				actions = append(actions, req.Action)
				if req.Action == "generate" {
					return nil, context.DeadlineExceeded
				}
				return &LicenseCheckResult{
					Allowed:     true,
					AccessMode:  LicenseAccessModePaid,
					CommitToken: signTestCommitToken(req, LicenseAccessModePaid, UsageCommitToken{}),
				}, nil
			},
		}, nil
	}

	outputDir := t.TempDir()
	cfg := Config{
		Defaults: DefaultsConfig{OutputDir: outputDir, Publish: false, Mode: "best"},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "llm-key", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}
	job := GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Q3 Board Review",
		Prompt:       "Create a board review deck for OfficeCLI",
		OutputDir:    outputDir,
		RuntimeMode:  RuntimeModeExternal,
		Mode:         "best",
		EnableImages: false,
	}

	_, err := app.executeGenerateJob(context.Background(), cfg, job, true, nil, fixedPrompter{optionID: "1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(actions, []string{"status", "generate"}) {
		t.Fatalf("license actions = %#v", actions)
	}
	if entries, readErr := os.ReadDir(outputDir); readErr != nil {
		t.Fatalf("ReadDir: %v", readErr)
	} else if len(entries) != 0 {
		t.Fatalf("expected no output files, got %d", len(entries))
	}
}
