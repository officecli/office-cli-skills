package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
		"new <pptx|docx|xlsx|report> <topic> [brief]",
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
		{args: []string{"new", "--help"}, needles: []string{"officecli new <pptx|docx|xlsx|report>", "--prompt-file", "--mode fast|best", "--file <path>", "automatic PPT images", "officecli config set-generation", "requires `--file <xlsx-path>`"}},
		{args: []string{"new", "pptx", "--help"}, needles: []string{"officecli new <pptx|docx|xlsx|report>", "--prompt-file", "--mode fast|best"}},
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
