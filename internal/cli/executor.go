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
	emitProgress(ctx, e.progress, progressStepGenerate, "running", "正在生成文档内容")
	artifact, err := e.generator.Generate(ctx, GenerateParams{
		DocumentType: job.DocumentType,
		Topic:        job.Topic,
		Prompt:       job.Prompt,
		Mode:         job.Mode,
		Language:     job.Language,
		Style:        job.Style,
		Audience:     job.Audience,
		EnableImages: job.EnableImages,
	})
	if err != nil {
		emitProgress(ctx, e.progress, progressStepGenerate, "failed", "生成内容阶段失败")
		return GenerateResult{}, fmt.Errorf("生成内容阶段失败：%w", err)
	}
	emitProgress(ctx, e.progress, progressStepGenerate, "completed", "文档内容生成完成")
	emitProgress(ctx, e.progress, progressStepWriteFile, "running", "正在写入本地文件")
	if err := os.MkdirAll(job.OutputDir, 0o755); err != nil {
		emitProgress(ctx, e.progress, progressStepWriteFile, "failed", "写入文件阶段失败")
		return GenerateResult{}, fmt.Errorf("写入文件阶段失败：%w", err)
	}
	fileName := artifact.DocumentName
	if fileName == "" {
		fileName = job.Topic + "." + string(job.DocumentType)
	}
	filePath := filepath.Join(job.OutputDir, fileName)
	if err := os.WriteFile(filePath, artifact.Bytes, 0o644); err != nil {
		emitProgress(ctx, e.progress, progressStepWriteFile, "failed", "写入文件阶段失败")
		return GenerateResult{}, fmt.Errorf("写入文件阶段失败：%w", err)
	}
	emitProgress(ctx, e.progress, progressStepWriteFile, "completed", "本地文件写入完成")

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

	if job.Publish {
		if e.publisher == nil {
			result.Warnings = append(result.Warnings, "未配置发布端，跳过在线预览")
		} else {
			emitProgress(ctx, e.progress, progressStepPublish, "running", "正在发布在线预览")
			published, err := e.publisher.Publish(ctx, PublishRequest{
				LocalFilePath: filePath,
				DocumentType:  string(job.DocumentType),
				DocumentName:  artifact.DocumentName,
			})
			if err != nil {
				emitProgress(ctx, e.progress, progressStepPublish, "failed", "发布阶段失败")
				return GenerateResult{}, fmt.Errorf("发布阶段失败：%w", err)
			}
			result.Published = true
			result.AccessURL = published.AccessURL
			result.Password = published.Password
			result.ExpiresAt = published.ExpiresAt
			emitProgress(ctx, e.progress, progressStepPublish, "completed", "在线预览发布完成")
		}
	}

	if e.license != nil && job.LicenseCheck != nil && job.LicenseCheck.AccessMode != LicenseAccessModeHosted && strings.TrimSpace(job.LicenseCheck.CommitToken.RequestID) != "" {
		consumeResult, err := e.license.Consume(ctx, job.LicenseCheck.CommitToken)
		if err != nil {
			result.Warnings = append(result.Warnings, "生成已完成，但额度同步失败，请稍后执行 `officecli auth status` 检查状态")
			emitProgress(ctx, e.progress, progressStepFinalize, "completed", "文档已生成")
			return result, nil
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
				result.Warnings = append(result.Warnings, fmt.Sprintf("当前为付费模式，剩余 %d 次生成额度。", consumeResult.Remaining))
			case LicenseAccessModeReward:
				result.Warnings = append(result.Warnings, fmt.Sprintf("当前为奖励模式，剩余 %d 次生成额度。", consumeResult.Remaining))
			default:
				result.Warnings = append(result.Warnings, fmt.Sprintf("当前为免费模式，剩余 %d 次生成额度。", consumeResult.Remaining))
			}
		}
	} else if job.LicenseCheck != nil && job.LicenseCheck.AccessMode == LicenseAccessModeHosted {
		result.Warnings = append(result.Warnings, fmt.Sprintf("当前为托管模式，剩余 %d credits。", job.LicenseCheck.CreditBalance))
	}
	emitProgress(ctx, e.progress, progressStepFinalize, "completed", "文档已生成")
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
