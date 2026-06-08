package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/officecli/officecli/engine"
	"github.com/officecli/officecli/internal/imagewatermark"
	reviewprovider "github.com/officecli/officecli/internal/review"
)

type Executor struct {
	generator Generator
	publisher Publisher
	license   LicenseManager
	progress  engine.ProgressEmitter
}

func NewExecutor(generator Generator, publisher Publisher, license LicenseManager) *Executor {
	return &Executor{generator: generator, publisher: publisher, license: license}
}

func (e *Executor) Run(ctx context.Context, job GenerateJob) (GenerateResult, error) {
	if e == nil || e.generator == nil {
		return GenerateResult{}, fmt.Errorf("generator is unavailable")
	}
	emitProgress(ctx, e.progress, progressStepGenerate, "running", "Generating document content")
	artifact, err := e.generator.Generate(ctx, GenerateParams{
		DocumentType:         job.DocumentType,
		Topic:                job.Topic,
		Prompt:               job.Prompt,
		SourceFilePath:       job.SourceFilePath,
		Mode:                 job.Mode,
		Language:             job.Language,
		Style:                job.Style,
		Audience:             job.Audience,
		EnableImages:         job.EnableImages,
		ImageRatio:           job.ImageRatio,
		ImageSize:            job.ImageSize,
		GifFPS:               job.GifFPS,
		ReferenceImages:      append([]engine.ImageReference(nil), job.ReferenceImages...),
		ReferenceScanEnabled: job.ReferenceScanEnabled,
		ReferenceScanRoot:    job.ReferenceScanRoot,
		ReferencePPTXSources: append([]string(nil), job.ReferencePPTXSources...),
		PPTXBackend:          job.PPTXBackend,
		LocalPreview:         job.LocalPreview,
		Debug:                job.Debug,
	})
	if err != nil {
		emitProgress(ctx, e.progress, progressStepGenerate, "failed", "Content generation failed")
		return GenerateResult{}, fmt.Errorf("content generation failed: %w", err)
	}
	emitProgress(ctx, e.progress, progressStepGenerate, "completed", "Document content generation completed")
	return e.finalizeArtifact(ctx, job, artifact)
}

func (e *Executor) finalizeArtifact(ctx context.Context, job GenerateJob, artifact *GeneratedArtifact) (GenerateResult, error) {
	if artifact == nil {
		return GenerateResult{}, fmt.Errorf("generated artifact is unavailable")
	}
	fileName := artifact.DocumentName
	if fileName == "" {
		fileName = job.Topic + "." + string(job.DocumentType)
	}
	filePath := filepath.Join(job.OutputDir, fileName)
	fileBase := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	localPreviewPath := filepath.Join(job.OutputDir, fileBase+".preview.html")
	localPreviewDataPath := filepath.Join(job.OutputDir, fileBase+".preview.json")
	allWarnings := append([]engine.GenerateIssue(nil), job.Warnings...)
	allWarnings = append(allWarnings, artifact.Warnings...)
	result := GenerateResult{
		Status:            "success",
		FilePath:          filePath,
		DocumentType:      string(job.DocumentType),
		DocumentName:      artifact.DocumentName,
		RuntimeMode:       resultRuntimeMode(job, job.LicenseCheck),
		Warnings:          warningMessages(allWarnings),
		CreditMode:        inferCreditMode(job),
		ReferenceStyle:    artifact.ReferenceStyle,
		PPTXArtifactDebug: artifact.PPTXArtifactDebug,
		PPTXBackend:       resultPPTXBackend(artifact, job),
	}
	if artifact.HostedCreditsCharged != nil {
		result.CreditsCharged = *artifact.HostedCreditsCharged
	}
	if job.LicenseCheck != nil {
		result.AccessMode = string(job.LicenseCheck.AccessMode)
		result.AllowedModes = append([]string(nil), job.LicenseCheck.AllowedModes...)
		result.HostedEnabled = job.LicenseCheck.HostedEnabled
		result.CreditBalance = job.LicenseCheck.CreditBalance
	}
	if artifact.HostedCreditBalance != nil {
		result.AccessMode = string(LicenseAccessModeHosted)
		result.HostedEnabled = true
		result.CreditBalance = *artifact.HostedCreditBalance
		if len(quotaWarnings(result.Warnings)) == 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Current mode: hosted. %d credits remaining.", *artifact.HostedCreditBalance))
		}
	}
	if strings.TrimSpace(artifact.AccessMode) != "" {
		result.AccessMode = artifact.AccessMode
		result.Remaining = artifact.Remaining
		result.RewardRemaining = artifact.RewardRemaining
		result.PaidQuotaRemaining = artifact.PaidQuotaRemaining
	}
	if artifact.HostedCreditBalance != nil {
		result.HostedEnabled = true
		result.CreditBalance = *artifact.HostedCreditBalance
	}

	if e.license != nil && job.LicenseCheck != nil && job.LicenseCheck.AccessMode != LicenseAccessModeHosted && strings.TrimSpace(job.LicenseCheck.CommitToken.RequestID) != "" {
		consumeResult, err := e.license.Consume(ctx, job.LicenseCheck.CommitToken)
		if err != nil {
			emitProgress(ctx, e.progress, progressStepFinalize, "failed", "Quota sync failed")
			return GenerateResult{}, fmt.Errorf("quota sync failed: %w", err)
		}
		if consumeResult != nil {
			if consumeResult.AccessMode != "" {
				result.AccessMode = string(consumeResult.AccessMode)
			}
			result.Remaining = consumeResult.Remaining
			result.RewardRemaining = consumeResult.RewardRemaining
			result.PaidQuotaRemaining = consumeResult.PaidQuotaRemaining
			result.CreditBalance = consumeResult.CreditBalance
			switch job.LicenseCheck.AccessMode {
			case LicenseAccessModePaid:
				result.Warnings = append(result.Warnings, fmt.Sprintf("Current mode: paid. %d document generations remaining.", consumeResult.Remaining))
			case LicenseAccessModeReward:
				result.Warnings = append(result.Warnings, fmt.Sprintf("Current mode: reward. %d document generations remaining.", consumeResult.Remaining))
			default:
				result.Warnings = append(result.Warnings, "Current mode: external (using your locally-configured LLM key).")
			}
			if snapshot := job.LicenseCheck.QuotaSnapshot; snapshot != nil {
				if snapshot.CreditAccount.OwnerKind != "" || snapshot.CreditAccount.Balance > 0 || snapshot.CreditAccount.Reserved > 0 {
					result.Warnings = append(result.Warnings, fmt.Sprintf("Credit balance: %d available (%d reserved).", snapshot.CreditAccount.Available, snapshot.CreditAccount.Reserved))
				}

				rewardRemaining := snapshot.RewardQuota.Remaining
				if job.LicenseCheck.AccessMode == LicenseAccessModeReward {
					rewardRemaining = consumeResult.Remaining
				}
				result.Warnings = append(result.Warnings, fmt.Sprintf("Reward quota: %d remaining.", rewardRemaining))

				paidRemaining := snapshot.PaidExternalQuota.CurrentKeyRemaining
				if job.LicenseCheck.AccessMode == LicenseAccessModePaid {
					paidRemaining = consumeResult.Remaining
				}
				if snapshot.PaidExternalQuota.CurrentKeyPrefix != "" || paidRemaining > 0 {
					result.Warnings = append(result.Warnings, fmt.Sprintf("Paid quota on current key: %d remaining.", paidRemaining))
				}
			}
		}
	} else if job.LicenseCheck != nil && job.LicenseCheck.AccessMode == LicenseAccessModeHosted {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Current mode: hosted. %d credits remaining.", job.LicenseCheck.CreditBalance))
	}
	if artifact.HostedCreditBalance != nil {
		result.HostedEnabled = true
		result.CreditBalance = *artifact.HostedCreditBalance
	}

	emitProgress(ctx, e.progress, progressStepWriteFile, "running", "Writing local files")
	if err := os.MkdirAll(job.OutputDir, 0o755); err != nil {
		emitProgress(ctx, e.progress, progressStepWriteFile, "failed", "File write failed")
		return GenerateResult{}, fmt.Errorf("file write failed: %w", err)
	}
	if err := os.WriteFile(filePath, artifact.Bytes, 0o644); err != nil {
		emitProgress(ctx, e.progress, progressStepWriteFile, "failed", "File write failed")
		return GenerateResult{}, fmt.Errorf("file write failed: %w", err)
	}
	if err := applyImageWatermarkToGeneratedFile(filePath, job, &result); err != nil {
		emitProgress(ctx, e.progress, progressStepWriteFile, "failed", "Image watermark failed")
		return GenerateResult{}, fmt.Errorf("image watermark failed: %w", err)
	}
	for _, sidecar := range artifact.Sidecars {
		name := strings.TrimSpace(sidecar.FileName)
		if name == "" || len(sidecar.Bytes) == 0 {
			continue
		}
		sidecarPath := filepath.Join(job.OutputDir, filepath.Base(name))
		if err := writeFileAtomic(sidecarPath, sidecar.Bytes, 0o644); err != nil {
			emitProgress(ctx, e.progress, progressStepWriteFile, "failed", "Sidecar file write failed")
			return GenerateResult{}, fmt.Errorf("sidecar file write failed: %w", err)
		}
	}
	if previewSidecarAllowed(job.DocumentType) {
		if len(artifact.PreviewHTML) > 0 {
			if err := writeFileAtomic(localPreviewPath, artifact.PreviewHTML, 0o644); err != nil {
				emitProgress(ctx, e.progress, progressStepWriteFile, "failed", "Preview file write failed")
				return GenerateResult{}, fmt.Errorf("preview file write failed: %w", err)
			}
			result.LocalPreviewPath = localPreviewPath
		}
		if len(artifact.PreviewJSON) > 0 {
			if err := writeFileAtomic(localPreviewDataPath, artifact.PreviewJSON, 0o644); err != nil {
				emitProgress(ctx, e.progress, progressStepWriteFile, "failed", "Preview file write failed")
				return GenerateResult{}, fmt.Errorf("preview file write failed: %w", err)
			}
			result.LocalPreviewDataPath = localPreviewDataPath
		}
	}
	if job.Debug && artifact.PPTXArtifactDebug != nil && strings.TrimSpace(artifact.PPTXArtifactDebug.NarrativePlanMarkdown) != "" {
		narrativePlanPath := filepath.Join(job.OutputDir, "narrative_plan.md")
		if err := writeFileAtomic(narrativePlanPath, []byte(artifact.PPTXArtifactDebug.NarrativePlanMarkdown), 0o644); err != nil {
			emitProgress(ctx, e.progress, progressStepWriteFile, "failed", "Narrative plan file write failed")
			return GenerateResult{}, fmt.Errorf("narrative plan file write failed: %w", err)
		}
		artifact.PPTXArtifactDebug.NarrativePlanPath = narrativePlanPath
		result.PPTXArtifactDebug = artifact.PPTXArtifactDebug
	}
	emitProgress(ctx, e.progress, progressStepWriteFile, "completed", "Local file write completed")

	if shouldRunPPTXReferenceStructuralReview(job, artifact) {
		reviewMeta, err := runPPTXStructuralReview(ctx, filePath)
		if err != nil {
			result.Warnings = append(result.Warnings, "PPTX structural review was skipped: "+err.Error())
		} else {
			result.PPTXReview = reviewMeta
			if reviewMeta.StructureScore < 70 {
				result.Warnings = append(result.Warnings, fmt.Sprintf("PPTX structural review score is %d, below the recommended threshold 70.", reviewMeta.StructureScore))
			}
		}
	}

	if job.Publish {
		if e.publisher == nil {
			result.Warnings = append(result.Warnings, "Publishing is not configured, so online preview publishing was skipped.")
		} else {
			emitProgress(ctx, e.progress, progressStepPublish, "running", "Publishing online preview")
			published, err := e.publisher.Publish(ctx, PublishRequest{
				LocalFilePath: filePath,
				DocumentType:  string(job.DocumentType),
				DocumentName:  artifact.DocumentName,
			})
			if err != nil {
				result.PublishedSkippedReason = fmt.Sprintf("publishing failed: %v", err)
				result.Warnings = append(result.Warnings, publishFailureWarning(err))
				emitProgress(ctx, e.progress, progressStepPublish, "completed", "Online preview publishing skipped")
			} else {
				result.Published = true
				result.AccessURL = published.AccessURL
				result.Password = published.Password
				result.ExpiresAt = published.ExpiresAt
				emitProgress(ctx, e.progress, progressStepPublish, "completed", "Online preview publishing completed")
			}
		}
	}
	emitProgress(ctx, e.progress, progressStepFinalize, "completed", "Document generated")
	return result, nil
}

func applyImageWatermarkToGeneratedFile(filePath string, job GenerateJob, result *GenerateResult) error {
	if job.DocumentType != engine.DocumentTypeIMG || job.ImageWatermark == nil {
		return nil
	}
	metadata := &ImageWatermarkResult{
		Applied:         false,
		PaidEntitlement: job.ImageWatermark.PaidEntitlement,
		CanDisable:      job.ImageWatermark.CanDisable,
	}
	if result != nil {
		result.ImageWatermark = metadata
	}
	if !job.ImageWatermark.Apply {
		return nil
	}
	if !isStaticImageWatermarkTarget(filePath) {
		return fmt.Errorf("unsupported image watermark target: %s", filepath.Ext(filePath))
	}
	if err := imagewatermark.Apply(filePath, imagewatermark.Options{}); err != nil {
		return err
	}
	metadata.Applied = true
	return nil
}

func isStaticImageWatermarkTarget(filePath string) bool {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(filePath), ".")) {
	case "png", "jpg", "jpeg":
		return true
	default:
		return false
	}
}

func shouldRunPPTXReferenceStructuralReview(job GenerateJob, artifact *GeneratedArtifact) bool {
	if artifact == nil || job.DocumentType != engine.DocumentTypePPTX {
		return false
	}
	return job.ReferenceScanEnabled || len(job.ReferencePPTXSources) > 0 || artifact.ReferenceStyle != nil
}

func resultPPTXBackend(artifact *GeneratedArtifact, job GenerateJob) string {
	if artifact != nil && strings.TrimSpace(artifact.PPTXBackend) != "" {
		return strings.TrimSpace(artifact.PPTXBackend)
	}
	return strings.TrimSpace(job.PPTXBackend)
}

func runPPTXStructuralReview(ctx context.Context, filePath string) (*PPTXReviewMetadata, error) {
	result, err := reviewprovider.NewService(nil, nil, nil).Review(ctx, reviewprovider.Request{
		FilePath:     filePath,
		DocumentType: string(engine.DocumentTypePPTX),
		EnableVisual: false,
	})
	if err != nil {
		return nil, err
	}
	return &PPTXReviewMetadata{
		Status:         result.Status,
		OverallScore:   result.OverallScore,
		StructureScore: result.StructureScore,
		Summary:        result.Summary,
		Warnings:       append([]string(nil), result.Warnings...),
	}, nil
}

func publishFailureWarning(err error) string {
	message := "Publishing failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message += ": " + strings.TrimSpace(err.Error())
	}
	return message + ". The local file was saved. Run `officecli login` or `officecli config set-publish` to enable online preview publishing."
}

func warningMessages(items []engine.GenerateIssue) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item.Message == "" {
			out = append(out, item.Code)
			continue
		}
		out = append(out, item.Message)
	}
	return out
}

// previewSidecarAllowed gates *.preview.html / *.preview.json emission to the
// extensions officedex's renderer actually consumes; other types would be
// silently ignored on the consumer side.
func previewSidecarAllowed(documentType engine.DocumentType) bool {
	switch documentType {
	case engine.DocumentTypePPTX, engine.DocumentTypeDOCX, engine.DocumentTypeXLSX:
		return true
	default:
		return false
	}
}

// writeFileAtomic writes data via a sibling temp file then renames it into
// place so the renderer never observes a partially-written sidecar.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}
