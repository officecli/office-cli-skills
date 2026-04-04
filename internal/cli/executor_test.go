package cli

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/officecli/officecli/engine"
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

type failingGenerator struct {
	err error
}

func (f failingGenerator) Generate(_ context.Context, _ GenerateParams) (*GeneratedArtifact, error) {
	return nil, f.err
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
	return &UsageConsumeResult{AccessMode: LicenseAccessModePaid, Remaining: 2, FreeRemaining: 2}, nil
}

func TestExecutorGenerateAndPublish(t *testing.T) {
	tmpDir := t.TempDir()
	executor := NewExecutor(fakeGenerator{}, fakePublisher{}, nil)

	result, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "企业协作平台介绍",
		Prompt:       "介绍这款企业协作平台的产品能力、客户价值与应用场景",
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

func TestExecutorConsumesUsageAfterSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	manager := &fakeLicenseManager{}
	executor := NewExecutor(fakeGenerator{}, fakePublisher{}, manager)

	result, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "企业协作平台介绍",
		Prompt:       "介绍这款企业协作平台的产品能力、客户价值与应用场景",
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
		Topic:        "企业协作平台介绍",
		Prompt:       "介绍这款企业协作平台的产品能力、客户价值与应用场景",
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
		"generate:running:正在生成文档内容",
		"write_file:running:正在写入本地文件",
		"publish:running:正在发布在线预览",
		"finalize:completed:文档已生成",
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
		Topic:        "企业协作平台介绍",
		Prompt:       "介绍这款企业协作平台的产品能力、客户价值与应用场景",
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
	if !strings.Contains(err.Error(), "额度同步失败") {
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
			AccessMode:    LicenseAccessModeFree,
			Remaining:     3,
			FreeRemaining: 3,
		},
	}
	executor := NewExecutor(fakeGenerator{}, fakePublisher{}, manager)

	result, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "企业协作平台介绍",
		Prompt:       "介绍这款企业协作平台的产品能力、客户价值与应用场景",
		OutputDir:    tmpDir,
		Publish:      false,
		LicenseCheck: &LicenseCheckResult{
			Allowed:    true,
			AccessMode: LicenseAccessModeFree,
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
	if !strings.Contains(warnings, "当前为免费模式，剩余 3 次生成额度") {
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
		Topic:        "企业协作平台介绍",
		Prompt:       "介绍这款企业协作平台的产品能力、客户价值与应用场景",
		OutputDir:    tmpDir,
		Publish:      false,
		LicenseCheck: &LicenseCheckResult{
			Allowed:    true,
			AccessMode: LicenseAccessModePaid,
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
	if !strings.Contains(warnings, "当前为付费模式，剩余 7 次生成额度") {
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
		Topic:        "企业协作平台介绍",
		Prompt:       "介绍这款企业协作平台的产品能力、客户价值与应用场景",
		OutputDir:    tmpDir,
		Publish:      false,
		LicenseCheck: &LicenseCheckResult{
			Allowed:    true,
			AccessMode: LicenseAccessModeReward,
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
	if !strings.Contains(warnings, "当前为奖励模式，剩余 4 次生成额度") {
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
		Topic:        "企业协作平台介绍",
		Prompt:       "介绍这款企业协作平台的产品能力、客户价值与应用场景",
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
		Topic:        "企业协作平台介绍",
		Prompt:       "介绍这款企业协作平台的产品能力、客户价值与应用场景",
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

func TestExecutorDoesNotConsumeWhenPublishFails(t *testing.T) {
	tmpDir := t.TempDir()
	manager := &fakeLicenseManager{}
	executor := NewExecutor(fakeGenerator{}, failingPublisher{err: context.DeadlineExceeded}, manager)

	_, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "企业协作平台介绍",
		Prompt:       "介绍这款企业协作平台的产品能力、客户价值与应用场景",
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
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "发布阶段失败") {
		t.Fatalf("unexpected error: %v", err)
	}
	if manager.consumeCalls != 1 {
		t.Fatalf("consumeCalls = %d, want 1", manager.consumeCalls)
	}
}
