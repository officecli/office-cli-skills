package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/officecli/officecli/engine"
	licenseprovider "github.com/officecli/officecli/internal/license"
	publishprovider "github.com/officecli/officecli/internal/providers/publish"
)

type stubLicenseManager struct {
	checkResult *LicenseCheckResult
	checkErr    error
}

func TestMain(m *testing.M) {
	_ = os.Setenv(officeTaskPreflightSkipEnv, "1")
	_ = os.Setenv(updateCheckSkipEnv, "1")
	os.Exit(m.Run())
}

type dynamicLicenseManager struct {
	check func(req LicenseCheckRequest) (*LicenseCheckResult, error)
}

func (s stubLicenseManager) Check(_ context.Context, req LicenseCheckRequest) (*LicenseCheckResult, error) {
	if s.checkErr != nil {
		return nil, s.checkErr
	}
	if s.checkResult != nil {
		cloned := *s.checkResult
		if cloned.Allowed {
			cloned.CommitToken = signTestCommitToken(req, cloned.AccessMode, cloned.CommitToken)
		}
		return &cloned, nil
	}
	return &LicenseCheckResult{
		Allowed:     true,
		AccessMode:  LicenseAccessModePaid,
		CommitToken: signTestCommitToken(req, LicenseAccessModePaid, UsageCommitToken{}),
	}, nil
}

func (s stubLicenseManager) Consume(_ context.Context, token UsageCommitToken) (*UsageConsumeResult, error) {
	return &UsageConsumeResult{}, nil
}

func (d dynamicLicenseManager) Check(_ context.Context, req LicenseCheckRequest) (*LicenseCheckResult, error) {
	return d.check(req)
}

func (d dynamicLicenseManager) Consume(_ context.Context, token UsageCommitToken) (*UsageConsumeResult, error) {
	return &UsageConsumeResult{}, nil
}

const testLicenseProofSeed = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA"

func signTestCommitToken(req LicenseCheckRequest, accessMode LicenseAccessMode, existing UsageCommitToken) UsageCommitToken {
	seed, err := base64.RawURLEncoding.DecodeString(testLicenseProofSeed)
	if err != nil {
		panic(err)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	token := existing
	if token.FingerprintHash == "" {
		token.FingerprintHash = req.FingerprintHash
	}
	token.UserID = req.UserID
	if token.RequestID == "" {
		token.RequestID = "req-test-" + strings.ReplaceAll(req.RequestNonce, "-", "")
	}
	token.AccessMode = accessMode
	token.Action = req.Action
	token.DocumentType = req.DocumentType
	token.RuntimeMode = req.RuntimeMode
	token.RequestNonce = req.RequestNonce
	token.ProofVersion = "v1"
	if token.IssuedAt.IsZero() {
		token.IssuedAt = time.Now().UTC()
	}
	if token.ExpiresAt.IsZero() {
		token.ExpiresAt = token.IssuedAt.Add(2 * time.Minute)
	}
	token.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, []byte(testCommitTokenPayload(token))))
	return token
}

func testCommitTokenPayload(token UsageCommitToken) string {
	parts := []string{
		"version=" + strings.TrimSpace(token.ProofVersion),
		"fingerprint_hash=" + strings.TrimSpace(token.FingerprintHash),
		"user_id=" + strconv.FormatUint(token.UserID, 10),
		"request_id=" + strings.TrimSpace(token.RequestID),
		"access_mode=" + strings.TrimSpace(string(token.AccessMode)),
		"api_key_hint=" + strings.TrimSpace(token.APIKeyHint),
		"action=" + strings.TrimSpace(token.Action),
		"document_type=" + strings.TrimSpace(token.DocumentType),
		"runtime_mode=" + strings.TrimSpace(token.RuntimeMode),
		"request_nonce=" + strings.TrimSpace(token.RequestNonce),
		"issued_at=" + token.IssuedAt.UTC().Format(time.RFC3339Nano),
		"expires_at=" + token.ExpiresAt.UTC().Format(time.RFC3339Nano),
	}
	return strings.Join(parts, "\n")
}

type fakeAppLLMClient struct {
	jsonResponse string
	jsonErr      error
	delay        time.Duration
}

func (fakeAppLLMClient) CompleteText(_ context.Context, _ []engine.LLMMessage) (string, error) {
	return "", nil
}

func (f fakeAppLLMClient) CompleteJSON(_ context.Context, _ []engine.LLMMessage) (string, error) {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.jsonErr != nil {
		return "", f.jsonErr
	}
	if f.jsonResponse != "" {
		return f.jsonResponse, nil
	}
	return "", nil
}

func (fakeAppLLMClient) CompleteStructured(_ context.Context, _ engine.StructuredCompletionRequest) (string, error) {
	return "", nil
}

func (fakeAppLLMClient) GenerateImage(_ context.Context, _ engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	return nil, nil
}

type sequencedAppLLMClient struct {
	mu                  sync.Mutex
	structuredResponses []string
	jsonResponses       []string
	structuredCalls     int
	jsonCalls           int
	lastStructuredReq   engine.StructuredCompletionRequest
	lastJSONMsgs        []engine.LLMMessage
}

func (f *sequencedAppLLMClient) CompleteText(_ context.Context, _ []engine.LLMMessage) (string, error) {
	return "", nil
}

func (f *sequencedAppLLMClient) CompleteJSON(_ context.Context, msgs []engine.LLMMessage) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jsonCalls++
	f.lastJSONMsgs = append([]engine.LLMMessage(nil), msgs...)
	if len(f.jsonResponses) == 0 {
		return "", fmt.Errorf("unexpected CompleteJSON call")
	}
	response := f.jsonResponses[0]
	f.jsonResponses = f.jsonResponses[1:]
	return response, nil
}

func (f *sequencedAppLLMClient) CompleteStructured(_ context.Context, req engine.StructuredCompletionRequest) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.structuredCalls++
	f.lastStructuredReq = req
	if len(f.structuredResponses) == 0 {
		return "", fmt.Errorf("unexpected CompleteStructured call")
	}
	response := f.structuredResponses[0]
	f.structuredResponses = f.structuredResponses[1:]
	return response, nil
}

func (f *sequencedAppLLMClient) GenerateImage(_ context.Context, _ engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	return nil, nil
}

type recordingPrompter struct {
	events    *[]string
	optionIDs []string
	answers   []string
}

func (p *recordingPrompter) Ask(_ string, _ []string, _ bool) (string, string, error) {
	if p.events != nil {
		*p.events = append(*p.events, "prompt")
	}
	optionID := "1"
	if len(p.optionIDs) > 0 {
		optionID = p.optionIDs[0]
		p.optionIDs = p.optionIDs[1:]
	}
	answer := ""
	if len(p.answers) > 0 {
		answer = p.answers[0]
		p.answers = p.answers[1:]
	}
	return optionID, answer, nil
}

type orderedLicenseManager struct {
	events      *[]string
	checkResult *LicenseCheckResult
	checkErr    error
}

func (m *orderedLicenseManager) Check(_ context.Context, req LicenseCheckRequest) (*LicenseCheckResult, error) {
	if m.events != nil {
		*m.events = append(*m.events, "check")
	}
	if m.checkErr != nil {
		return nil, m.checkErr
	}
	if m.checkResult != nil {
		cloned := *m.checkResult
		if cloned.Allowed {
			cloned.CommitToken = signTestCommitToken(req, cloned.AccessMode, cloned.CommitToken)
		}
		return &cloned, nil
	}
	return &LicenseCheckResult{
		Allowed:     true,
		AccessMode:  LicenseAccessModePaid,
		CommitToken: signTestCommitToken(req, LicenseAccessModePaid, UsageCommitToken{}),
	}, nil
}

func (m *orderedLicenseManager) Consume(_ context.Context, _ UsageCommitToken) (*UsageConsumeResult, error) {
	if m.events != nil {
		*m.events = append(*m.events, "consume")
	}
	return &UsageConsumeResult{}, nil
}

type noopProgressController struct{}

func (noopProgressController) Emit(context.Context, engine.ProgressEvent) {}
func (noopProgressController) Pause(string)                               {}

func containsWarning(warnings []string, needle string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, needle) {
			return true
		}
	}
	return false
}

type fakeBestModeLLMClient struct {
	structuredResponses []string
	structuredErrors    []error
	jsonResponses       []string
	jsonErrors          []error
	structuredCalls     int
	jsonCalls           int
}

func (fakeBestModeLLMClient) CompleteText(_ context.Context, _ []engine.LLMMessage) (string, error) {
	return "", nil
}

func (f *fakeBestModeLLMClient) CompleteJSON(_ context.Context, _ []engine.LLMMessage) (string, error) {
	f.jsonCalls++
	if len(f.jsonErrors) >= f.jsonCalls && f.jsonErrors[f.jsonCalls-1] != nil {
		return "", f.jsonErrors[f.jsonCalls-1]
	}
	if len(f.jsonResponses) >= f.jsonCalls {
		return f.jsonResponses[f.jsonCalls-1], nil
	}
	return "", fmt.Errorf("missing json response")
}

func (f *fakeBestModeLLMClient) CompleteStructured(_ context.Context, _ engine.StructuredCompletionRequest) (string, error) {
	f.structuredCalls++
	if len(f.structuredErrors) >= f.structuredCalls && f.structuredErrors[f.structuredCalls-1] != nil {
		return "", f.structuredErrors[f.structuredCalls-1]
	}
	if len(f.structuredResponses) >= f.structuredCalls {
		return f.structuredResponses[f.structuredCalls-1], nil
	}
	return "", fmt.Errorf("missing structured response")
}

func (fakeBestModeLLMClient) GenerateImage(_ context.Context, _ engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	return nil, nil
}

type errorPrompter struct {
	err error
}

func (p errorPrompter) Ask(string, []string, bool) (string, string, error) {
	if p.err != nil {
		return "", "", p.err
	}
	return "", "", fmt.Errorf("prompt stopped")
}

type fixedPrompter struct {
	optionID string
	answer   string
}

func (p fixedPrompter) Ask(string, []string, bool) (string, string, error) {
	return p.optionID, p.answer, nil
}
func disabledPublishConfig() publishprovider.Config {
	return publishprovider.Config{Enabled: false}
}

func writeTestPreflightScript(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

type terminalBuffer struct {
	mu      sync.Mutex
	raw     strings.Builder
	line    []rune
	history []string
}

func (b *terminalBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, r := range string(p) {
		b.raw.WriteRune(r)
		switch r {
		case '\r':
			b.line = b.line[:0]
		case '\n':
			b.history = append(b.history, string(b.line))
			b.line = b.line[:0]
		default:
			b.line = append(b.line, r)
		}
	}
	return len(p), nil
}

func (b *terminalBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.raw.String()
}

func (b *terminalBuffer) History() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.history))
	copy(out, b.history)
	return out
}

func (b *terminalBuffer) IsTerminal() bool { return true }

type terminalInputBuffer struct {
	*strings.Reader
}

func (b *terminalInputBuffer) IsTerminal() bool { return true }

func TestRenderProgress_TTYAnimatesAndFinalizesStage(t *testing.T) {
	oldInterval := spinnerFrameInterval
	spinnerFrameInterval = 5 * time.Millisecond
	defer func() { spinnerFrameInterval = oldInterval }()

	out := &terminalBuffer{}
	renderer := NewProgressRenderer(out, false, true)
	renderer.Emit(context.Background(), engine.ProgressEvent{Step: "generate", Status: "running", Content: "Generating document content"})
	time.Sleep(30 * time.Millisecond)
	renderer.Emit(context.Background(), engine.ProgressEvent{Step: "generate", Status: "completed", Content: "Generated document content"})

	raw := out.String()
	if !strings.Contains(raw, "⠋") || !strings.Contains(raw, "⠙") {
		t.Fatalf("expected multiple spinner frames, got %q", raw)
	}
	history := strings.Join(out.History(), "\n")
	if !strings.Contains(history, "✔ Generated document content") {
		t.Fatalf("expected finalized success line, got %q", history)
	}
}

func TestAppRun_NewInvokesInstalledSkillPreflight(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	configPath := filepath.Join(tmpDir, "config.json")
	markerPath := filepath.Join(tmpDir, "preflight-ran")
	t.Setenv("HOME", homeDir)
	t.Setenv("OFFICE_CLI_CONFIG", configPath)
	t.Setenv(officeTaskPreflightSkipEnv, "0")
	writeTestPreflightScript(t, filepath.Join(homeDir, ".codex", "skills", "officecli", "fix-officecli-env.sh"), "#!/usr/bin/env bash\nset -euo pipefail\n: > \""+markerPath+"\"\n")

	_, err := WriteConfig("", Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false, Mode: "fast"},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "llm-key", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return fakeAppLLMClient{jsonResponse: `{"title":"Enterprise Collaboration Platform Overview","sections":[{"heading":"Product Overview","level":1,"paragraphs":["This collaboration platform is designed for enterprise teams."]}]}`}, nil
	}

	if err := app.Run(t.Context(), []string{"new", "docx", "Enterprise Collaboration Platform Overview", "Introduce this enterprise collaboration platform", "--json", "--no-publish"}); err != nil {
		t.Fatalf("Run(new): %v", err)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("expected preflight marker: %v", err)
	}
}

func TestAppRun_NewSuppressesReadyPreflightJSON(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("HOME", homeDir)
	t.Setenv("OFFICE_CLI_CONFIG", configPath)
	t.Setenv(officeTaskPreflightSkipEnv, "0")
	writeTestPreflightScript(t, filepath.Join(homeDir, ".codex", "skills", "officecli", "fix-officecli-env.sh"), "#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s\\n' '{\"status\":\"ready\",\"officecli_found\":true,\"missing_items\":[]}'\n")

	_, err := WriteConfig("", Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false, Mode: "fast"},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "llm-key", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return fakeAppLLMClient{jsonResponse: `{"title":"Enterprise Collaboration Platform Overview","sections":[{"heading":"Product Overview","level":1,"paragraphs":["This collaboration platform is designed for enterprise teams."]}]}`}, nil
	}

	if err := app.Run(t.Context(), []string{"new", "docx", "Enterprise Collaboration Platform Overview", "--json", "--no-publish"}); err != nil {
		t.Fatalf("Run(new): %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json output invalid: %v, output=%s", err, stdout.String())
	}
	if payload["status"] != "success" {
		t.Fatalf("status = %v", payload["status"])
	}
	if strings.Contains(stdout.String(), "officecli_found") || strings.Contains(stdout.String(), `"status":"ready"`) {
		t.Fatalf("expected preflight json to be suppressed, got %s", stdout.String())
	}
}

func TestAppRun_NewPreflightSetsSkipEnvForScriptChildren(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	configPath := filepath.Join(tmpDir, "config.json")
	markerPath := filepath.Join(tmpDir, "skip-env")
	t.Setenv("HOME", homeDir)
	t.Setenv("OFFICE_CLI_CONFIG", configPath)
	t.Setenv(officeTaskPreflightSkipEnv, "0")
	writeTestPreflightScript(t, filepath.Join(homeDir, ".codex", "skills", "officecli", "fix-officecli-env.sh"), "#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s' \"${OFFICECLI_SKIP_SKILL_PREFLIGHT:-}\" > \""+markerPath+"\"\n")

	_, err := WriteConfig("", Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false, Mode: "fast"},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "llm-key", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return fakeAppLLMClient{jsonResponse: `{"title":"Enterprise Collaboration Platform Overview","sections":[{"heading":"Product Overview","level":1,"paragraphs":["This collaboration platform is designed for enterprise teams."]}]}`}, nil
	}

	if err := app.Run(t.Context(), []string{"new", "docx", "Enterprise Collaboration Platform Overview", "--json", "--no-publish"}); err != nil {
		t.Fatalf("Run(new): %v", err)
	}
	raw, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("ReadFile(marker): %v", err)
	}
	if strings.TrimSpace(string(raw)) != "1" {
		t.Fatalf("expected skip env to be set, got %q", string(raw))
	}
}

func TestAppRun_NewNoPublishSetsSkipPublishEnvForPreflight(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	configPath := filepath.Join(tmpDir, "config.json")
	markerPath := filepath.Join(tmpDir, "skip-publish-env")
	t.Setenv("HOME", homeDir)
	t.Setenv("OFFICE_CLI_CONFIG", configPath)
	t.Setenv(officeTaskPreflightSkipEnv, "0")
	writeTestPreflightScript(t, filepath.Join(homeDir, ".codex", "skills", "officecli", "fix-officecli-env.sh"), "#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s' \"${OFFICECLI_SKIP_PUBLISH_SETUP:-}\" > \""+markerPath+"\"\n")

	_, err := WriteConfig("", Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false, Mode: "fast"},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "llm-key", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return fakeAppLLMClient{jsonResponse: `{"title":"Enterprise Collaboration Platform Overview","sections":[{"heading":"Product Overview","level":1,"paragraphs":["This collaboration platform is designed for enterprise teams."]}]}`}, nil
	}

	if err := app.Run(t.Context(), []string{"new", "docx", "Enterprise Collaboration Platform Overview", "--json", "--no-publish"}); err != nil {
		t.Fatalf("Run(new): %v", err)
	}
	raw, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("ReadFile(marker): %v", err)
	}
	if strings.TrimSpace(string(raw)) != "1" {
		t.Fatalf("expected skip publish env to be set, got %q", string(raw))
	}
}

func TestAppRun_NewFailsWhenInstalledSkillPreflightFails(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)

	_, err := WriteConfig("", Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false, Mode: "fast"},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "llm-key", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.officeTaskPreflight = func(ctx context.Context, command string, args []string) error {
		return fmt.Errorf("boom")
	}
	err = app.Run(t.Context(), []string{"new", "docx", "Enterprise Collaboration Platform Overview", "--json", "--no-publish"})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected preflight failure, got %v", err)
	}
}

func TestAppRun_NewSurfacesLLMRequestFailureBody(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)

	_, err := WriteConfig("", Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false, Mode: "fast"},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "llm-key", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return fakeAppLLMClient{jsonErr: fmt.Errorf("llm request failed: invalid json response body=<html><body>bad gateway</body></html>")}, nil
	}

	err = app.Run(t.Context(), []string{"new", "docx", "Enterprise Collaboration Platform Overview", "Introduce this enterprise collaboration platform", "--json", "--no-publish"})
	if err == nil {
		t.Fatal("expected llm failure")
	}
	if !strings.Contains(err.Error(), "content generation failed: content generation failed: llm request failed: invalid json response body=<html><body>bad gateway</body></html>") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAppRun_NewReloadsConfigAfterInstalledSkillPreflight(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	configPath := filepath.Join(tmpDir, "config.json")
	initialOutDir := filepath.Join(tmpDir, "initial-output")
	updatedOutDir := filepath.Join(tmpDir, "updated-output")
	t.Setenv("HOME", homeDir)
	t.Setenv("OFFICE_CLI_CONFIG", configPath)
	t.Setenv(officeTaskPreflightSkipEnv, "0")
	writeTestPreflightScript(t, filepath.Join(homeDir, ".codex", "skills", "officecli", "fix-officecli-env.sh"), "#!/usr/bin/env bash\nset -euo pipefail\ncat > \"$OFFICE_CLI_CONFIG\" <<'JSON'\n{\n  \"defaults\": {\n    \"output_dir\": \""+updatedOutDir+"\",\n    \"mode\": \"fast\",\n    \"publish\": false\n  },\n  \"llm\": {\n    \"base_url\": \"https://api.example.com/v1\",\n    \"api_key\": \"llm-key\",\n    \"model\": \"gpt-4.1\"\n  },\n  \"license\": {\n    \"base_url\": \"https://license.example.com/api\",\n    \"enabled\": true,\n    \"timeout_sec\": 60\n  },\n  \"publish\": {\n    \"enabled\": false\n  }\n}\nJSON\n")

	_, err := WriteConfig("", Config{
		Defaults: DefaultsConfig{OutputDir: initialOutDir, Publish: false, Mode: "fast"},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "llm-key", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return fakeAppLLMClient{jsonResponse: `{"title":"Enterprise Collaboration Platform Overview","sections":[{"heading":"Product Overview","level":1,"paragraphs":["This collaboration platform is designed for enterprise teams."]}]}`}, nil
	}

	if err := app.Run(t.Context(), []string{"new", "docx", "Enterprise Collaboration Platform Overview", "--no-publish"}); err != nil {
		t.Fatalf("Run(new): %v", err)
	}
	entries, err := os.ReadDir(updatedOutDir)
	if err != nil {
		t.Fatalf("ReadDir(updated output): %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected generated file in updated output dir")
	}
	if _, err := os.Stat(initialOutDir); !os.IsNotExist(err) {
		t.Fatalf("expected initial output dir to remain unused, got %v", err)
	}
}

func TestAppRun_NewRetriesPreflightAfterSkillRefresh(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	configPath := filepath.Join(tmpDir, "config.json")
	counterPath := filepath.Join(tmpDir, "preflight-count")
	t.Setenv("HOME", homeDir)
	t.Setenv("OFFICE_CLI_CONFIG", configPath)
	t.Setenv(officeTaskPreflightSkipEnv, "0")
	writeTestPreflightScript(t, filepath.Join(homeDir, ".codex", "skills", "officecli", "fix-officecli-env.sh"), "#!/usr/bin/env bash\nset -euo pipefail\ncount=0\nif [[ -f \""+counterPath+"\" ]]; then count=$(cat \""+counterPath+"\"); fi\ncount=$((count+1))\nprintf '%s' \"$count\" > \""+counterPath+"\"\nif [[ \"$count\" == \"1\" ]]; then\ncat > \"$0\" <<'SCRIPT'\n#!/usr/bin/env bash\nset -euo pipefail\ncount=0\nif [[ -f \""+counterPath+"\" ]]; then count=$(cat \""+counterPath+"\"); fi\ncount=$((count+1))\nprintf '%s' \"$count\" > \""+counterPath+"\"\nexit 0\nSCRIPT\nchmod +x \"$0\"\nexit 20\nfi\n")

	_, err := WriteConfig("", Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false, Mode: "fast"},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "llm-key", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return fakeAppLLMClient{jsonResponse: `{"title":"Enterprise Collaboration Platform Overview","sections":[{"heading":"Product Overview","level":1,"paragraphs":["This collaboration platform is designed for enterprise teams."]}]}`}, nil
	}

	if err := app.Run(t.Context(), []string{"new", "docx", "Enterprise Collaboration Platform Overview", "--json", "--no-publish"}); err != nil {
		t.Fatalf("Run(new): %v", err)
	}
	raw, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatalf("ReadFile(counter): %v", err)
	}
	if strings.TrimSpace(string(raw)) != "2" {
		t.Fatalf("expected two preflight runs, got %q", string(raw))
	}
}

func TestRenderProgress_TTYClearsTrailingCharactersForShorterMessage(t *testing.T) {
	oldInterval := spinnerFrameInterval
	spinnerFrameInterval = 5 * time.Millisecond
	defer func() { spinnerFrameInterval = oldInterval }()

	out := &terminalBuffer{}
	renderer := NewProgressRenderer(out, false, true)
	renderer.Emit(context.Background(), engine.ProgressEvent{Step: "generate", Status: "running", Content: "This is a very long stage message"})
	time.Sleep(10 * time.Millisecond)
	renderer.Emit(context.Background(), engine.ProgressEvent{Step: "generate", Status: "running", Content: "Short line"})
	time.Sleep(10 * time.Millisecond)
	renderer.Emit(context.Background(), engine.ProgressEvent{Step: "generate", Status: "completed", Content: "Short line completed"})

	history := strings.Join(out.History(), "\n")
	if strings.Contains(history, "Short linege") || strings.Contains(history, "Short linessage") {
		t.Fatalf("expected trailing characters to be cleared, got %q", history)
	}
	if !strings.Contains(history, "✔ Short line completed") {
		t.Fatalf("expected completed short line, got %q", history)
	}
}

func TestRenderProgress_PauseStopsSpinnerAndPrintsWaitingLine(t *testing.T) {
	oldInterval := spinnerFrameInterval
	spinnerFrameInterval = 5 * time.Millisecond
	defer func() { spinnerFrameInterval = oldInterval }()

	out := &terminalBuffer{}
	renderer := NewProgressRenderer(out, false, true)
	renderer.Emit(context.Background(), engine.ProgressEvent{Step: "question", Status: "running", Content: "Waiting for your answer to a follow-up question"})
	time.Sleep(15 * time.Millisecond)
	renderer.Pause("Waiting for your input")
	rawAfterPause := out.String()
	time.Sleep(20 * time.Millisecond)

	if out.String() != rawAfterPause {
		t.Fatalf("expected spinner to stop after pause")
	}
	history := strings.Join(out.History(), "\n")
	if !strings.Contains(history, "… Waiting for your input") {
		t.Fatalf("expected waiting line after pause, got %q", history)
	}
}

func TestRenderProgress_NonTTYPrintsStageLines(t *testing.T) {
	var out bytes.Buffer
	renderer := NewProgressRenderer(&out, false, false)
	renderer.Emit(context.Background(), engine.ProgressEvent{Step: "license", Status: "running", Content: "Checking access status", ElapsedMs: 5})
	renderer.Emit(context.Background(), engine.ProgressEvent{Step: "generate", Status: "running", Content: "Generating document content", ElapsedMs: 15})

	output := out.String()
	if strings.Contains(output, "%") {
		t.Fatalf("progress output should not contain percent: %s", output)
	}
	for _, needle := range []string{"Checking access status", "Generating document content"} {
		if !strings.Contains(output, needle) {
			t.Fatalf("progress output missing %q: %s", needle, output)
		}
	}
}

func TestAppRun_NewShowsProgressBeforeFinalResult(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)

	_, err := WriteConfig("", Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false, Mode: "fast"},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "llm-key", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return fakeAppLLMClient{jsonResponse: `{"title":"Enterprise Collaboration Platform Overview","theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},"slides":[{"title":"Enterprise Collaboration Platform Overview","layout":"title","subtitle":"Product context and business status","isTitle":true},{"title":"Product Capabilities","layout":"content","points":["Multi-user collaboration","Real-time editing","Enterprise administration"]}]}`, delay: 25 * time.Millisecond}, nil
	}

	err = app.Run(t.Context(), []string{"new", "pptx", "Enterprise Collaboration Platform Overview", "Describe the product capabilities, customer value, and use cases of this collaboration platform.", "--no-publish"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := stdout.String()
	for _, needle := range []string{"Checking access status", "Generating document content", "Writing local files", "Generation completed. Saved to"} {
		if !strings.Contains(output, needle) {
			t.Fatalf("stdout missing %q: %s", needle, output)
		}
	}
	if strings.Index(output, "Generating document content") > strings.Index(output, "Generation completed. Saved to") {
		t.Fatalf("progress should appear before final result: %s", output)
	}
}

func TestAppRun_NewTTYShowsSpinnerFrames(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)

	_, err := WriteConfig("", Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false, Mode: "fast"},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "llm-key", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	oldInterval := spinnerFrameInterval
	spinnerFrameInterval = 5 * time.Millisecond
	defer func() { spinnerFrameInterval = oldInterval }()

	stdout := &terminalBuffer{}
	app := NewApp(stdout, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return fakeAppLLMClient{jsonResponse: `{"title":"Enterprise Collaboration Platform Overview","theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},"slides":[{"title":"Enterprise Collaboration Platform Overview","layout":"title","subtitle":"Product context and business status","isTitle":true},{"title":"Product Capabilities","layout":"content","points":["Multi-user collaboration","Real-time editing","Enterprise administration"]}]}`, delay: 25 * time.Millisecond}, nil
	}

	err = app.Run(t.Context(), []string{"new", "pptx", "Enterprise Collaboration Platform Overview", "Describe the product capabilities, customer value, and use cases of this collaboration platform.", "--no-publish"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	raw := stdout.String()
	if !strings.Contains(raw, "⠋") || !strings.Contains(raw, "⠙") {
		t.Fatalf("expected spinner frames in tty output, got %q", raw)
	}
	for _, needle := range []string{"✔ Access check completed", "✔ Document generated"} {
		if !strings.Contains(raw, needle) {
			t.Fatalf("expected transient tty output to include %q, got %q", needle, raw)
		}
	}
	history := strings.Join(stdout.History(), "\n")
	if strings.Contains(history, "✔ Access check completed") || strings.Contains(history, "✔ Document generated") {
		t.Fatalf("tty history should not retain transient completion lines: %s", history)
	}
	if !strings.Contains(history, "Generation completed. Saved to") {
		t.Fatalf("tty history missing final result: %s", history)
	}
}

func TestAppRun_NewJSONSkipsProgressOutput(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)

	_, err := WriteConfig("", Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false, Mode: "fast"},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "llm-key", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return fakeAppLLMClient{jsonResponse: `{"title":"Enterprise Collaboration Platform Overview","sections":[{"heading":"Product Overview","level":1,"paragraphs":["This collaboration platform is designed for enterprise teams."]}]}`}, nil
	}

	err = app.Run(t.Context(), []string{"new", "docx", "Enterprise Collaboration Platform Overview", "Introduce this enterprise collaboration platform", "--json", "--no-publish"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := stdout.String()
	if strings.Contains(output, "5%") || strings.Contains(output, "60%") || strings.Contains(output, "100%") {
		t.Fatalf("json output should not contain progress text: %s", output)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json output invalid: %v, output=%s", err, output)
	}
	if payload["status"] != "success" {
		t.Fatalf("status = %v", payload["status"])
	}
}

func TestBuildGenerateJob_PromptPrecedence(t *testing.T) {
	tmpDir := t.TempDir()
	promptFile := filepath.Join(tmpDir, "prompt.txt")
	if err := os.WriteFile(promptFile, []byte("from file"), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	cfg := Config{}
	job, err := BuildGenerateJob([]string{
		"pptx",
		"Positional Title",
		"Positional Description",
		"--prompt-file", promptFile,
		"--prompt", "from flag",
	}, cfg, InputSources{
		Stdin: "from stdin",
		IsTTY: true,
		CWD:   tmpDir,
	})
	if err != nil {
		t.Fatalf("BuildGenerateJob: %v", err)
	}

	if job.Prompt != "from flag" {
		t.Fatalf("prompt = %q, want flag value", job.Prompt)
	}
	if job.Topic != "Positional Title" {
		t.Fatalf("topic = %q", job.Topic)
	}
}

func TestBuildGenerateJob_UsesPromptFileBeforeStdinAndPositionals(t *testing.T) {
	tmpDir := t.TempDir()
	promptFile := filepath.Join(tmpDir, "prompt.txt")
	if err := os.WriteFile(promptFile, []byte("from file"), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	job, err := BuildGenerateJob([]string{
		"docx",
		"Document Title",
		"Positional Description",
		"--prompt-file", promptFile,
	}, Config{}, InputSources{
		Stdin: "from stdin",
		IsTTY: true,
		CWD:   tmpDir,
	})
	if err != nil {
		t.Fatalf("BuildGenerateJob: %v", err)
	}

	if job.Prompt != "from file" {
		t.Fatalf("prompt = %q", job.Prompt)
	}
}

func TestBuildGenerateJob_PublishFlagsOverrideConfig(t *testing.T) {
	cfg := Config{}
	cfg.Defaults.Publish = true

	job, err := BuildGenerateJob([]string{
		"xlsx",
		"Report",
		"--no-publish",
	}, cfg, InputSources{IsTTY: true, CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("BuildGenerateJob: %v", err)
	}
	if job.Publish {
		t.Fatal("expected publish to be disabled by flag")
	}
}

func TestBuildGenerateJob_ImagesEnabledByDefault(t *testing.T) {
	job, err := BuildGenerateJob([]string{
		"pptx",
		"Illustrated Demo",
	}, Config{}, InputSources{IsTTY: true, CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("BuildGenerateJob: %v", err)
	}
	if !job.EnableImages {
		t.Fatal("expected images to be enabled by default")
	}
}

func TestBuildGenerateJob_NoImagesDisablesImageGeneration(t *testing.T) {
	job, err := BuildGenerateJob([]string{
		"pptx",
		"Illustrated Demo",
		"--no-images",
	}, Config{}, InputSources{IsTTY: true, CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("BuildGenerateJob: %v", err)
	}
	if job.EnableImages {
		t.Fatal("expected images to be disabled by flag")
	}
}

func TestBuildGenerateJob_IMGDefaultsToServerImageRuntime(t *testing.T) {
	cfg := Config{Defaults: DefaultsConfig{Publish: false, Mode: "best"}}

	job, err := BuildGenerateJob([]string{
		"img",
		"Launch Visual",
		"--prompt", "A polished product launch hero image",
	}, cfg, InputSources{IsTTY: true, CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("BuildGenerateJob: %v", err)
	}
	if job.DocumentType != engine.DocumentTypeIMG {
		t.Fatalf("document type = %q", job.DocumentType)
	}
	if job.RuntimeMode != RuntimeModeExternal {
		t.Fatalf("runtime mode = %q", job.RuntimeMode)
	}
	if job.Mode != "fast" {
		t.Fatalf("mode = %q", job.Mode)
	}
	if job.ImageRatio != "square" {
		t.Fatalf("image ratio = %q", job.ImageRatio)
	}
	if !job.Publish {
		t.Fatal("img generation should publish by default")
	}
	if len(job.Warnings) != 0 {
		t.Fatalf("warnings = %#v", job.Warnings)
	}
}

func TestBuildGenerateJob_IMGNoPublishDisablesDefaultPublish(t *testing.T) {
	job, err := BuildGenerateJob([]string{
		"img",
		"Launch Visual",
		"--no-publish",
	}, Config{}, InputSources{IsTTY: true, CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("BuildGenerateJob: %v", err)
	}
	if job.Publish {
		t.Fatal("expected --no-publish to disable image publishing")
	}
}

func TestBuildGenerateJob_IMGAcceptsExplicitPublish(t *testing.T) {
	job, err := BuildGenerateJob([]string{
		"img",
		"Launch Visual",
		"--publish",
	}, Config{}, InputSources{IsTTY: true, CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("BuildGenerateJob: %v", err)
	}
	if !job.Publish {
		t.Fatal("expected --publish to keep image publishing enabled")
	}
}

func TestBuildGenerateJob_IMGRejectsUnsupportedOptions(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "invalid ratio", args: []string{"img", "Demo", "--ratio", "ultrawide"}, want: "unsupported image ratio"},
		{name: "best mode", args: []string{"img", "Demo", "--mode", "best"}, want: "--mode best is not supported for img generation"},
		{name: "file", args: []string{"img", "Demo", "--file", "input.xlsx"}, want: "--file is not supported for img generation"},
		{name: "local preview", args: []string{"img", "Demo", "--local-preview"}, want: "--local-preview is not supported for img generation"},
		{name: "no images", args: []string{"img", "Demo", "--no-images"}, want: "--no-images is not supported for img generation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildGenerateJob(tc.args, Config{}, InputSources{IsTTY: true, CWD: t.TempDir()})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestBuildGenerateJob_IMGAcceptsReferenceImageSource(t *testing.T) {
	tmpDir := t.TempDir()
	refPath := filepath.Join(tmpDir, "reference.png")
	if err := os.WriteFile(refPath, []byte("png-bytes"), 0o600); err != nil {
		t.Fatalf("write reference image: %v", err)
	}

	job, err := BuildGenerateJob([]string{
		"img",
		"Launch Visual",
		"--reference-image", refPath,
		"--no-publish",
	}, Config{}, InputSources{IsTTY: true, CWD: tmpDir})
	if err != nil {
		t.Fatalf("BuildGenerateJob: %v", err)
	}
	if job.ReferenceImageSource != refPath {
		t.Fatalf("reference image source = %q, want %q", job.ReferenceImageSource, refPath)
	}

	job, err = BuildGenerateJob([]string{
		"img",
		"Launch Visual",
		"--reference-image=https://assets.example.com/reference.jpg",
		"--no-publish",
	}, Config{}, InputSources{IsTTY: true, CWD: tmpDir})
	if err != nil {
		t.Fatalf("BuildGenerateJob URL: %v", err)
	}
	if job.ReferenceImageSource != "https://assets.example.com/reference.jpg" {
		t.Fatalf("reference image URL = %q", job.ReferenceImageSource)
	}
}

func TestBuildGenerateJob_ReferenceImageOnlySupportedForIMG(t *testing.T) {
	_, err := BuildGenerateJob([]string{
		"pptx",
		"Demo",
		"--reference-image", "reference.png",
	}, Config{}, InputSources{IsTTY: true, CWD: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "--reference-image is only supported for img generation") {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildGenerateJob_ReferenceImageRejectsMultipleValues(t *testing.T) {
	_, err := BuildGenerateJob([]string{
		"img",
		"Demo",
		"--reference-image", "first.png",
		"--reference-image=second.png",
	}, Config{}, InputSources{IsTTY: true, CWD: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "only one --reference-image is supported") {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildGenerateJob_RatioOnlySupportedForIMG(t *testing.T) {
	_, err := BuildGenerateJob([]string{
		"pptx",
		"Demo",
		"--ratio", "landscape",
	}, Config{}, InputSources{IsTTY: true, CWD: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "--ratio is only supported for img generation") {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildGenerateJob_UsesDefaultPPTStylePresetAndLocalPreview(t *testing.T) {
	cfg := Config{}
	cfg.Defaults.PPTXStylePreset = "executive-dark"

	job, err := BuildGenerateJob([]string{
		"pptx",
		"Board Presentation",
		"--local-preview",
	}, cfg, InputSources{IsTTY: true, CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("BuildGenerateJob: %v", err)
	}
	if job.Style != "executive-dark" {
		t.Fatalf("style = %q", job.Style)
	}
	if !job.LocalPreview {
		t.Fatal("expected local preview to be enabled")
	}
}

func TestBuildGenerateJob_DocxAllowsLocalPreview(t *testing.T) {
	job, err := BuildGenerateJob([]string{
		"docx",
		"Board Memo",
		"--local-preview",
	}, Config{}, InputSources{IsTTY: true, CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("BuildGenerateJob: %v", err)
	}
	if !job.LocalPreview {
		t.Fatal("expected docx local preview to be enabled")
	}
}

func TestBuildGenerateJob_DebugFlag(t *testing.T) {
	job, err := BuildGenerateJob([]string{
		"pptx",
		"Board Presentation",
		"--debug",
	}, Config{}, InputSources{IsTTY: true, CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("BuildGenerateJob: %v", err)
	}
	if !job.Debug {
		t.Fatal("expected debug to be enabled")
	}
}

func TestBuildGenerateJob_ReportRequiresWorkbookFile(t *testing.T) {
	_, err := BuildGenerateJob([]string{
		"report",
		"Board Review",
	}, Config{}, InputSources{IsTTY: true, CWD: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "requires --file") {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildGenerateJob_ReportAcceptsWorkbookFile(t *testing.T) {
	tmpDir := t.TempDir()
	workbookPath := filepath.Join(tmpDir, "metrics.xlsx")
	if err := os.WriteFile(workbookPath, []byte("demo"), 0o644); err != nil {
		t.Fatalf("write workbook: %v", err)
	}

	job, err := BuildGenerateJob([]string{
		"report",
		"Board Review",
		"--file", workbookPath,
		"--prompt", "Summarize the key shifts",
	}, Config{}, InputSources{IsTTY: true, CWD: tmpDir})
	if err != nil {
		t.Fatalf("BuildGenerateJob: %v", err)
	}
	if job.SourceFilePath != workbookPath {
		t.Fatalf("source file = %q", job.SourceFilePath)
	}
	if job.DocumentType != engine.DocumentTypeReport {
		t.Fatalf("document type = %q", job.DocumentType)
	}
}

func TestExecuteGenerateJob_IMGUsesServerImageRouteAndGenerationQuota(t *testing.T) {
	imageBytes := []byte("server-png-bytes")
	var gotAuth string
	var gotPayload map[string]any
	var gotPublishAuth string
	var gotPublishDocType string
	var gotPublishDocName string
	var gotPublishFileName string
	var gotPublishFileBytes []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/llm/v1/image" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = fmt.Fprintf(w, `{"data":"%s","mime":"image/png","access_mode":"paid","remaining":89,"paid_quota_remaining":89}`, base64.StdEncoding.EncodeToString(imageBytes))
	}))
	defer server.Close()
	publishServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/publish" {
			t.Fatalf("unexpected publish path: %s", r.URL.Path)
		}
		gotPublishAuth = r.Header.Get("Authorization")
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			t.Fatalf("parse publish multipart: %v", err)
		}
		gotPublishDocType = r.FormValue("document_type")
		gotPublishDocName = r.FormValue("document_name")
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("publish file: %v", err)
		}
		defer file.Close()
		gotPublishFileName = header.Filename
		gotPublishFileBytes, err = io.ReadAll(file)
		if err != nil {
			t.Fatalf("read publish file: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_url":"https://officecli.io/p/share-img","password":"654321","file_id":"file-img","expires_at":"2026-05-06T00:00:00Z"}`)
	}))
	defer publishServer.Close()

	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		t.Fatal("img generation must not initialize the local generation provider")
		return nil, nil
	}
	licenseEvents := []string{}
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return &orderedLicenseManager{
			events: &licenseEvents,
			checkResult: &LicenseCheckResult{
				Allowed:            true,
				AccessMode:         LicenseAccessModePaid,
				PaidQuotaRemaining: 90,
			},
		}, nil
	}

	tmpDir := t.TempDir()
	job, err := BuildGenerateJob([]string{
		"img",
		"Launch Visual",
		"--prompt", "A polished product launch hero image",
		"--ratio", "landscape",
	}, Config{Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: true}}, InputSources{IsTTY: true, CWD: tmpDir})
	if err != nil {
		t.Fatalf("BuildGenerateJob: %v", err)
	}
	result, err := app.executeGenerateJob(t.Context(), Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: true},
		License:  LicenseConfig{BaseURL: server.URL, APIKey: "hosted-key", Enabled: true, TimeoutSec: 5},
		Publish:  publishprovider.Config{Enabled: true, BaseURL: publishServer.URL},
	}, job, false, noopProgressController{}, nil)
	if err != nil {
		t.Fatalf("executeGenerateJob: %v", err)
	}
	if gotAuth != "Bearer hosted-key" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotPayload["model"] != "hosted/img" {
		t.Fatalf("model = %#v", gotPayload["model"])
	}
	if gotPayload["aspect_ratio"] != 16.0/9.0 {
		t.Fatalf("aspect_ratio = %#v", gotPayload["aspect_ratio"])
	}
	if result.DocumentType != "img" {
		t.Fatalf("document type = %q", result.DocumentType)
	}
	if gotPayload["commit_token"] == nil || gotPayload["access_mode"] != "paid" {
		t.Fatalf("quota payload = %#v", gotPayload)
	}
	if result.AccessMode != "paid" || result.PaidQuotaRemaining != 89 || result.Remaining != 89 {
		t.Fatalf("quota result = %+v", result)
	}
	data, err := os.ReadFile(result.FilePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != string(imageBytes) {
		t.Fatalf("image bytes = %q", string(data))
	}
	if filepath.Ext(result.FilePath) != ".png" {
		t.Fatalf("file path = %s", result.FilePath)
	}
	if !result.Published {
		t.Fatal("img generation should publish")
	}
	if result.AccessURL != "https://officecli.io/p/share-img" || result.Password != "654321" || result.ExpiresAt != "2026-05-06T00:00:00Z" {
		t.Fatalf("publish result = %+v", result)
	}
	if gotPublishAuth != "Bearer hosted-key" {
		t.Fatalf("publish authorization = %q", gotPublishAuth)
	}
	if gotPublishDocType != "img" {
		t.Fatalf("publish document_type = %q", gotPublishDocType)
	}
	if gotPublishDocName != result.DocumentName {
		t.Fatalf("publish document_name = %q, result name = %q", gotPublishDocName, result.DocumentName)
	}
	if gotPublishFileName != filepath.Base(result.FilePath) {
		t.Fatalf("publish filename = %q, file path = %q", gotPublishFileName, result.FilePath)
	}
	if string(gotPublishFileBytes) != string(imageBytes) {
		t.Fatalf("published image bytes = %q", string(gotPublishFileBytes))
	}
	if strings.Join(licenseEvents, ",") != "check" {
		t.Fatalf("license events = %#v, want check only because server consumes image quota", licenseEvents)
	}
}

func TestExecuteGenerateJob_IMGUsesExtendedServerImageTimeout(t *testing.T) {
	imageBytes := []byte("slow-server-png-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/llm/v1/image" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		time.Sleep(1100 * time.Millisecond)
		_, _ = fmt.Fprintf(w, `{"data":"%s","mime":"image/png","access_mode":"free","remaining":2,"free_remaining":2}`, base64.StdEncoding.EncodeToString(imageBytes))
	}))
	defer server.Close()

	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{
			Allowed:       true,
			AccessMode:    LicenseAccessModeFree,
			FreeRemaining: 3,
		}}, nil
	}

	tmpDir := t.TempDir()
	result, err := app.executeGenerateJob(t.Context(), Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false},
		License:  LicenseConfig{BaseURL: server.URL, Enabled: true, TimeoutSec: 1},
	}, GenerateJob{
		DocumentType: engine.DocumentTypeIMG,
		Topic:        "Slow Image",
		Prompt:       "A polished product launch hero image",
		RuntimeMode:  RuntimeModeExternal,
		Mode:         "fast",
		OutputDir:    tmpDir,
		ImageRatio:   "square",
	}, false, noopProgressController{}, nil)
	if err != nil {
		t.Fatalf("executeGenerateJob: %v", err)
	}
	if result.FreeRemaining != 2 || result.Remaining != 2 {
		t.Fatalf("quota result = %+v", result)
	}
}

func TestExecuteGenerateJob_IMGRequiresLicenseConfig(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	tmpDir := t.TempDir()
	job := GenerateJob{
		DocumentType: engine.DocumentTypeIMG,
		Topic:        "Launch Visual",
		Prompt:       "A polished product launch hero image",
		OutputDir:    tmpDir,
		ImageRatio:   "square",
	}

	_, err := app.executeGenerateJob(t.Context(), Config{}, job, false, noopProgressController{}, nil)
	if err == nil || !strings.Contains(err.Error(), "officecli config set-license") {
		t.Fatalf("err = %v", err)
	}
}

func TestCompleteBestModeWithPrompter_DebugLogsTemplateFallback(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr, bytes.NewBuffer(nil))
	llm := &fakeBestModeLLMClient{
		structuredErrors: []error{
			fmt.Errorf("upstream unavailable"),
			fmt.Errorf("upstream unavailable"),
		},
		jsonErrors: []error{fmt.Errorf("json unavailable")},
	}

	_, err := app.completeBestModeWithPrompter(
		t.Context(),
		llm,
		errorPrompter{err: fmt.Errorf("stop after debug output")},
		GenerateJob{
			DocumentType: engine.DocumentTypePPTX,
			Prompt:       "Create a quarterly project review deck",
			Mode:         "best",
			Debug:        true,
		},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "stop after debug output") {
		t.Fatalf("err = %v", err)
	}
	debugOutput := stderr.String()
	for _, needle := range []string{
		"question_source=template_fallback",
		"question_error_kind=llm_request_failed",
		"current_question_id=ppt_report_audience",
	} {
		if !strings.Contains(debugOutput, needle) {
			t.Fatalf("stderr missing %q:\n%s", needle, debugOutput)
		}
	}
}

func TestCompleteBestModeWithPrompter_EmitsPlanSynthesisProgressAfterFinalAnswer(t *testing.T) {
	app := &App{}
	llm := &fakeBestModeLLMClient{
		structuredResponses: []string{
			`{"questions":[{"id":"audience","question":"Who is the main audience for this deck?","allowFreeform":true,"options":[{"id":"management","label":"Leadership","description":"Emphasize conclusions and judgment.","recommended":true},{"id":"team","label":"Internal team","description":"Emphasize execution detail.","recommended":false}]}]}`,
			`{"plan_markdown":"# Execution Plan\n\n## Summary\n- Conclusion-first for leadership.","execution_prompt":"Generate the PPT in 6 slides or fewer, for leadership, with a conclusion-first structure."}`,
		},
		jsonResponses: []string{
			`{"presentationType":"Overview deck","targetAudience":"Leadership","presentationPurpose":"Introduce Minecraft","pageCount":6,"contentStyle":"Conclusion-first","visualEffect":"Clean and credible","slideOutline":[{"slideIndex":1,"purpose":"Cover","contentFormat":"paragraph","suggestedLayout":"title","maxItems":1,"contentRequirements":"State the topic and audience","visualSuggestion":"hero"}],"contentGuideline":"Keep one core point per slide"}`,
		},
	}
	progress := &progressCollector{}

	job, err := app.completeBestModeWithPrompter(
		t.Context(),
		llm,
		fixedPrompter{optionID: "1"},
		GenerateJob{
			DocumentType: engine.DocumentTypePPTX,
			Prompt:       "Create a PPT about Minecraft",
			Mode:         "best",
		},
		progress,
	)
	if err != nil {
		t.Fatalf("completeBestModeWithPrompter: %v", err)
	}
	if !strings.Contains(job.Prompt, "leadership") {
		t.Fatalf("prompt = %q", job.Prompt)
	}

	foundRunning := false
	foundCompleted := false
	for _, event := range progress.events {
		if event.Step == progressStepPlanPrepare && event.Status == "running" && strings.Contains(event.Content, "Synthesizing the execution plan from your answers") {
			foundRunning = true
		}
		if event.Step == progressStepPlanPrepare && event.Status == "completed" && strings.Contains(event.Content, "Execution plan synthesized from your answers") {
			foundCompleted = true
		}
	}
	if !foundRunning {
		t.Fatalf("missing plan synthesis running progress: %#v", progress.events)
	}
	if !foundCompleted {
		t.Fatalf("missing plan synthesis completed progress: %#v", progress.events)
	}
}

func TestRenderResult_HumanSummarizesQuotaInSingleLine(t *testing.T) {
	var out bytes.Buffer
	result := GenerateResult{
		Status:             "success",
		FilePath:           "/tmp/test.html",
		DocumentType:       "report",
		DocumentName:       "test.html",
		AccessMode:         "paid",
		Remaining:          109,
		FreeRemaining:      10,
		RewardRemaining:    0,
		PaidQuotaRemaining: 109,
		Warnings: []string{
			"Current mode: paid. 109 document generations remaining.",
			"Trial today on this machine: 10 remaining.",
			"Reward quota: 0 remaining.",
			"Paid quota on current key: 109 remaining.",
			"Publishing is not configured, so online preview publishing was skipped.",
		},
	}

	if err := RenderResult(&out, result, false); err != nil {
		t.Fatalf("RenderResult: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Access: paid mode; 109 generations remaining; trial 10") {
		t.Fatalf("missing access summary: %s", output)
	}
	for _, needle := range []string{
		"Warning: Current mode: paid.",
		"Warning: Trial today on this machine:",
		"Warning: Reward quota:",
		"Warning: Paid quota on current key:",
	} {
		if strings.Contains(output, needle) {
			t.Fatalf("quota warning should not be rendered separately: %s", output)
		}
	}
	if !strings.Contains(output, "Warning: Publishing is not configured, so online preview publishing was skipped.") {
		t.Fatalf("expected non-quota warning to remain: %s", output)
	}
}

func TestRenderResult_JSONIncludesPublishFields(t *testing.T) {
	var out bytes.Buffer
	result := GenerateResult{
		Status:                 "success",
		FilePath:               "/tmp/test.pptx",
		DocumentType:           "pptx",
		DocumentName:           "test.pptx",
		Published:              true,
		AccessURL:              "https://example.com/preview/1",
		Password:               "123456",
		ExpiresAt:              "2026-04-04T08:00:00Z",
		AccessMode:             "free",
		PublishedSkippedReason: "publish disabled",
	}

	if err := RenderResult(&out, result, true); err != nil {
		t.Fatalf("RenderResult: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if payload["access_url"] != result.AccessURL {
		t.Fatalf("access_url = %v", payload["access_url"])
	}
	if payload["password"] != result.Password {
		t.Fatalf("password = %v", payload["password"])
	}
	if payload["expires_at"] != result.ExpiresAt {
		t.Fatalf("expires_at = %v", payload["expires_at"])
	}
	if payload["access_mode"] != "free" {
		t.Fatalf("access_mode = %v", payload["access_mode"])
	}
	if payload["published_skipped_reason"] != "publish disabled" {
		t.Fatalf("published_skipped_reason = %v", payload["published_skipped_reason"])
	}
}

func TestAppRun_HelpOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr, bytes.NewBuffer(nil))

	if err := app.Run(t.Context(), []string{"--help"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := stdout.String()
	for _, needle := range []string{
		"officecli",
		"config                  View or update local configuration",
		"auth                    View or update access settings",
		"score                   Run PPTX scoring on demand",
		"upgrade                 Check for updates and upgrade officecli",
		"new <pptx|docx|xlsx|report|img> <topic> [brief]",
		"--ratio <value>",
		"officecli config status",
		"officecli upgrade --help",
		"officecli score --help",
		"officecli auth --help",
		"officecli new --help",
		"Commands:",
		"Default behavior:",
		"Config file:",
		"macOS   ~/Library/Application Support/officecli/config.json",
		"Linux   ~/.config/officecli/config.json",
		"officecli new pptx \"Enterprise Collaboration Platform Overview\" \"Explain the product capabilities, customer value, and use cases of this enterprise collaboration platform\"",
		"officecli new img \"Launch Visual\"",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("help output missing %q: %s", needle, output)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestAppRun_SubcommandHelpOutput(t *testing.T) {
	cases := []struct {
		args    []string
		needles []string
	}{
		{args: []string{"config", "--help"}, needles: []string{"Usage:", "officecli config status", "officecli config set-generation", "officecli config set-license"}},
		{args: []string{"auth", "--help"}, needles: []string{"officecli auth status", "officecli auth set-key", "View access status or save a paid API key."}},
		{args: []string{"score", "--help"}, needles: []string{"officecli score pptx <file>", "Scoring does not run automatically after generation"}},
		{args: []string{"upgrade", "--help"}, needles: []string{"officecli upgrade", "apply the upgrade using the current installation channel"}},
		{args: []string{"new", "--help"}, needles: []string{"officecli new <pptx|docx|xlsx|report|img>", "--prompt-file", "--mode fast|best", "--file <path>", "--ratio <value>", "automatic PPT images", "officecli config set-generation", "requires `--file <xlsx-path>`", "officecli config set-license"}},
		{args: []string{"new", "pptx", "--help"}, needles: []string{"officecli new <pptx|docx|xlsx|report|img>", "--prompt-file", "--mode fast|best"}},
		{args: []string{"review", "pptx", "--help"}, needles: []string{"officecli review pptx <file>", "--no-visual"}},
	}
	for _, tc := range cases {
		var stdout bytes.Buffer
		app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
		if err := app.Run(t.Context(), tc.args); err != nil {
			t.Fatalf("Run(%v): %v", tc.args, err)
		}
		out := stdout.String()
		for _, needle := range tc.needles {
			if !strings.Contains(out, needle) {
				t.Fatalf("help(%v) missing %q: %s", tc.args, needle, out)
			}
		}
	}
}

func TestAppRun_VersionOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr, bytes.NewBuffer(nil))
	originalVersion := Version
	originalCommit := Commit
	originalBuildDate := BuildDate
	Version = "0.1.0-test"
	Commit = "abc1234"
	BuildDate = "2026-03-31T10:00:00Z"
	defer func() {
		Version = originalVersion
		Commit = originalCommit
		BuildDate = originalBuildDate
	}()

	if err := app.Run(t.Context(), []string{"--version"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "officecli version 0.1.0-test") {
		t.Fatalf("unexpected version output: %s", output)
	}
	if !strings.Contains(output, "abc1234") {
		t.Fatalf("version output missing commit: %s", output)
	}
	if !strings.Contains(output, "2026-03-31T10:00:00Z") {
		t.Fatalf("version output missing build date: %s", output)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestAppRun_UpgradeCommandUpdatesImmediately(t *testing.T) {
	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.checkForUpdates = func(ctx context.Context) (UpdateInfo, error) {
		return UpdateInfo{
			Available:           true,
			CurrentVersion:      "0.2.5",
			LatestVersionLabel:  "0.2.6",
			InstallMethod:       InstallMethodNPM,
			PackageManager:      "npm",
			Channel:             UpdateChannelNPM,
			AutoUpdateSupported: true,
			UpdateCommand:       "npm install -g officecli",
		}, nil
	}
	updated := false
	app.performUpdate = func(ctx context.Context, info UpdateInfo) error {
		updated = true
		if info.InstallMethod != InstallMethodNPM || info.PackageManager != "npm" {
			t.Fatalf("unexpected update info: %+v", info)
		}
		return nil
	}
	app.restartCommand = func(ctx context.Context, info UpdateInfo, args []string) error {
		t.Fatal("restartCommand should not be called for explicit upgrade")
		return nil
	}

	if err := app.Run(t.Context(), []string{"upgrade"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !updated {
		t.Fatal("performUpdate was not called")
	}
	output := stdout.String()
	if !strings.Contains(output, "Update available for officecli: current 0.2.5, latest stable 0.2.6.") {
		t.Fatalf("stdout = %q", output)
	}
	if !strings.Contains(output, "Suggested update command: npm install -g officecli") {
		t.Fatalf("stdout = %q", output)
	}
	if !strings.Contains(output, "officecli was updated.") {
		t.Fatalf("stdout = %q", output)
	}
}

func TestAppRun_UpgradeCommandReportsUpToDate(t *testing.T) {
	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.checkForUpdates = func(ctx context.Context) (UpdateInfo, error) {
		return UpdateInfo{
			Available:      false,
			CurrentVersion: "0.2.6",
		}, nil
	}
	app.performUpdate = func(ctx context.Context, info UpdateInfo) error {
		t.Fatal("performUpdate should not be called")
		return nil
	}

	if err := app.Run(t.Context(), []string{"upgrade"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "officecli is already up to date (0.2.6).") {
		t.Fatalf("stdout = %q", got)
	}
}

func TestAppRun_UpgradeCommandPrintsManualInstructionWhenAutoUpgradeUnsupported(t *testing.T) {
	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.checkForUpdates = func(ctx context.Context) (UpdateInfo, error) {
		return UpdateInfo{
			Available:           true,
			CurrentVersion:      "0.2.5",
			LatestVersionLabel:  "0.2.6",
			InstallMethod:       InstallMethodUnknown,
			AutoUpdateSupported: false,
			UpdateCommand:       "curl -fsSL https://example.com/install.sh | bash",
		}, nil
	}
	app.performUpdate = func(ctx context.Context, info UpdateInfo) error {
		t.Fatal("performUpdate should not be called")
		return nil
	}

	if err := app.Run(t.Context(), []string{"upgrade"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Suggested update command: curl -fsSL https://example.com/install.sh | bash") {
		t.Fatalf("stdout = %q", output)
	}
	if strings.Contains(output, "officecli was updated.") {
		t.Fatalf("stdout = %q", output)
	}
}

func TestAppRun_InteractiveUpdatePromptRunsUpdaterAndRestarts(t *testing.T) {
	t.Setenv(updateCheckSkipEnv, "0")
	stdout := &terminalBuffer{}
	var stderr bytes.Buffer
	app := NewApp(stdout, &stderr, &terminalInputBuffer{Reader: strings.NewReader("1\n")})
	originalVersion := Version
	originalBuildDate := BuildDate
	Version = "0.2.2"
	BuildDate = "2026-04-09T09:07:59Z"
	defer func() {
		Version = originalVersion
		BuildDate = originalBuildDate
	}()
	app.checkForUpdates = func(ctx context.Context) (UpdateInfo, error) {
		return UpdateInfo{
			Available:           true,
			CurrentVersion:      "0.2.2",
			LatestVersionLabel:  "0.2.6",
			InstallMethod:       InstallMethodScript,
			Channel:             UpdateChannelLatest,
			AutoUpdateSupported: true,
			UpdateCommand:       "curl -fsSL https://example.com/install.sh | bash",
		}, nil
	}
	updated := false
	restarted := false
	app.performUpdate = func(ctx context.Context, info UpdateInfo) error {
		updated = true
		return nil
	}
	app.restartCommand = func(ctx context.Context, info UpdateInfo, args []string) error {
		restarted = true
		if info.InstallMethod != InstallMethodScript {
			t.Fatalf("unexpected restart install method: %q", info.InstallMethod)
		}
		if len(args) != 2 || args[0] != "config" || args[1] != "status" {
			t.Fatalf("unexpected restart args: %v", args)
		}
		return nil
	}

	if err := app.Run(t.Context(), []string{"config", "status"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !updated || !restarted {
		t.Fatalf("updated=%t restarted=%t", updated, restarted)
	}
	if !strings.Contains(stdout.String(), "Update available for officecli") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "1. Update now and continue") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "Config file:") {
		t.Fatalf("current process should stop after restart, stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestAppRun_InteractiveUpdatePromptStopsParentBeforeNewXLSXGeneration(t *testing.T) {
	t.Setenv(updateCheckSkipEnv, "0")
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	outputDir := filepath.Join(tmpDir, "output")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)

	_, err := WriteConfig("", Config{
		Defaults: DefaultsConfig{OutputDir: outputDir, Publish: false, Mode: "fast"},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "llm-key", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	stdout := &terminalBuffer{}
	var stderr bytes.Buffer
	app := NewApp(stdout, &stderr, &terminalInputBuffer{Reader: strings.NewReader("1\n")})
	originalVersion := Version
	originalBuildDate := BuildDate
	Version = "0.2.12"
	BuildDate = "2026-04-16T10:37:00Z"
	defer func() {
		Version = originalVersion
		BuildDate = originalBuildDate
	}()

	app.checkForUpdates = func(ctx context.Context) (UpdateInfo, error) {
		return UpdateInfo{
			Available:           true,
			CurrentVersion:      "0.2.12",
			LatestVersionLabel:  "0.2.13",
			InstallMethod:       InstallMethodScript,
			Channel:             UpdateChannelLatest,
			AutoUpdateSupported: true,
			UpdateCommand:       "curl -fsSL https://example.com/install.sh | bash",
		}, nil
	}

	updated := false
	restarted := false
	preflightCalls := 0
	licenseClientCalls := 0
	llmClientCalls := 0
	app.performUpdate = func(ctx context.Context, info UpdateInfo) error {
		updated = true
		return nil
	}
	app.restartCommand = func(ctx context.Context, info UpdateInfo, args []string) error {
		restarted = true
		if len(args) < 4 || args[0] != "new" || args[1] != "xlsx" || args[2] != "Sales Analysis" {
			t.Fatalf("unexpected restart args: %v", args)
		}
		return nil
	}
	app.officeTaskPreflight = func(ctx context.Context, command string, args []string) error {
		preflightCalls++
		return nil
	}
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		licenseClientCalls++
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		llmClientCalls++
		return fakeAppLLMClient{jsonResponse: `{"title":"Quarterly Sales Analysis Workbook","sheets":[{"name":"Summary","headers":["Region","Revenue","Year-over-Year Growth","Owner","Target Attainment"],"rows":[["North America","2.4M","+12%","Avery","108%"],["Europe","1.8M","+9%","Jordan","101%"]]},{"name":"Regional Analysis","headers":["Region","Revenue","Year-over-Year Growth","Owner","Target Attainment"],"rows":[["APAC","1.5M","+15%","Taylor","112%"],["LATAM","0.9M","+7%","Morgan","97%"]]}]}`}, nil
	}

	if err := app.Run(t.Context(), []string{"new", "xlsx", "Sales Analysis", "--prompt", "Generate a quarterly sales analysis workbook with a summary sheet and a regional analysis sheet. Include region, revenue, year-over-year growth, owner, and target attainment with plausible demo data.", "--no-publish"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !updated || !restarted {
		t.Fatalf("updated=%t restarted=%t", updated, restarted)
	}
	if preflightCalls != 0 {
		t.Fatalf("preflightCalls = %d, want 0", preflightCalls)
	}
	if licenseClientCalls != 0 {
		t.Fatalf("licenseClientCalls = %d, want 0", licenseClientCalls)
	}
	if llmClientCalls != 0 {
		t.Fatalf("llmClientCalls = %d, want 0", llmClientCalls)
	}
	if entries, err := os.ReadDir(outputDir); err == nil && len(entries) > 0 {
		t.Fatalf("expected no generated files before restart handoff, found %d", len(entries))
	}
	if strings.Contains(stdout.String(), "Generation completed. Saved to") {
		t.Fatalf("parent process should not render generation output: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestAppRun_InteractiveUpdatePromptCanSkipUpdate(t *testing.T) {
	t.Setenv(updateCheckSkipEnv, "0")
	stdout := &terminalBuffer{}
	app := NewApp(stdout, bytes.NewBuffer(nil), &terminalInputBuffer{Reader: strings.NewReader("2\n")})
	originalVersion := Version
	originalBuildDate := BuildDate
	Version = "0.2.2"
	BuildDate = "2026-04-09T09:07:59Z"
	defer func() {
		Version = originalVersion
		BuildDate = originalBuildDate
	}()
	app.checkForUpdates = func(ctx context.Context) (UpdateInfo, error) {
		return UpdateInfo{
			Available:           true,
			CurrentVersion:      "0.2.2",
			LatestVersionLabel:  "0.2.6",
			InstallMethod:       InstallMethodScript,
			Channel:             UpdateChannelLatest,
			AutoUpdateSupported: true,
		}, nil
	}
	app.performUpdate = func(ctx context.Context, info UpdateInfo) error {
		t.Fatal("performUpdate should not be called")
		return nil
	}
	app.restartCommand = func(ctx context.Context, info UpdateInfo, args []string) error {
		t.Fatal("restartCommand should not be called")
		return nil
	}

	if err := app.Run(t.Context(), []string{"config", "status"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Update available for officecli") {
		t.Fatalf("stdout = %q", output)
	}
	if !strings.Contains(output, "2. Continue without updating") {
		t.Fatalf("stdout = %q", output)
	}
	if !strings.Contains(output, "Config file:") {
		t.Fatalf("expected command to continue, got %q", output)
	}
}

func TestAppRun_UpdateCheckSkipsJSONAndHelp(t *testing.T) {
	t.Setenv(updateCheckSkipEnv, "0")
	t.Setenv("OFFICE_CLI_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	originalVersion := Version
	originalBuildDate := BuildDate
	Version = "0.2.2"
	BuildDate = "2026-04-09T09:07:59Z"
	defer func() {
		Version = originalVersion
		BuildDate = originalBuildDate
	}()

	var stdout bytes.Buffer
	app := NewApp(&terminalBuffer{}, bytes.NewBuffer(nil), &terminalInputBuffer{Reader: strings.NewReader("1\n")})
	app.checkForUpdates = func(ctx context.Context) (UpdateInfo, error) {
		t.Fatal("checkForUpdates should not be called")
		return UpdateInfo{}, nil
	}

	if err := app.Run(t.Context(), []string{"--help"}); err != nil {
		t.Fatalf("Run(help): %v", err)
	}

	app = NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBufferString(""))
	app.checkForUpdates = func(ctx context.Context) (UpdateInfo, error) {
		t.Fatal("checkForUpdates should not be called")
		return UpdateInfo{}, nil
	}
	if err := app.Run(t.Context(), []string{"new", "docx", "Quarterly Retrospective", "--json", "--no-publish"}); err == nil {
		t.Fatal("expected config-related error for missing setup")
	}
}

func TestAppRun_HelpIncludesConfigCommand(t *testing.T) {
	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBuffer(nil))

	if err := app.Run(t.Context(), []string{"--help"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "config") {
		t.Fatalf("help output missing config command: %s", output)
	}
	if strings.Contains(output, "init") {
		t.Fatalf("help output should not expose init: %s", output)
	}
}

func TestAppRun_ConfigSetGenerationWritesConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "officecli.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)

	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBufferString("https://api.example.com/v1\nsk-test\nno\n"))

	if err := app.Run(t.Context(), []string{"config", "set-generation"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	for _, needle := range []string{"https://api.example.com/v1", "sk-test", "gpt-4.1"} {
		if !strings.Contains(content, needle) {
			t.Fatalf("config missing %q: %s", needle, content)
		}
	}
	if strings.Contains(content, "\"image_base_url\"") {
		t.Fatalf("config should reuse text generation service by default: %s", content)
	}
	if !strings.Contains(stdout.String(), "Updated generation service config") {
		t.Fatalf("stdout = %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "--no-images") {
		t.Fatalf("stdout should include image guidance: %s", stdout.String())
	}
}

func TestAppRun_ConfigSetGenerationWritesSeparateImageConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "officecli.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)

	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBufferString("https://api.example.com/v1\nsk-test\nyes\nhttps://img.example.com/v1\nimg-key\ngpt-image-1\n"))

	if err := app.Run(t.Context(), []string{"config", "set-generation"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	for _, needle := range []string{"https://img.example.com/v1", "img-key", "gpt-image-1"} {
		if !strings.Contains(content, needle) {
			t.Fatalf("config missing %q: %s", needle, content)
		}
	}
}

func TestAppRun_ConfigSetLicenseUsesFixedPlatformURL(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "officecli.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)

	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBufferString("yes\n\n"))

	if err := app.Run(t.Context(), []string{"config", "set-license"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "https://platform.officecli.io") {
		t.Fatalf("config = %s", content)
	}
	if strings.Contains(stdout.String(), "Enter the license service URL") {
		t.Fatalf("stdout should not prompt for license base url: %s", stdout.String())
	}
}

func TestAppRun_ConfigSetPublishWritesConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "officecli.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)

	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBufferString("yes\nhttps://publish.example.com/api\npub-token\n"))

	if err := app.Run(t.Context(), []string{"config", "set-publish"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	for _, needle := range []string{"https://publish.example.com/api", "pub-token"} {
		if !strings.Contains(content, needle) {
			t.Fatalf("config missing %q: %s", needle, content)
		}
	}
}

func TestAppRun_ConfigSetDefaultsWritesConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "officecli.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)

	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBufferString("./dist\n2\nyes\n"))

	if err := app.Run(t.Context(), []string{"config", "set-defaults"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	for _, needle := range []string{"./dist", "\"mode\": \"best\"", "\"publish\": true"} {
		if !strings.Contains(content, needle) {
			t.Fatalf("config missing %q: %s", needle, content)
		}
	}
	if !strings.Contains(stdout.String(), "Updated default config") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestAppRun_ConfigStatusShowsProductState(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "officecli.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)
	t.Setenv("OFFICE_CLI_LLM_IMAGE_BASE_URL", "")
	t.Setenv("OFFICE_CLI_LLM_IMAGE_API_KEY", "")
	t.Setenv("OFFICE_CLI_LLM_IMAGE_MODEL", "")
	_, err := WriteConfig("", Config{
		Defaults: DefaultsConfig{OutputDir: "./out", Mode: "best", Publish: true},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "sk-test", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://platform.officecli.io", Enabled: true},
		Publish:  publishprovider.Config{Enabled: true},
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	if err := app.Run(t.Context(), []string{"config", "status"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	output := stdout.String()
	for _, needle := range []string{"Config file:", "Generation service configured: true", "Image generation config: Not configured separately (reuses the generation service by default)", "Access checks enabled: true", "Default output directory: ./out", "Default generation mode: best", "Publish by default after generation: true"} {
		if !strings.Contains(output, needle) {
			t.Fatalf("status missing %q: %s", needle, output)
		}
	}
}

func TestAppRun_MissingGenerationConfigShowsConfigGuidance(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "missing.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr, bytes.NewBuffer(nil))

	err := app.Run(t.Context(), []string{"new", "pptx", "Enterprise Collaboration Platform Overview", "Introduce this enterprise collaboration platform"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "officecli config set-generation") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAppRun_NewStopsBeforeLLMWhenFreeQuotaExhausted(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)

	_, err := WriteConfig("", Config{
		LLM: LLMConfig{
			BaseURL: "https://api.example.com/v1",
			APIKey:  "llm-key",
			Model:   "gpt-4.1",
		},
		License: LicenseConfig{
			BaseURL:    "https://license.example.com/api",
			Enabled:    true,
			TimeoutSec: 60,
		},
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	var llmCalled bool
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{
			checkResult: &LicenseCheckResult{
				Allowed:       false,
				AccessMode:    LicenseAccessModeBlocked,
				FreeRemaining: 0,
				Message:       "Free quota is exhausted. Add license.api_key to the config file and try again.",
			},
		}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		llmCalled = true
		return fakeAppLLMClient{}, nil
	}

	err = app.Run(t.Context(), []string{"new", "pptx", "Topic"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Free quota is exhausted") {
		t.Fatalf("unexpected error: %v", err)
	}
	if llmCalled {
		t.Fatal("expected llm client init to be skipped")
	}
}

func TestAppRun_AuthSetKeyWritesConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)

	_, err := WriteConfig("", Config{
		License: LicenseConfig{
			BaseURL:    "https://license.example.com/api",
			Enabled:    true,
			TimeoutSec: 60,
		},
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{
			checkResult: &LicenseCheckResult{
				Allowed:    true,
				AccessMode: LicenseAccessModePaid,
				PlanName:   "pro",
			},
		}, nil
	}

	if err := app.Run(t.Context(), []string{"auth", "set-key", "sk-license"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "\"api_key\": \"sk-license\"") {
		t.Fatalf("config = %s", string(data))
	}
	if !strings.Contains(stdout.String(), "Saved the paid API key") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestAppRun_AuthSetKeyPromptsWhenArgMissing(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)

	_, err := WriteConfig("", Config{
		License: LicenseConfig{
			BaseURL:    "https://license.example.com/api",
			Enabled:    true,
			TimeoutSec: 60,
		},
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBufferString("sk-interactive\n"))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{
			checkResult: &LicenseCheckResult{
				Allowed:    true,
				AccessMode: LicenseAccessModePaid,
				PlanName:   "pro",
			},
		}, nil
	}

	if err := app.Run(t.Context(), []string{"auth", "set-key"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "\"api_key\": \"sk-interactive\"") {
		t.Fatalf("config = %s", string(data))
	}
	if !strings.Contains(stdout.String(), "Enter the paid API key") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestAppRun_AuthSetKeyValidationFailureKeepsOldConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)
	_, err := WriteConfig("", Config{
		License: LicenseConfig{
			BaseURL:    "https://license.example.com/api",
			APIKey:     "old-key",
			Enabled:    true,
			TimeoutSec: 60,
		},
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{
			checkResult: &LicenseCheckResult{
				Allowed:    false,
				AccessMode: LicenseAccessModeBlocked,
				Message:    "key invalid",
			},
		}, nil
	}
	err = app.Run(t.Context(), []string{"auth", "set-key", "bad-key"})
	if err == nil {
		t.Fatal("expected error")
	}
	data, _ := os.ReadFile(configPath)
	if !strings.Contains(string(data), "\"api_key\": \"old-key\"") {
		t.Fatalf("config = %s", string(data))
	}
}

func TestAppRun_AuthStatusShowsRemainingPaidQuota(t *testing.T) {
	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{
			checkResult: &LicenseCheckResult{
				Allowed:            true,
				AccessMode:         LicenseAccessModePaid,
				PlanName:           "pro",
				FreeLimit:          10,
				FreeUsed:           2,
				FreeRemaining:      8,
				RewardRemaining:    3,
				PaidQuotaRemaining: 8,
				PaidQuotaTotal:     12,
				PaidQuotaUsed:      4,
				QuotaSnapshot: &licenseprovider.QuotaSnapshot{
					FreeTrialDaily: licenseprovider.FreeTrialDailySnapshot{Limit: 10, Used: 2, Remaining: 8},
					RewardQuota:    licenseprovider.RewardQuotaSnapshot{Remaining: 3},
					PaidExternalQuota: licenseprovider.PaidExternalQuotaSnapshot{
						CurrentKeyPrefix:    "cop_live_demo",
						CurrentKeyTotal:     12,
						CurrentKeyUsed:      4,
						CurrentKeyRemaining: 8,
					},
				},
			},
		}, nil
	}

	if err := app.Run(t.Context(), []string{"auth", "status"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Current access mode: paid") || !strings.Contains(output, "Free trial today (this machine, UTC): 10 total / 2 used / 8 remaining") || !strings.Contains(output, "Reward quota remaining: 3") || !strings.Contains(output, "Paid quota on current key (cop_live_demo): 12 total / 4 used / 8 remaining") {
		t.Fatalf("stdout = %s", output)
	}
}

func TestAppRun_AuthStatusShowsDisabledWhenLicenseServiceNotEnabled(t *testing.T) {
	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return nil, nil
	}

	if err := app.Run(t.Context(), []string{"auth", "status"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Current access mode: disabled") {
		t.Fatalf("stdout = %s", output)
	}
	if strings.Contains(output, "Current access mode: paid") {
		t.Fatalf("stdout should not pretend to be paid: %s", output)
	}
}

func TestAppRun_AuthStatusShowsRemainingRewardQuota(t *testing.T) {
	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{
			checkResult: &LicenseCheckResult{
				Allowed:         true,
				AccessMode:      LicenseAccessModeReward,
				RewardRemaining: 5,
				QuotaSnapshot: &licenseprovider.QuotaSnapshot{
					FreeTrialDaily: licenseprovider.FreeTrialDailySnapshot{Limit: 10, Used: 0, Remaining: 10},
					RewardQuota:    licenseprovider.RewardQuotaSnapshot{Remaining: 5},
				},
			},
		}, nil
	}

	if err := app.Run(t.Context(), []string{"auth", "status"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Current access mode: reward") || !strings.Contains(output, "Reward quota remaining: 5") || !strings.Contains(output, "Free trial today (this machine, UTC): 10 total / 0 used / 10 remaining") {
		t.Fatalf("stdout = %s", output)
	}
}

func TestCheckLicensePaidQuotaExhaustedShowsPaidMessage(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{
			checkResult: &LicenseCheckResult{
				Allowed:    false,
				AccessMode: LicenseAccessModeBlocked,
				ReasonCode: "paid_quota_exhausted",
			},
		}, nil
	}

	_, err := app.checkLicense(t.Context(), LicenseConfig{Enabled: true, BaseURL: "https://license.example.com/api", APIKey: "paid-key"}, "pptx", "generate")
	if err == nil || !strings.Contains(err.Error(), "out of quota") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckLicenseRejectsTamperedReplayProof(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return dynamicLicenseManager{
			check: func(req LicenseCheckRequest) (*LicenseCheckResult, error) {
				token := signTestCommitToken(req, LicenseAccessModePaid, UsageCommitToken{})
				token.DocumentType = "docx"
				return &LicenseCheckResult{
					Allowed:     true,
					AccessMode:  LicenseAccessModePaid,
					CommitToken: token,
				}, nil
			},
		}, nil
	}

	_, err := app.checkLicense(t.Context(), LicenseConfig{Enabled: true, BaseURL: "https://license.example.com/api", APIKey: "paid-key"}, "pptx", "generate")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "license proof validation failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckLicenseAcceptsLegacyPlatformToken(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return dynamicLicenseManager{
			check: func(req LicenseCheckRequest) (*LicenseCheckResult, error) {
				return &LicenseCheckResult{
					Allowed:    true,
					AccessMode: LicenseAccessModePaid,
					CommitToken: UsageCommitToken{
						FingerprintHash: req.FingerprintHash,
						RequestID:       "req-legacy",
						AccessMode:      LicenseAccessModePaid,
						APIKeyHint:      "cop_live_DBq",
					},
				}, nil
			},
		}, nil
	}

	result, err := app.checkLicense(t.Context(), LicenseConfig{Enabled: true, BaseURL: "https://license.example.com/api", APIKey: "paid-key"}, "pptx", "generate")
	if err != nil {
		t.Fatalf("checkLicense() error = %v", err)
	}
	if result == nil || result.AccessMode != LicenseAccessModePaid {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestCheckLicenseRejectsExpiredProof(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return dynamicLicenseManager{
			check: func(req LicenseCheckRequest) (*LicenseCheckResult, error) {
				token := signTestCommitToken(req, LicenseAccessModePaid, UsageCommitToken{
					IssuedAt:  time.Now().UTC().Add(-10 * time.Minute),
					ExpiresAt: time.Now().UTC().Add(-8 * time.Minute),
				})
				return &LicenseCheckResult{
					Allowed:     true,
					AccessMode:  LicenseAccessModePaid,
					CommitToken: token,
				}, nil
			},
		}, nil
	}

	_, err := app.checkLicense(t.Context(), LicenseConfig{Enabled: true, BaseURL: "https://license.example.com/api", APIKey: "paid-key"}, "pptx", "generate")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "license proof validation failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckLicenseOfflineWithoutPaidKeyReturnsOriginalError(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkErr: context.DeadlineExceeded}, nil
	}

	_, err := app.checkLicense(t.Context(), LicenseConfig{Enabled: true, BaseURL: "https://license.example.com/api"}, "pptx", "status")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckLicenseOfflineWithPaidKeyRequiresOnlineValidation(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkErr: context.DeadlineExceeded}, nil
	}

	_, err := app.checkLicense(t.Context(), LicenseConfig{Enabled: true, BaseURL: "https://license.example.com/api", APIKey: "paid-key"}, "pptx", "status")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Paid access requires online validation") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteGenerateJob_BestExternalChecksStatusBeforeFollowUp(t *testing.T) {
	events := []string{}
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return &orderedLicenseManager{
			events:   &events,
			checkErr: fmt.Errorf("quota exhausted"),
		}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return &sequencedAppLLMClient{
			structuredResponses: []string{
				`{"questions":[{"id":"audience","question":"Who is the main audience for this deck?","allowFreeform":true,"options":[{"id":"client","label":"Client or partner","description":"Emphasize value and persuasion.","recommended":true},{"id":"team","label":"Internal team","description":"Emphasize process and execution."}]}]}`,
				`{"plan_markdown":"# Execution Plan\n\n## Summary\n- Lead with client value.\n\n## Framework Blueprint\n- Keep the deck concise and persuasive.","execution_prompt":"Generate a concise, persuasive client deck that leads with value and keeps the storyline tight."}`,
			},
			jsonResponses: []string{
				`{"presentationType":"Product introduction deck","targetAudience":"Client or partner","presentationPurpose":"Introduce the product and persuade the audience","pageCount":6,"contentStyle":"Concise and persuasive","visualEffect":"Professional and clean","contentGuideline":"Keep each slide focused on one buyer-relevant point","slideOutline":[{"slideIndex":1,"purpose":"Cover","suggestedLayout":"title","contentFormat":"paragraph","maxItems":1,"contentRequirements":"State the product and audience value","visualSuggestion":"hero"}]}`,
			},
		}, nil
	}

	_, err := app.executeGenerateJob(t.Context(), Config{
		LLM: LLMConfig{
			BaseURL: "https://api.example.com/v1",
			APIKey:  "llm-key",
			Model:   "gpt-4.1",
		},
		License: LicenseConfig{
			BaseURL: "https://license.example.com/api",
			APIKey:  "paid-key",
			Enabled: true,
		},
		Publish: disabledPublishConfig(),
	}, GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "minecraft 游戏介绍",
		Prompt:       "介绍 minecraft 这款游戏",
		RuntimeMode:  RuntimeModeExternal,
		Mode:         "best",
		OutputDir:    t.TempDir(),
	}, true, noopProgressController{}, &recordingPrompter{
		events:    &events,
		optionIDs: []string{"1"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "quota exhausted") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(events, ",") != "check" {
		t.Fatalf("events = %v", events)
	}
}

func TestExecuteGenerateJob_FastEnrichesShortPPTPrompt(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	llm := &sequencedAppLLMClient{
		structuredResponses: []string{
			`{"prompt":"请生成一份面向第一次接触 Minecraft 的观众的英文介绍型 PPT，先解释 Minecraft 是什么，再介绍核心玩法、为什么它有高自由度和高可玩性，并在结尾给出新手如何开始的建议。控制在 6 页左右，每页标题尽量短，避免卡片和说明文字过密。","assumptions":["面向新手观众","以入门介绍为主"]}`,
		},
		jsonResponses: []string{
			`{
				"title":"Minecraft Introduction",
				"subtitle":"What it is, why it stands out, and how to begin",
				"stylePreset":"tech-contrast",
				"theme":{"preset":"analysis","primaryColor":"1D4ED8","accentColor":"0F766E","accentSoft":"D1FAE5","backgroundColor":"F8FAFC","surfaceColor":"FFFFFF","borderColor":"DCE4F2","textColor":"0F172A","mutedColor":"64748B","titleColor":"020617","fontFamily":"Aptos","eaFontFamily":"Microsoft YaHei"},
				"slides":[
					{"role":"cover","layout":"title","headline":"Minecraft Introduction","takeaway":"A beginner-friendly guide","blocks":[],"visual":null,"source":"","bgColor":"","bgColor2":""},
					{"role":"summary","layout":"content","headline":"What It Is","takeaway":"Minecraft is a sandbox game built around exploration, building, and survival","blocks":[{"type":"sections","text":"","items":[],"sections":[{"heading":"Sandbox","detail":"Players shape their own goals in an open world"},{"heading":"Build","detail":"Blocks become houses, tools, and large creations"},{"heading":"Survive","detail":"Resources, crafting, and danger create a simple but engaging loop"}],"metrics":[],"chart":null}],"visual":null,"source":"","bgColor":"","bgColor2":""}
				]
			}`,
		},
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return llm, nil
	}
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}

	result, err := app.executeGenerateJob(t.Context(), Config{
		LLM: LLMConfig{
			BaseURL: "https://api.example.com/v1",
			APIKey:  "llm-key",
			Model:   "gpt-4.1",
		},
		License: LicenseConfig{
			BaseURL: "https://license.example.com/api",
			APIKey:  "paid-key",
			Enabled: true,
		},
		Publish: disabledPublishConfig(),
	}, GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "minecraft 游戏介绍",
		Prompt:       "介绍 minecraft 这款游戏",
		RuntimeMode:  RuntimeModeExternal,
		Mode:         "fast",
		Language:     "en-US",
		OutputDir:    t.TempDir(),
	}, false, noopProgressController{}, nil)
	if err != nil {
		t.Fatalf("executeGenerateJob: %v", err)
	}
	if llm.structuredCalls != 1 {
		t.Fatalf("structuredCalls = %d, want 1", llm.structuredCalls)
	}
	if llm.jsonCalls != 1 {
		t.Fatalf("jsonCalls = %d, want 1", llm.jsonCalls)
	}
	if llm.lastStructuredReq.Schema.Name != "ppt_prompt_enrichment" {
		t.Fatalf("schema name = %q", llm.lastStructuredReq.Schema.Name)
	}
	warnings := strings.Join(result.Warnings, "\n")
	if !strings.Contains(warnings, "automatically expanded") {
		t.Fatalf("warnings = %q, want prompt enrichment warning", warnings)
	}
}

func TestExecuteGenerateJob_BestHostedChecksAccessBeforeFollowUp(t *testing.T) {
	events := []string{}
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return &orderedLicenseManager{
			events:   &events,
			checkErr: fmt.Errorf("hosted credits exhausted"),
		}, nil
	}

	_, err := app.executeGenerateJob(t.Context(), Config{
		License: LicenseConfig{
			BaseURL: "https://platform.officecli.io",
			APIKey:  "paid-key",
			Enabled: true,
		},
		Publish: disabledPublishConfig(),
	}, GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "minecraft 游戏介绍",
		Prompt:       "介绍 minecraft 这款游戏",
		RuntimeMode:  RuntimeModeHosted,
		Mode:         "best",
		OutputDir:    t.TempDir(),
	}, true, noopProgressController{}, &recordingPrompter{
		events:    &events,
		optionIDs: []string{"1"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "hosted credits exhausted") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(events, ",") != "check" {
		t.Fatalf("events = %v", events)
	}
}

func TestCheckLicenseWhenDisabledReturnsBypassMessage(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return nil, nil
	}

	result, err := app.checkLicense(t.Context(), LicenseConfig{Enabled: false}, "pptx", "status")
	if err != nil {
		t.Fatalf("checkLicense: %v", err)
	}
	if !result.Allowed || result.AccessMode != LicenseAccessModeDisabled {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !strings.Contains(result.Message, "Access checks are not configured") {
		t.Fatalf("unexpected message: %+v", result)
	}
}

func TestAppRun_AuthStatusShowsRemainingFreeQuota(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)
	_, err := WriteConfig("", Config{
		License: LicenseConfig{
			BaseURL:    "https://license.example.com/api",
			Enabled:    true,
			TimeoutSec: 60,
		},
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	var stdout bytes.Buffer
	app := NewApp(&stdout, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{
			checkResult: &LicenseCheckResult{
				Allowed:       true,
				AccessMode:    LicenseAccessModeFree,
				FreeLimit:     5,
				FreeUsed:      2,
				FreeRemaining: 3,
				Message:       "Current mode: free.",
				QuotaSnapshot: &licenseprovider.QuotaSnapshot{
					FreeTrialDaily: licenseprovider.FreeTrialDailySnapshot{Limit: 5, Used: 2, Remaining: 3},
				},
			},
		}, nil
	}

	if err := app.Run(t.Context(), []string{"auth", "status"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Current access mode: free") {
		t.Fatalf("stdout = %s", output)
	}
	if !strings.Contains(output, "Free trial today (this machine, UTC): 5 total / 2 used / 3 remaining") {
		t.Fatalf("stdout = %s", output)
	}
	if !strings.Contains(output, "Access checks enabled: true") {
		t.Fatalf("stdout = %s", output)
	}
	if !strings.Contains(output, "Paid API key configured: false") {
		t.Fatalf("stdout = %s", output)
	}
}
