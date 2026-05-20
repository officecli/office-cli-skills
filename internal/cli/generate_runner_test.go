package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/officecli/officecli-internal/engine"
)

func TestExecuteGenerateJob_HostedAnonymousSendsCommitTokenToTextLLM(t *testing.T) {
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/llm/v1/json" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("anonymous hosted request should not send Authorization, got %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"content":"{\"title\":\"Anonymous Trial\",\"sections\":[{\"heading\":\"Summary\",\"level\":1,\"paragraphs\":[\"Hosted anonymous generation works.\"]}]}"}`)
	}))
	defer server.Close()

	var actions []string
	consumes := 0
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return countingLicenseManager{
			check: func(req LicenseCheckRequest) (*LicenseCheckResult, error) {
				actions = append(actions, req.Action)
				return &LicenseCheckResult{
					Allowed:       true,
					AccessMode:    LicenseAccessModeFree,
					FreeRemaining: 10,
					CommitToken:   signTestCommitToken(req, LicenseAccessModeFree, UsageCommitToken{}),
				}, nil
			},
			consume: func(token UsageCommitToken) (*UsageConsumeResult, error) {
				consumes++
				return &UsageConsumeResult{AccessMode: LicenseAccessModeFree, Remaining: 9, FreeRemaining: 9}, nil
			},
		}, nil
	}

	outputDir := t.TempDir()
	result, err := app.executeGenerateJob(context.Background(), Config{
		Defaults: DefaultsConfig{OutputDir: outputDir, Publish: false, Mode: "fast"},
		License:  LicenseConfig{BaseURL: server.URL, Enabled: true, TimeoutSec: 5},
		Publish:  disabledPublishConfig(),
	}, GenerateJob{
		DocumentType: engine.DocumentTypeDOCX,
		Topic:        "Anonymous Trial",
		Prompt:       "Write a short status note",
		RuntimeMode:  RuntimeModeHosted,
		Mode:         "fast",
		OutputDir:    outputDir,
	}, false, noopProgressController{}, nil)

	if err != nil {
		t.Fatalf("executeGenerateJob: %v", err)
	}
	if result.AccessMode != "free" || result.FreeRemaining != 9 {
		t.Fatalf("result = %+v", result)
	}
	if !reflect.DeepEqual(actions, []string{"generate"}) {
		t.Fatalf("license actions = %#v", actions)
	}
	if consumes != 1 {
		t.Fatalf("license consumes = %d", consumes)
	}
	if gotPayload["commit_token"] == nil || gotPayload["access_mode"] != "free" || gotPayload["fingerprint_hash"] == "" {
		t.Fatalf("anonymous access payload = %#v", gotPayload)
	}
}

func TestExecuteGenerateJob_HostedAccountSendsOnlyFingerprintToTextLLM(t *testing.T) {
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/llm/v1/json" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer hosted-key" {
			t.Fatalf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"content":"{\"title\":\"Account Hosted\",\"sections\":[{\"heading\":\"Summary\",\"level\":1,\"paragraphs\":[\"Account hosted generation works.\"]}]}"}`)
	}))
	defer server.Close()

	var checkedRuntime RuntimeMode
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return countingLicenseManager{
			check: func(req LicenseCheckRequest) (*LicenseCheckResult, error) {
				checkedRuntime = RuntimeMode(req.RuntimeMode)
				return &LicenseCheckResult{
					Allowed:       true,
					AccessMode:    LicenseAccessModeHosted,
					HostedEnabled: true,
					CreditBalance: 18,
					CommitToken:   signTestCommitToken(req, LicenseAccessModeHosted, UsageCommitToken{}),
				}, nil
			},
			consume: func(token UsageCommitToken) (*UsageConsumeResult, error) {
				t.Fatalf("account hosted text generation should not consume external quota token: %+v", token)
				return nil, nil
			},
		}, nil
	}

	outputDir := t.TempDir()
	result, err := app.executeGenerateJob(context.Background(), Config{
		Defaults: DefaultsConfig{OutputDir: outputDir, Publish: false, Mode: "fast"},
		License:  LicenseConfig{BaseURL: server.URL, APIKey: "hosted-key", Enabled: true, TimeoutSec: 5},
		Publish:  disabledPublishConfig(),
	}, GenerateJob{
		DocumentType: engine.DocumentTypeDOCX,
		Topic:        "Account Hosted",
		Prompt:       "Write a short status note",
		RuntimeMode:  RuntimeModeHosted,
		Mode:         "fast",
		OutputDir:    outputDir,
	}, false, noopProgressController{}, nil)

	if err != nil {
		t.Fatalf("executeGenerateJob: %v", err)
	}
	if checkedRuntime != RuntimeModeHosted {
		t.Fatalf("license runtime = %q, want hosted", checkedRuntime)
	}
	if gotPayload["fingerprint_hash"] == "" {
		t.Fatalf("fingerprint_hash missing from account hosted payload: %#v", gotPayload)
	}
	for _, key := range []string{"commit_token", "access_mode", "api_key"} {
		if _, ok := gotPayload[key]; ok {
			t.Fatalf("account hosted payload must not include %s: %#v", key, gotPayload)
		}
	}
	if result.AccessMode != string(LicenseAccessModeHosted) || !result.HostedEnabled {
		t.Fatalf("result = %+v", result)
	}
}

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

func TestExecuteGenerateJob_IMGChecksGenerationAccessBeforeImageRequest(t *testing.T) {
	imageCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		imageCalls++
		if r.URL.Path != "/api/llm/v1/image" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		http.Error(w, `{"error":"image quota check should stop before this request"}`, http.StatusBadRequest)
	}))
	defer server.Close()

	app := NewApp(nil, nil, nil)
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkErr: fmt.Errorf("the current key has no remaining image generations")}, nil
	}

	_, err := app.executeGenerateJob(context.Background(), Config{
		Defaults: DefaultsConfig{OutputDir: t.TempDir(), Publish: false, Mode: "fast"},
		License:  LicenseConfig{BaseURL: server.URL, APIKey: "paid-key", Enabled: true, TimeoutSec: 5},
		Publish:  disabledPublishConfig(),
	}, GenerateJob{
		DocumentType: engine.DocumentTypeIMG,
		Topic:        "Launch Visual",
		Prompt:       "A polished product launch hero image",
		RuntimeMode:  RuntimeModeHosted,
		Mode:         "fast",
		OutputDir:    t.TempDir(),
		ImageRatio:   "square",
	}, false, noopProgressController{}, nil)
	if err == nil {
		t.Fatal("expected hosted access error")
	}
	if !strings.Contains(err.Error(), "the current key has no remaining image generations") {
		t.Fatalf("unexpected error: %v", err)
	}
	if imageCalls != 0 {
		t.Fatalf("image requests = %d, want 0", imageCalls)
	}
}

func TestExecuteGenerateJob_IMGExternalUsesLocalImageProvider(t *testing.T) {
	imageBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+yF9sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode image: %v", err)
	}
	imageCalls := 0
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return localImageLLMClient{
			onImage: func(req engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
				imageCalls++
				if req.Prompt == "" {
					t.Fatalf("image prompt is empty")
				}
				return &engine.ImageGenerationResult{Data: imageBytes, MIME: "image/png"}, nil
			},
		}, nil
	}
	licenseChecks := 0
	licenseConsumes := 0
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return countingLicenseManager{
			check: func(req LicenseCheckRequest) (*LicenseCheckResult, error) {
				licenseChecks++
				if RuntimeMode(req.RuntimeMode) != RuntimeModeExternal {
					t.Fatalf("runtime mode = %q", req.RuntimeMode)
				}
				return &LicenseCheckResult{
					Allowed:     true,
					AccessMode:  LicenseAccessModeFree,
					CommitToken: signTestCommitToken(req, LicenseAccessModeFree, UsageCommitToken{}),
				}, nil
			},
			consume: func(token UsageCommitToken) (*UsageConsumeResult, error) {
				licenseConsumes++
				return &UsageConsumeResult{AccessMode: LicenseAccessModeFree}, nil
			},
		}, nil
	}

	tmpDir := t.TempDir()
	result, err := app.executeGenerateJob(context.Background(), Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false, Mode: "fast"},
		LLM:      LLMConfig{BaseURL: "https://llm.example.com/v1", APIKey: "image-key", Model: "gpt-4.1", ImageModel: "gpt-image-1"},
		License:  LicenseConfig{BaseURL: "https://platform.example.com/api", APIKey: "optional-key", Enabled: true, TimeoutSec: 5},
		Publish:  disabledPublishConfig(),
	}, GenerateJob{
		DocumentType: engine.DocumentTypeIMG,
		Topic:        "Launch Visual",
		Prompt:       "A polished product launch hero image",
		RuntimeMode:  RuntimeModeExternal,
		Mode:         "fast",
		OutputDir:    tmpDir,
		ImageRatio:   "landscape",
	}, false, noopProgressController{}, nil)

	if err != nil {
		t.Fatalf("executeGenerateJob: %v", err)
	}
	if imageCalls != 1 {
		t.Fatalf("image calls = %d", imageCalls)
	}
	if licenseChecks != 1 || licenseConsumes != 1 {
		t.Fatalf("license checks=%d consumes=%d", licenseChecks, licenseConsumes)
	}
	data, err := os.ReadFile(result.FilePath)
	if err != nil {
		t.Fatalf("read image: %v", err)
	}
	if string(data) != string(imageBytes) {
		t.Fatalf("image bytes = %q", string(data))
	}
	if result.RuntimeMode != string(RuntimeModeExternal) {
		t.Fatalf("runtime mode = %q", result.RuntimeMode)
	}
}

type localImageLLMClient struct {
	onImage func(engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error)
}

func (localImageLLMClient) CompleteText(context.Context, []engine.LLMMessage) (string, error) {
	return "", nil
}

func (localImageLLMClient) CompleteJSON(context.Context, []engine.LLMMessage) (string, error) {
	return "", nil
}

func (localImageLLMClient) CompleteStructured(context.Context, engine.StructuredCompletionRequest) (string, error) {
	return "", nil
}

func (c localImageLLMClient) GenerateImage(_ context.Context, req engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	if c.onImage == nil {
		return nil, fmt.Errorf("unexpected image request")
	}
	return c.onImage(req)
}

type countingLicenseManager struct {
	check   func(LicenseCheckRequest) (*LicenseCheckResult, error)
	consume func(UsageCommitToken) (*UsageConsumeResult, error)
}

func (m countingLicenseManager) Check(_ context.Context, req LicenseCheckRequest) (*LicenseCheckResult, error) {
	if m.check == nil {
		return nil, fmt.Errorf("unexpected check")
	}
	return m.check(req)
}

func (m countingLicenseManager) Consume(_ context.Context, token UsageCommitToken) (*UsageConsumeResult, error) {
	if m.consume == nil {
		return nil, nil
	}
	return m.consume(token)
}
