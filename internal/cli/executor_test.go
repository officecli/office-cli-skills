package cli

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/officecli/officecli/engine"
	licenseprovider "github.com/officecli/officecli/internal/license"
	"github.com/officecli/officecli/internal/runtime"
	"github.com/officecli/officecli/pkg/officegen"
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

type gifGenerator struct{}

func (gifGenerator) Generate(_ context.Context, params GenerateParams) (*GeneratedArtifact, error) {
	return &GeneratedArtifact{
		DocumentName: params.Topic + ".gif",
		DocumentType: string(params.DocumentType),
		Bytes:        []byte("gif-bytes"),
		Sidecars: []GeneratedSidecar{{
			FileName: params.Topic + ".sheet.png",
			Bytes:    []byte("sheet-bytes"),
		}},
	}, nil
}

type pngImageGenerator struct{}

func (pngImageGenerator) Generate(_ context.Context, params GenerateParams) (*GeneratedArtifact, error) {
	return &GeneratedArtifact{
		DocumentName: params.Topic + ".png",
		DocumentType: string(params.DocumentType),
		Bytes:        solidPNGBytesForTest(480, 270, color.RGBA{R: 40, G: 90, B: 160, A: 255}),
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

type referencePPTXGenerator struct{}

func (referencePPTXGenerator) Generate(_ context.Context, params GenerateParams) (*GeneratedArtifact, error) {
	bytes, err := officegen.NewPPTXGenerator().Generate([]officegen.Slide{
		{Title: params.Topic, Subtitle: "Reference-aware deck", Layout: "title", IsTitle: true},
		{Title: "What changes", Layout: "content", Points: []string{"Reference style profile", "Editable text"}},
	}, officegen.PPTXOptions{Title: params.Topic, Creator: "test", StylePreset: officegen.StylePresetEditorialLight})
	if err != nil {
		return nil, err
	}
	return &GeneratedArtifact{
		DocumentName: params.Topic + ".pptx",
		DocumentType: string(params.DocumentType),
		Bytes:        bytes,
		ReferenceStyle: &runtime.ReferenceStyleMetadata{
			Enabled:         true,
			Root:            params.ReferenceScanRoot,
			DiscoveredCount: 2,
			ParsedCount:     1,
			FailedCount:     1,
			DuplicateCount:  0,
			SourceBuckets:   map[string]int{"other": 1, "tmp": 1},
			StyleBrief: &runtime.PPTXReferenceStyleBrief{
				StylePresetHint: officegen.StylePresetEditorialLight,
				LayoutRhythm:    "sections-grid",
			},
		},
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

func TestExecutorWritesGIFSheetSidecar(t *testing.T) {
	tmpDir := t.TempDir()
	executor := NewExecutor(gifGenerator{}, fakePublisher{}, nil)

	result, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypeGIF,
		Topic:        "Token_Reaction",
		OutputDir:    tmpDir,
		Publish:      false,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if filepath.Ext(result.FilePath) != ".gif" {
		t.Fatalf("file path = %s", result.FilePath)
	}
	data, err := os.ReadFile(result.FilePath)
	if err != nil {
		t.Fatalf("read gif: %v", err)
	}
	if string(data) != "gif-bytes" {
		t.Fatalf("gif bytes = %q", string(data))
	}
	sheetPath := filepath.Join(tmpDir, "Token_Reaction.sheet.png")
	sheet, err := os.ReadFile(sheetPath)
	if err != nil {
		t.Fatalf("read sheet sidecar: %v", err)
	}
	if string(sheet) != "sheet-bytes" {
		t.Fatalf("sheet bytes = %q", string(sheet))
	}
}

func TestExecutorAppliesImageWatermarkBeforeReturningIMG(t *testing.T) {
	tmpDir := t.TempDir()
	sourceColor := color.RGBA{R: 40, G: 90, B: 160, A: 255}
	executor := NewExecutor(pngImageGenerator{}, nil, nil)

	result, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypeIMG,
		Topic:        "Launch_Visual",
		OutputDir:    tmpDir,
		Publish:      false,
		ImageWatermark: &ImageWatermarkOptions{
			Apply:           true,
			PaidEntitlement: false,
			CanDisable:      false,
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ImageWatermark == nil || !result.ImageWatermark.Applied {
		t.Fatalf("image watermark result = %#v, want applied", result.ImageWatermark)
	}
	img := readPNGForTest(t, result.FilePath)
	if img.Bounds().Dy() <= 270 {
		t.Fatalf("image height = %d, want bottom watermark footer appended", img.Bounds().Dy())
	}
	if !rectHasChangedPixelForTest(img, sourceColor, image.Rect(0, 270, 480, img.Bounds().Dy())) {
		t.Fatal("bottom footer has no watermark pixels")
	}
	if !rectHasChangedPixelForTest(img, sourceColor, image.Rect(265, 218, 466, 258)) {
		t.Fatal("bottom-right corner has no footmark pixels")
	}
	if rectHasChangedPixelForTest(img, sourceColor, image.Rect(150, 75, 330, 190)) {
		t.Fatal("watermark changed center pixels; diagonal watermark should stay removed")
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

func solidPNGBytesForTest(width, height int, c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func readPNGForTest(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open png: %v", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	return img
}

func rectHasChangedPixelForTest(img image.Image, sourceColor color.RGBA, rect image.Rectangle) bool {
	rect = rect.Intersect(img.Bounds())
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			got := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			if got != sourceColor {
				return true
			}
		}
	}
	return false
}

func TestExecutorAddsReferenceStyleAndStructuralReviewMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	executor := NewExecutor(referencePPTXGenerator{}, nil, nil)

	result, err := executor.Run(context.Background(), GenerateJob{
		DocumentType:         engine.DocumentTypePPTX,
		Topic:                "Reference Metadata Deck",
		OutputDir:            tmpDir,
		Publish:              false,
		ReferenceScanEnabled: true,
		ReferenceScanRoot:    tmpDir,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ReferenceStyle == nil {
		t.Fatalf("missing reference style metadata: %+v", result)
	}
	if result.ReferenceStyle.DiscoveredCount != 2 || result.ReferenceStyle.ParsedCount != 1 || result.ReferenceStyle.FailedCount != 1 {
		t.Fatalf("reference style metadata = %#v", result.ReferenceStyle)
	}
	if result.PPTXReview == nil {
		t.Fatalf("missing pptx review metadata: %+v", result)
	}
	if result.PPTXReview.StructureScore < 70 {
		t.Fatalf("structure score = %d, want >= 70 (%#v)", result.PPTXReview.StructureScore, result.PPTXReview)
	}
}

func TestExecutorAddsPPTXArtifactDebugMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	executor := NewExecutor(debugPPTXGenerator{}, nil, nil)

	result, err := executor.Run(context.Background(), GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Debug Artifact Deck",
		OutputDir:    tmpDir,
		Publish:      false,
		Debug:        true,
		PPTXBackend:  runtime.PPTXBackendArtifactExperimental,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.PPTXArtifactDebug == nil {
		t.Fatalf("missing artifact debug metadata: %+v", result)
	}
	if result.PPTXArtifactDebug.WorkerVersion != "artifact-experimental-test" || result.PPTXArtifactDebug.PreviewCount != 2 {
		t.Fatalf("artifact debug metadata = %#v", result.PPTXArtifactDebug)
	}
	if strings.TrimSpace(result.PPTXArtifactDebug.NarrativePlanPath) == "" {
		t.Fatalf("missing narrative plan path: %#v", result.PPTXArtifactDebug)
	}
	planBytes, err := os.ReadFile(filepath.Join(tmpDir, "narrative_plan.md"))
	if err != nil {
		t.Fatalf("read narrative plan sidecar: %v", err)
	}
	plan := string(planBytes)
	for _, expected := range []string{"# Narrative Plan", "Audience", "Editability Plan"} {
		if !strings.Contains(plan, expected) {
			t.Fatalf("narrative plan sidecar missing %q:\n%s", expected, plan)
		}
	}
}

type debugPPTXGenerator struct{}

func (debugPPTXGenerator) Generate(_ context.Context, _ GenerateParams) (*GeneratedArtifact, error) {
	data, err := officegen.NewPPTXGenerator().Generate([]officegen.Slide{{
		Title:   "Debug Artifact Deck",
		Layout:  "title",
		IsTitle: true,
	}}, officegen.PPTXOptions{Title: "Debug Artifact Deck"})
	if err != nil {
		return nil, err
	}
	return &GeneratedArtifact{
		DocumentName: "debug-artifact.pptx",
		DocumentType: string(engine.DocumentTypePPTX),
		Bytes:        data,
		PPTXBackend:  runtime.PPTXBackendArtifactExperimental,
		PPTXArtifactDebug: &runtime.PPTXArtifactDebugMetadata{
			Enabled:               true,
			Backend:               runtime.PPTXBackendArtifactExperimental,
			WorkerVersion:         "artifact-experimental-test",
			PreviewCount:          2,
			InspectPath:           "/tmp/inspect.json",
			NarrativePlanMarkdown: "# Narrative Plan\n\n## Audience\nDebug audience.\n\n## Editability Plan\nKeep text editable.\n",
		},
	}, nil
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
