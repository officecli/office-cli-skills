package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/officecli/officecli/engine"
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
		DocumentType:   job.DocumentType,
		Topic:          job.Topic,
		Prompt:         job.Prompt,
		SourceFilePath: job.SourceFilePath,
		Mode:           job.Mode,
		Language:       job.Language,
		Style:          job.Style,
		Audience:       job.Audience,
		EnableImages:   job.EnableImages,
		LocalPreview:   job.LocalPreview,
	})
	if err != nil {
		emitProgress(ctx, e.progress, progressStepGenerate, "failed", "Content generation failed")
		return GenerateResult{}, fmt.Errorf("content generation failed: %w", err)
	}
	emitProgress(ctx, e.progress, progressStepGenerate, "completed", "Document content generation completed")
	fileName := artifact.DocumentName
	if fileName == "" {
		fileName = job.Topic + "." + string(job.DocumentType)
	}
	filePath := filepath.Join(job.OutputDir, fileName)
	fileBase := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	localPreviewPath := filepath.Join(job.OutputDir, fileBase+".preview.html")
	localPreviewDataPath := filepath.Join(job.OutputDir, fileBase+".preview.json")
	result := GenerateResult{
		Status:       "success",
		FilePath:     filePath,
		DocumentType: string(job.DocumentType),
		DocumentName: artifact.DocumentName,
		RuntimeMode:  resultRuntimeMode(job, job.LicenseCheck),
		Warnings:     warningMessages(artifact.Warnings),
	}
	if job.LicenseCheck != nil {
		result.AccessMode = string(job.LicenseCheck.AccessMode)
		result.AllowedModes = append([]string(nil), job.LicenseCheck.AllowedModes...)
		result.HostedEnabled = job.LicenseCheck.HostedEnabled
		result.CreditBalance = job.LicenseCheck.CreditBalance
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
			result.FreeRemaining = consumeResult.FreeRemaining
			result.RewardRemaining = consumeResult.RewardRemaining
			result.PaidQuotaRemaining = consumeResult.PaidQuotaRemaining
			result.CreditBalance = consumeResult.CreditBalance
			switch job.LicenseCheck.AccessMode {
			case LicenseAccessModePaid:
				result.Warnings = append(result.Warnings, fmt.Sprintf("Current mode: paid. %d document generations remaining.", consumeResult.Remaining))
			case LicenseAccessModeReward:
				result.Warnings = append(result.Warnings, fmt.Sprintf("Current mode: reward. %d document generations remaining.", consumeResult.Remaining))
			default:
				result.Warnings = append(result.Warnings, fmt.Sprintf("Current mode: free. %d document generations remaining.", consumeResult.Remaining))
			}
			if snapshot := job.LicenseCheck.QuotaSnapshot; snapshot != nil {
				freeRemaining := snapshot.FreeTrialDaily.Remaining
				if job.LicenseCheck.AccessMode == LicenseAccessModeFree {
					freeRemaining = consumeResult.Remaining
				}
				result.Warnings = append(result.Warnings, fmt.Sprintf("Trial today on this machine: %d remaining.", freeRemaining))

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

	emitProgress(ctx, e.progress, progressStepWriteFile, "running", "Writing local files")
	if err := os.MkdirAll(job.OutputDir, 0o755); err != nil {
		emitProgress(ctx, e.progress, progressStepWriteFile, "failed", "File write failed")
		return GenerateResult{}, fmt.Errorf("file write failed: %w", err)
	}
	if err := os.WriteFile(filePath, artifact.Bytes, 0o644); err != nil {
		emitProgress(ctx, e.progress, progressStepWriteFile, "failed", "File write failed")
		return GenerateResult{}, fmt.Errorf("file write failed: %w", err)
	}
	if len(artifact.PreviewHTML) > 0 {
		if err := os.WriteFile(localPreviewPath, artifact.PreviewHTML, 0o644); err != nil {
			emitProgress(ctx, e.progress, progressStepWriteFile, "failed", "Preview file write failed")
			return GenerateResult{}, fmt.Errorf("preview file write failed: %w", err)
		}
		result.LocalPreviewPath = localPreviewPath
	}
	if len(artifact.PreviewJSON) > 0 {
		if err := os.WriteFile(localPreviewDataPath, artifact.PreviewJSON, 0o644); err != nil {
			emitProgress(ctx, e.progress, progressStepWriteFile, "failed", "Preview file write failed")
			return GenerateResult{}, fmt.Errorf("preview file write failed: %w", err)
		}
		result.LocalPreviewDataPath = localPreviewDataPath
	}
	emitProgress(ctx, e.progress, progressStepWriteFile, "completed", "Local file write completed")

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
				emitProgress(ctx, e.progress, progressStepPublish, "failed", "Publishing failed")
				return GenerateResult{}, fmt.Errorf("publishing failed: %w", err)
			}
			result.Published = true
			result.AccessURL = published.AccessURL
			result.Password = published.Password
			result.ExpiresAt = published.ExpiresAt
			emitProgress(ctx, e.progress, progressStepPublish, "completed", "Online preview publishing completed")
		}
	}
	emitProgress(ctx, e.progress, progressStepFinalize, "completed", "Document generated")
	return result, nil
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
