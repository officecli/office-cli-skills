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
	publishprovider "github.com/officecli/officecli/internal/providers/publish"
)

type stubLicenseManager struct {
	checkResult *LicenseCheckResult
	checkErr    error
}

func TestMain(m *testing.M) {
	_ = os.Setenv(officeTaskPreflightSkipEnv, "1")
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

func TestRenderProgress_TTYAnimatesAndFinalizesStage(t *testing.T) {
	oldInterval := spinnerFrameInterval
	spinnerFrameInterval = 5 * time.Millisecond
	defer func() { spinnerFrameInterval = oldInterval }()

	out := &terminalBuffer{}
	renderer := NewProgressRenderer(out, false, true)
	renderer.Emit(context.Background(), engine.ProgressEvent{Step: "generate", Status: "running", Content: "正在生成文档内容"})
	time.Sleep(30 * time.Millisecond)
	renderer.Emit(context.Background(), engine.ProgressEvent{Step: "generate", Status: "completed", Content: "已生成文档内容"})

	raw := out.String()
	if !strings.Contains(raw, "⠋") || !strings.Contains(raw, "⠙") {
		t.Fatalf("expected multiple spinner frames, got %q", raw)
	}
	history := strings.Join(out.History(), "\n")
	if !strings.Contains(history, "✔ 已生成文档内容") {
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
		return fakeAppLLMClient{jsonResponse: `{"title":"企业协作平台介绍","sections":[{"heading":"产品概述","level":1,"paragraphs":["这是一款面向企业的协作平台产品。"]}]}`}, nil
	}

	if err := app.Run(t.Context(), []string{"new", "docx", "企业协作平台介绍", "介绍这款企业协作平台", "--json", "--no-publish"}); err != nil {
		t.Fatalf("Run(new): %v", err)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("expected preflight marker: %v", err)
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
		return fakeAppLLMClient{jsonResponse: `{"title":"企业协作平台介绍","sections":[{"heading":"产品概述","level":1,"paragraphs":["这是一款面向企业的协作平台产品。"]}]}`}, nil
	}

	if err := app.Run(t.Context(), []string{"new", "docx", "企业协作平台介绍", "--json", "--no-publish"}); err != nil {
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
		return fakeAppLLMClient{jsonResponse: `{"title":"企业协作平台介绍","sections":[{"heading":"产品概述","level":1,"paragraphs":["这是一款面向企业的协作平台产品。"]}]}`}, nil
	}

	if err := app.Run(t.Context(), []string{"new", "docx", "企业协作平台介绍", "--json", "--no-publish"}); err != nil {
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
	err = app.Run(t.Context(), []string{"new", "docx", "企业协作平台介绍", "--json", "--no-publish"})
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

	err = app.Run(t.Context(), []string{"new", "docx", "企业协作平台介绍", "介绍这款企业协作平台", "--json", "--no-publish"})
	if err == nil {
		t.Fatal("expected llm failure")
	}
	if !strings.Contains(err.Error(), "生成内容阶段失败：生成内容阶段失败：llm request failed: invalid json response body=<html><body>bad gateway</body></html>") {
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
		return fakeAppLLMClient{jsonResponse: `{"title":"企业协作平台介绍","sections":[{"heading":"产品概述","level":1,"paragraphs":["这是一款面向企业的协作平台产品。"]}]}`}, nil
	}

	if err := app.Run(t.Context(), []string{"new", "docx", "企业协作平台介绍", "--no-publish"}); err != nil {
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
		return fakeAppLLMClient{jsonResponse: `{"title":"企业协作平台介绍","sections":[{"heading":"产品概述","level":1,"paragraphs":["这是一款面向企业的协作平台产品。"]}]}`}, nil
	}

	if err := app.Run(t.Context(), []string{"new", "docx", "企业协作平台介绍", "--json", "--no-publish"}); err != nil {
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
	renderer.Emit(context.Background(), engine.ProgressEvent{Step: "generate", Status: "running", Content: "这是一个非常非常长的阶段文案"})
	time.Sleep(10 * time.Millisecond)
	renderer.Emit(context.Background(), engine.ProgressEvent{Step: "generate", Status: "running", Content: "短文案"})
	time.Sleep(10 * time.Millisecond)
	renderer.Emit(context.Background(), engine.ProgressEvent{Step: "generate", Status: "completed", Content: "已完成短文案"})

	history := strings.Join(out.History(), "\n")
	if strings.Contains(history, "短文案常") || strings.Contains(history, "短文案阶段") {
		t.Fatalf("expected trailing characters to be cleared, got %q", history)
	}
	if !strings.Contains(history, "✔ 已完成短文案") {
		t.Fatalf("expected completed short line, got %q", history)
	}
}

func TestRenderProgress_PauseStopsSpinnerAndPrintsWaitingLine(t *testing.T) {
	oldInterval := spinnerFrameInterval
	spinnerFrameInterval = 5 * time.Millisecond
	defer func() { spinnerFrameInterval = oldInterval }()

	out := &terminalBuffer{}
	renderer := NewProgressRenderer(out, false, true)
	renderer.Emit(context.Background(), engine.ProgressEvent{Step: "question", Status: "running", Content: "正在等待你回答补充问题"})
	time.Sleep(15 * time.Millisecond)
	renderer.Pause("等待你输入答案")
	rawAfterPause := out.String()
	time.Sleep(20 * time.Millisecond)

	if out.String() != rawAfterPause {
		t.Fatalf("expected spinner to stop after pause")
	}
	history := strings.Join(out.History(), "\n")
	if !strings.Contains(history, "… 等待你输入答案") {
		t.Fatalf("expected waiting line after pause, got %q", history)
	}
}

func TestRenderProgress_NonTTYPrintsStageLines(t *testing.T) {
	var out bytes.Buffer
	renderer := NewProgressRenderer(&out, false, false)
	renderer.Emit(context.Background(), engine.ProgressEvent{Step: "license", Status: "running", Content: "正在校验授权", ElapsedMs: 5})
	renderer.Emit(context.Background(), engine.ProgressEvent{Step: "generate", Status: "running", Content: "正在生成文档内容", ElapsedMs: 15})

	output := out.String()
	if strings.Contains(output, "%") {
		t.Fatalf("progress output should not contain percent: %s", output)
	}
	for _, needle := range []string{"正在校验授权", "正在生成文档内容"} {
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
		return fakeAppLLMClient{jsonResponse: `{"title":"企业协作平台介绍","theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},"slides":[{"title":"企业协作平台介绍","layout":"title","subtitle":"产品和企业状况","isTitle":true},{"title":"产品能力","layout":"content","points":["多人协作","实时编辑","企业管理"]}]}`, delay: 25 * time.Millisecond}, nil
	}

	err = app.Run(t.Context(), []string{"new", "pptx", "企业协作平台介绍", "介绍这款企业协作平台的产品能力、客户价值与应用场景", "--no-publish"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := stdout.String()
	for _, needle := range []string{"正在校验授权", "正在生成文档内容", "正在写入本地文件", "生成完成！已保存至"} {
		if !strings.Contains(output, needle) {
			t.Fatalf("stdout missing %q: %s", needle, output)
		}
	}
	if strings.Index(output, "正在生成文档内容") > strings.Index(output, "生成完成！已保存至") {
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
		return fakeAppLLMClient{jsonResponse: `{"title":"企业协作平台介绍","theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},"slides":[{"title":"企业协作平台介绍","layout":"title","subtitle":"产品和企业状况","isTitle":true},{"title":"产品能力","layout":"content","points":["多人协作","实时编辑","企业管理"]}]}`, delay: 25 * time.Millisecond}, nil
	}

	err = app.Run(t.Context(), []string{"new", "pptx", "企业协作平台介绍", "介绍这款企业协作平台的产品能力、客户价值与应用场景", "--no-publish"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	raw := stdout.String()
	if !strings.Contains(raw, "⠋") || !strings.Contains(raw, "⠙") {
		t.Fatalf("expected spinner frames in tty output, got %q", raw)
	}
	history := strings.Join(stdout.History(), "\n")
	for _, needle := range []string{"✔ 授权校验完成", "✔ 文档已生成", "生成完成！已保存至"} {
		if !strings.Contains(history, needle) {
			t.Fatalf("tty history missing %q: %s", needle, history)
		}
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
		return fakeAppLLMClient{jsonResponse: `{"title":"企业协作平台介绍","sections":[{"heading":"产品概述","level":1,"paragraphs":["这是一款面向企业的协作平台产品。"]}]}`}, nil
	}

	err = app.Run(t.Context(), []string{"new", "docx", "企业协作平台介绍", "介绍这款企业协作平台", "--json", "--no-publish"})
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
		"位置标题",
		"位置描述",
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
	if job.Topic != "位置标题" {
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
		"文档标题",
		"位置描述",
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
		"报表",
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
		"带图演示",
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
		"带图演示",
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
		"董事会汇报",
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
		"config                  查看或更新本地配置",
		"auth                    查看或设置授权信息",
		"score                   按需开启本地 PPTX 评分",
		"new <pptx|docx|xlsx> <topic> [brief]",
		"officecli config status",
		"officecli score --help",
		"officecli auth --help",
		"officecli new --help",
		"子命令：",
		"默认行为：",
		"配置文件：",
		"macOS   ~/Library/Application Support/officecli/config.json",
		"Linux   ~/.config/officecli/config.json",
		"officecli new pptx \"企业协作平台介绍\" \"介绍这款企业协作平台的产品能力、客户价值与应用场景\"",
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
		{args: []string{"config", "--help"}, needles: []string{"用法：", "officecli config status", "officecli config set-generation", "officecli config set-license"}},
		{args: []string{"auth", "--help"}, needles: []string{"officecli auth status", "officecli auth set-key", "查看额度状态"}},
		{args: []string{"score", "--help"}, needles: []string{"officecli score pptx <file>", "评分默认不会在生成后自动执行"}},
		{args: []string{"new", "--help"}, needles: []string{"officecli new <pptx|docx|xlsx>", "--prompt-file", "--mode fast|best", "默认会尝试自动配图", "officecli config set-generation"}},
		{args: []string{"new", "pptx", "--help"}, needles: []string{"officecli new <pptx|docx|xlsx>", "--prompt-file", "--mode fast|best"}},
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
	if !strings.Contains(stdout.String(), "已更新生成服务配置") {
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
	if strings.Contains(stdout.String(), "请输入额度服务地址") {
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
	if !strings.Contains(stdout.String(), "已更新默认配置") {
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
	for _, needle := range []string{"配置文件路径：", "生成服务已配置：true", "图片生成配置：未单独配置（默认复用生成服务）", "额度校验已启用：true", "默认输出目录：./out", "默认生成模式：best", "生成后默认发布：true"} {
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

	err := app.Run(t.Context(), []string{"new", "pptx", "企业协作平台介绍", "介绍这款企业协作平台"})
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
				Message:       "免费额度已用完，请在配置文件中填写 license.api_key 后重试。",
			},
		}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		llmCalled = true
		return fakeAppLLMClient{}, nil
	}

	err = app.Run(t.Context(), []string{"new", "pptx", "主题"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "免费额度已用完") {
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
	if !strings.Contains(stdout.String(), "已写入付费额度密钥") {
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
	if !strings.Contains(stdout.String(), "请输入付费额度密钥") {
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
				PaidQuotaRemaining: 8,
			},
		}, nil
	}

	if err := app.Run(t.Context(), []string{"auth", "status"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "当前授权状态：paid") || !strings.Contains(output, "剩余付费次数：8") {
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
	if !strings.Contains(output, "当前授权状态：未启用") {
		t.Fatalf("stdout = %s", output)
	}
	if strings.Contains(output, "当前授权状态：paid") {
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
			},
		}, nil
	}

	if err := app.Run(t.Context(), []string{"auth", "status"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "当前授权状态：reward") || !strings.Contains(output, "剩余奖励次数：5") {
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
	if err == nil || !strings.Contains(err.Error(), "次数已耗尽") {
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
	if !strings.Contains(err.Error(), "license proof 校验失败") {
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
	if !strings.Contains(err.Error(), "license proof 校验失败") {
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
	if !strings.Contains(err.Error(), "当前付费模式要求在线校验") {
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
	if !strings.Contains(result.Message, "未接入额度校验服务") {
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
				FreeRemaining: 3,
				Message:       "当前为免费模式",
			},
		}, nil
	}

	if err := app.Run(t.Context(), []string{"auth", "status"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "当前授权状态：free") {
		t.Fatalf("stdout = %s", output)
	}
	if !strings.Contains(output, "剩余免费次数：3") {
		t.Fatalf("stdout = %s", output)
	}
	if !strings.Contains(output, "额度校验已启用：true") {
		t.Fatalf("stdout = %s", output)
	}
	if !strings.Contains(output, "付费额度密钥已配置：false") {
		t.Fatalf("stdout = %s", output)
	}
}
