package cli

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/officecli/officecli/engine"
	licenseprovider "github.com/officecli/officecli/internal/license"
)

type fakeGenerator struct{}

func (fakeGenerator) Generate(_ context.Context, params GenerateParams) (*GeneratedArtifact, error) {
	return &GeneratedArtifact{
		DocumentName: params.Topic + ".pptx",
		DocumentType: string(params.DocumentType),
		Bytes:        []byte("pptx-bytes"),
		Warnings:     []engine.GenerateIssue{{Code: "WARN_DEMO", Message: "demo warning"}},
	}, nil
}

type hostedCreditGenerator struct{}

func (hostedCreditGenerator) Generate(_ context.Context, params GenerateParams) (*GeneratedArtifact, error) {
	return &GeneratedArtifact{
		DocumentName:         params.Topic + ".pptx",
		DocumentType:         string(params.DocumentType),
		Bytes:                []byte("pptx-bytes"),
		HostedCreditBalance:  cliIntPtr(1100230),
		HostedCreditsCharged: cliIntPtr(14),
	}, nil
}

type failingGenerator struct {
	err error
}

func (f failingGenerator) Generate(_ context.Context, _ GenerateParams) (*GeneratedArtifact, error) {
	return nil, f.err
}

type previewGenerator struct{}

func (previewGenerator) Generate(_ context.Context, params GenerateParams) (*GeneratedArtifact, error) {
	return &GeneratedArtifact{
		DocumentName: params.Topic + ".pptx",
		DocumentType: string(params.DocumentType),
		Bytes:        []byte("pptx-bytes"),
		PreviewHTML:  []byte("<html>preview</html>"),
		PreviewJSON:  []byte(`{"title":"preview"}`),
	}, nil
}

type htmlGenerator struct{}

func (htmlGenerator) Generate(_ context.Context, params GenerateParams) (*GeneratedArtifact, error) {
	return &GeneratedArtifact{
		DocumentName: params.Topic + ".html",
		DocumentType: string(params.DocumentType),
		Bytes:        []byte("<html><body>report</body></html>"),
	}, nil
}

type fakePublisher struct{}

func (fakePublisher) Publish(_ context.Context, req PublishRequest) (*PublishResult, error) {
	return &PublishResult{
		AccessURL: "https://example.com/preview/123",
		Password:  "123456",
		FileID:    "file-123",
		ExpiresAt: "2026-04-04T00:00:00Z",
	}, nil
}

type failingPublisher struct {
	err error
}

func (f failingPublisher) Publish(_ context.Context, _ PublishRequest) (*PublishResult, error) {
	return nil, f.err
}

type fakeLicenseManager struct {
	consumeCalls int
	consumed     UsageCommitToken
	consumeErr   error
	consumeResp  *UsageConsumeResult
}

type progressCollector struct {
	events []engine.ProgressEvent
}

func (c *progressCollector) Emit(_ context.Context, event engine.ProgressEvent) {
	c.events = append(c.events, event)
}

func (f *fakeLicenseManager) Check(_ context.Context, req LicenseCheckRequest) (*LicenseCheckResult, error) {
	return &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}, nil
}

func (f *fakeLicenseManager) Consume(_ context.Context, token UsageCommitToken) (*UsageConsumeResult, error) {
	f.consumeCalls++
	f.consumed = token
	if f.consumeErr != nil {
		return nil, f.consumeErr
	}
	if f.consumeResp != nil {
		return f.consumeResp, nil
	}
	return &UsageConsumeResult{AccessMode: LicenseAccessModePaid, Remaining: 2}, nil
}

func cliIntPtr(value int) *int {
	return &value
}

func TestExecutorGenerateAndPublish(t *testing.T) {
	tmpDir := t.TempDir()
	executor := NewExecutor(fakeGenerator{}, fakePublisher{}, nil)

	result, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Enterprise Collaboration Platform Overview",
		Prompt:       "Describe the product capabilities, customer value, and use cases of this collaboration platform.",
		OutputDir:    tmpDir,
		Publish:      true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !result.Published {
		t.Fatal("expected published result")
	}
	if _, err := os.Stat(result.FilePath); err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if result.AccessURL == "" || result.Password == "" || result.ExpiresAt == "" {
		t.Fatalf("unexpected publish result: %+v", result)
	}
}

func TestExecutorPropagatesHostedCreditArtifactFields(t *testing.T) {
	tmpDir := t.TempDir()
	executor := NewExecutor(hostedCreditGenerator{}, fakePublisher{}, nil)

	result, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Product Launch",
		OutputDir:    tmpDir,
		Publish:      false,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.CreditsCharged != 14 {
		t.Fatalf("credits charged = %d, want 14", result.CreditsCharged)
	}
	if result.CreditBalance != 1100230 {
		t.Fatalf("credit balance = %d, want 1100230", result.CreditBalance)
	}
	if result.AccessMode != string(LicenseAccessModeHosted) || !result.HostedEnabled {
		t.Fatalf("hosted result metadata = access_mode %q hosted_enabled %v", result.AccessMode, result.HostedEnabled)
	}
}

func TestExecutorWritesLocalPreviewSidecars(t *testing.T) {
	tmpDir := t.TempDir()
	executor := NewExecutor(previewGenerator{}, fakePublisher{}, nil)

	result, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Enterprise Collaboration Platform Overview",
		Prompt:       "Describe the product capabilities, customer value, and use cases of this collaboration platform.",
		OutputDir:    tmpDir,
		Publish:      false,
		LocalPreview: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.LocalPreviewPath == "" || result.LocalPreviewDataPath == "" {
		t.Fatalf("preview paths = %+v", result)
	}
	if _, err := os.Stat(result.LocalPreviewPath); err != nil {
		t.Fatalf("stat preview html: %v", err)
	}
	if _, err := os.Stat(result.LocalPreviewDataPath); err != nil {
		t.Fatalf("stat preview json: %v", err)
	}
	// No temp orphans should remain after a successful atomic write.
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.Contains(name, ".tmp-") || strings.HasSuffix(name, ".tmp") {
			t.Fatalf("orphan temp file remains: %s", name)
		}
	}
}

// docPreviewGenerator emits a sidecar-shaped artifact for any document type.
// Used to confirm the executor enforces the preview allowlist regardless of
// what the generator returns.
type docPreviewGenerator struct {
	ext string
}

func (g docPreviewGenerator) Generate(_ context.Context, params GenerateParams) (*GeneratedArtifact, error) {
	return &GeneratedArtifact{
		DocumentName: params.Topic + "." + g.ext,
		DocumentType: string(params.DocumentType),
		Bytes:        []byte("primary-bytes"),
		PreviewHTML:  []byte("<html>preview</html>"),
		PreviewJSON:  []byte(`{"title":"preview"}`),
	}, nil
}

func TestExecutorSkipsPreviewSidecarsForNonAllowlistedTypes(t *testing.T) {
	cases := []struct {
		name string
		dt   engine.DocumentType
		ext  string
	}{
		{"img", engine.DocumentTypeIMG, "png"},
		{"report", engine.DocumentTypeReport, "html"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			executor := NewExecutor(docPreviewGenerator{ext: tc.ext}, fakePublisher{}, nil)

			result, err := executor.Run(context.Background(), GenerateJob{
				DocumentType: tc.dt,
				Topic:        "Preview Test",
				OutputDir:    tmpDir,
				LocalPreview: true,
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if result.LocalPreviewPath != "" {
				t.Fatalf("expected no LocalPreviewPath for %s, got %q", tc.dt, result.LocalPreviewPath)
			}
			if result.LocalPreviewDataPath != "" {
				t.Fatalf("expected no LocalPreviewDataPath for %s, got %q", tc.dt, result.LocalPreviewDataPath)
			}
			entries, err := os.ReadDir(tmpDir)
			if err != nil {
				t.Fatalf("ReadDir: %v", err)
			}
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".preview.html") || strings.HasSuffix(entry.Name(), ".preview.json") {
					t.Fatalf("unexpected sidecar emitted for %s: %s", tc.dt, entry.Name())
				}
			}
		})
	}
}

func TestExecutorWritesPreviewSidecarsForDOCXAndXLSX(t *testing.T) {
	cases := []struct {
		name string
		dt   engine.DocumentType
		ext  string
	}{
		{"docx", engine.DocumentTypeDOCX, "docx"},
		{"xlsx", engine.DocumentTypeXLSX, "xlsx"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			executor := NewExecutor(docPreviewGenerator{ext: tc.ext}, fakePublisher{}, nil)

			result, err := executor.Run(context.Background(), GenerateJob{
				DocumentType: tc.dt,
				Topic:        "Preview Allowlist",
				OutputDir:    tmpDir,
				LocalPreview: true,
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if result.LocalPreviewPath == "" || result.LocalPreviewDataPath == "" {
				t.Fatalf("expected sidecar paths for %s, got %+v", tc.dt, result)
			}
			if _, err := os.Stat(result.LocalPreviewPath); err != nil {
				t.Fatalf("stat sidecar html: %v", err)
			}
			if _, err := os.Stat(result.LocalPreviewDataPath); err != nil {
				t.Fatalf("stat sidecar json: %v", err)
			}
		})
	}
}

func TestExecutorPublishesReport(t *testing.T) {
	tmpDir := t.TempDir()
	executor := NewExecutor(htmlGenerator{}, fakePublisher{}, nil)

	result, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypeReport,
		Topic:        "Q2 Business Review",
		Prompt:       "Create a business review Report.",
		OutputDir:    tmpDir,
		Publish:      true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Published {
		t.Fatalf("expected report publish to succeed: %+v", result)
	}
	if _, err := os.Stat(result.FilePath); err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if result.AccessURL == "" || result.Password == "" || result.ExpiresAt == "" {
		t.Fatalf("unexpected publish result: %+v", result)
	}
}

func TestExecutorConsumesUsageAfterSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	manager := &fakeLicenseManager{}
	executor := NewExecutor(fakeGenerator{}, fakePublisher{}, manager)

	result, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Enterprise Collaboration Platform Overview",
		Prompt:       "Describe the product capabilities, customer value, and use cases of this collaboration platform.",
		OutputDir:    tmpDir,
		Publish:      false,
		LicenseCheck: &LicenseCheckResult{
			Allowed:    true,
			AccessMode: LicenseAccessModePaid,
			CommitToken: UsageCommitToken{
				FingerprintHash: "fp",
				RequestID:       "req-1",
			},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if manager.consumeCalls != 1 {
		t.Fatalf("consumeCalls = %d", manager.consumeCalls)
	}
	if manager.consumed.RequestID != "req-1" {
		t.Fatalf("consumed = %+v", manager.consumed)
	}
	if _, err := os.Stat(result.FilePath); err != nil {
		t.Fatalf("stat file: %v", err)
	}
}

func TestExecutorEmitsProgressEvents(t *testing.T) {
	tmpDir := t.TempDir()
	collector := &progressCollector{}
	executor := NewExecutor(fakeGenerator{}, fakePublisher{}, nil)
	executor.progress = collector

	_, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Enterprise Collaboration Platform Overview",
		Prompt:       "Describe the product capabilities, customer value, and use cases of this collaboration platform.",
		OutputDir:    tmpDir,
		Publish:      true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	joined := make([]string, 0, len(collector.events))
	for _, event := range collector.events {
		joined = append(joined, event.Step+":"+event.Status+":"+event.Content)
	}
	output := strings.Join(joined, "\n")
	for _, needle := range []string{
		"generate:running:Generating document content",
		"write_file:running:Writing local files",
		"publish:running:Publishing online preview",
		"finalize:completed:Document generated",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("progress output missing %q:\n%s", needle, output)
		}
	}
	if len(collector.events) < 4 {
		t.Fatalf("expected multiple progress events, got %d", len(collector.events))
	}
}

func TestExecutorFailsWhenConsumeFails(t *testing.T) {
	tmpDir := t.TempDir()
	manager := &fakeLicenseManager{consumeErr: context.DeadlineExceeded}
	executor := NewExecutor(fakeGenerator{}, fakePublisher{}, manager)

	_, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Enterprise Collaboration Platform Overview",
		Prompt:       "Describe the product capabilities, customer value, and use cases of this collaboration platform.",
		OutputDir:    tmpDir,
		Publish:      false,
		LicenseCheck: &LicenseCheckResult{
			Allowed:    true,
			AccessMode: LicenseAccessModePaid,
			CommitToken: UsageCommitToken{
				FingerprintHash: "fp",
				RequestID:       "req-1",
			},
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "quota sync failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries, statErr := os.ReadDir(tmpDir); statErr != nil {
		t.Fatalf("ReadDir: %v", statErr)
	} else if len(entries) != 0 {
		t.Fatalf("expected no output files, got %d", len(entries))
	}
}

func TestExecutorAddsFreeModeRemainingWarningAfterConsume(t *testing.T) {
	tmpDir := t.TempDir()
	manager := &fakeLicenseManager{
		consumeResp: &UsageConsumeResult{
			AccessMode: LicenseAccessModeFree,
			Remaining:  3,
		},
	}
	executor := NewExecutor(fakeGenerator{}, fakePublisher{}, manager)

	result, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Enterprise Collaboration Platform Overview",
		Prompt:       "Describe the product capabilities, customer value, and use cases of this collaboration platform.",
		OutputDir:    tmpDir,
		Publish:      false,
		LicenseCheck: &LicenseCheckResult{
			Allowed:    true,
			AccessMode: LicenseAccessModeFree,
			QuotaSnapshot: &licenseprovider.QuotaSnapshot{
				CreditAccount: licenseprovider.CreditAccountSnapshot{OwnerKind: "fingerprint", Balance: 30, Reserved: 0, Available: 30},
				RewardQuota:   licenseprovider.RewardQuotaSnapshot{Remaining: 0},
			},
			CommitToken: UsageCommitToken{
				FingerprintHash: "fp",
				RequestID:       "req-free",
			},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	warnings := strings.Join(result.Warnings, "\n")
	if !strings.Contains(warnings, "Current mode: external") {
		t.Fatalf("warnings = %s", warnings)
	}
	if !strings.Contains(warnings, "Credit balance: 30 available") {
		t.Fatalf("warnings = %s", warnings)
	}
	if result.AccessMode != string(LicenseAccessModeFree) {
		t.Fatalf("accessMode = %s", result.AccessMode)
	}
}

func TestExecutorAddsPaidModeRemainingWarningAfterConsume(t *testing.T) {
	tmpDir := t.TempDir()
	manager := &fakeLicenseManager{
		consumeResp: &UsageConsumeResult{
			AccessMode:         LicenseAccessModePaid,
			Remaining:          7,
			PaidQuotaRemaining: 7,
		},
	}
	executor := NewExecutor(fakeGenerator{}, fakePublisher{}, manager)

	result, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Enterprise Collaboration Platform Overview",
		Prompt:       "Describe the product capabilities, customer value, and use cases of this collaboration platform.",
		OutputDir:    tmpDir,
		Publish:      false,
		LicenseCheck: &LicenseCheckResult{
			Allowed:    true,
			AccessMode: LicenseAccessModePaid,
			QuotaSnapshot: &licenseprovider.QuotaSnapshot{
				CreditAccount: licenseprovider.CreditAccountSnapshot{},
				RewardQuota:   licenseprovider.RewardQuotaSnapshot{Remaining: 2},
				PaidExternalQuota: licenseprovider.PaidExternalQuotaSnapshot{
					CurrentKeyPrefix:    "cop_live_demo",
					CurrentKeyRemaining: 7,
				},
			},
			CommitToken: UsageCommitToken{
				FingerprintHash: "fp",
				RequestID:       "req-paid",
			},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	warnings := strings.Join(result.Warnings, "\n")
	if !strings.Contains(warnings, "Current mode: paid. 7 document generations remaining.") {
		t.Fatalf("warnings = %s", warnings)
	}
	if !strings.Contains(warnings, "Paid quota on current key: 7 remaining.") || !strings.Contains(warnings, "Reward quota: 2 remaining.") {
		t.Fatalf("warnings = %s", warnings)
	}
	if result.AccessMode != string(LicenseAccessModePaid) {
		t.Fatalf("accessMode = %s", result.AccessMode)
	}
}

func TestExecutorAddsRewardModeRemainingWarningAfterConsume(t *testing.T) {
	tmpDir := t.TempDir()
	manager := &fakeLicenseManager{
		consumeResp: &UsageConsumeResult{
			AccessMode:      LicenseAccessModeReward,
			Remaining:       4,
			RewardRemaining: 4,
		},
	}
	executor := NewExecutor(fakeGenerator{}, fakePublisher{}, manager)

	result, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Enterprise Collaboration Platform Overview",
		Prompt:       "Describe the product capabilities, customer value, and use cases of this collaboration platform.",
		OutputDir:    tmpDir,
		Publish:      false,
		LicenseCheck: &LicenseCheckResult{
			Allowed:    true,
			AccessMode: LicenseAccessModeReward,
			QuotaSnapshot: &licenseprovider.QuotaSnapshot{
				CreditAccount: licenseprovider.CreditAccountSnapshot{},
				RewardQuota:   licenseprovider.RewardQuotaSnapshot{Remaining: 4},
			},
			CommitToken: UsageCommitToken{
				FingerprintHash: "fp",
				RequestID:       "req-reward",
			},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	warnings := strings.Join(result.Warnings, "\n")
	if !strings.Contains(warnings, "Current mode: reward. 4 document generations remaining.") {
		t.Fatalf("warnings = %s", warnings)
	}
	if !strings.Contains(warnings, "Reward quota: 4 remaining.") {
		t.Fatalf("warnings = %s", warnings)
	}
	if result.AccessMode != string(LicenseAccessModeReward) {
		t.Fatalf("accessMode = %s", result.AccessMode)
	}
}

func TestExecutorCarriesAccessModeWithoutConsumeRequestID(t *testing.T) {
	tmpDir := t.TempDir()
	manager := &fakeLicenseManager{}
	executor := NewExecutor(fakeGenerator{}, fakePublisher{}, manager)

	result, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Enterprise Collaboration Platform Overview",
		Prompt:       "Describe the product capabilities, customer value, and use cases of this collaboration platform.",
		OutputDir:    tmpDir,
		Publish:      false,
		LicenseCheck: &LicenseCheckResult{
			Allowed:    true,
			AccessMode: LicenseAccessModeReward,
			CommitToken: UsageCommitToken{
				FingerprintHash: "fp",
			},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if manager.consumeCalls != 0 {
		t.Fatalf("consumeCalls = %d", manager.consumeCalls)
	}
	if result.AccessMode != string(LicenseAccessModeReward) {
		t.Fatalf("accessMode = %s", result.AccessMode)
	}
}

func TestExecutorDoesNotConsumeWhenGenerationFails(t *testing.T) {
	tmpDir := t.TempDir()
	manager := &fakeLicenseManager{}
	executor := NewExecutor(failingGenerator{err: context.DeadlineExceeded}, fakePublisher{}, manager)

	_, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Enterprise Collaboration Platform Overview",
		Prompt:       "Describe the product capabilities, customer value, and use cases of this collaboration platform.",
		OutputDir:    tmpDir,
		Publish:      false,
		LicenseCheck: &LicenseCheckResult{
			Allowed:    true,
			AccessMode: LicenseAccessModePaid,
			CommitToken: UsageCommitToken{
				FingerprintHash: "fp",
				RequestID:       "req-fail-generate",
			},
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if manager.consumeCalls != 0 {
		t.Fatalf("consumeCalls = %d, want 0", manager.consumeCalls)
	}
}

func TestExecutorWarnsAndReturnsLocalFileWhenPublishFails(t *testing.T) {
	tmpDir := t.TempDir()
	manager := &fakeLicenseManager{}
	executor := NewExecutor(fakeGenerator{}, failingPublisher{err: context.DeadlineExceeded}, manager)

	result, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Enterprise Collaboration Platform Overview",
		Prompt:       "Describe the product capabilities, customer value, and use cases of this collaboration platform.",
		OutputDir:    tmpDir,
		Publish:      true,
		LicenseCheck: &LicenseCheckResult{
			Allowed:    true,
			AccessMode: LicenseAccessModePaid,
			CommitToken: UsageCommitToken{
				FingerprintHash: "fp",
				RequestID:       "req-fail-publish",
			},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Published {
		t.Fatal("publish failure should not mark result as published")
	}
	if _, err := os.Stat(result.FilePath); err != nil {
		t.Fatalf("stat local file: %v", err)
	}
	warnings := strings.Join(result.Warnings, "\n")
	if !strings.Contains(warnings, "Publishing failed") {
		t.Fatalf("warnings should include publishing failure, got: %v", result.Warnings)
	}
	if !strings.Contains(warnings, "officecli login") {
		t.Fatalf("warnings should prompt login, got: %v", result.Warnings)
	}
	if manager.consumeCalls != 1 {
		t.Fatalf("consumeCalls = %d, want 1", manager.consumeCalls)
	}
}
