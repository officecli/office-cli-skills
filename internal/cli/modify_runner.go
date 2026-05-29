package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/officecli/officecli/internal/runtime"
)

type ModifyResult struct {
	FilePath          string `json:"file_path"`
	DocumentType      string `json:"document_type"`
	RouterPath        string `json:"router_path,omitempty"`
	OpsApplied        int    `json:"ops_applied"`
	OpsFailed         int    `json:"ops_failed"`
	LLMCalls          int    `json:"llm_calls"`
	AttentionRequired bool   `json:"attention_required,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`
}

func (a *App) runModify(ctx context.Context, cfg Config, args []string) error {
	job, err := BuildModifyJob(args)
	if err != nil {
		return err
	}

	progress := NewProgressRenderer(a.Stdout, job.JSONOutput, isTerminalWriter(a.Stdout))
	progress.SetTransientCompletions(true)
	defer progress.Close()

	result, err := a.executeModifyJob(ctx, cfg, job, progress)
	if err != nil {
		return err
	}

	progress.Clear()
	return RenderModifyResult(a.Stdout, result, job.JSONOutput)
}

func (a *App) executeModifyJob(ctx context.Context, cfg Config, job ModifyJob, progress progressController) (ModifyResult, error) {
	if missing := missingLLMConfig(cfg); missing != "" {
		return ModifyResult{}, fmt.Errorf("generation service is not fully configured: missing %s. Run `officecli config set-generation` to finish setup", missing)
	}

	llmClient, err := a.newLLMClient(cfg.LLM)
	if err != nil {
		return ModifyResult{}, err
	}

	service := runtime.NewService(llmClient, progress)

	baseName := strings.TrimSuffix(filepath.Base(job.SourceFilePath), filepath.Ext(job.SourceFilePath))
	ext := filepath.Ext(job.SourceFilePath)
	outputPath := filepath.Join(job.OutputDir, baseName+".modified"+ext)

	emitProgress(ctx, progress, progressStepGenerate, "running", "Modifying document")

	result, err := service.Modify(ctx, runtime.ModifyParams{
		SourceFilePath: job.SourceFilePath,
		DocumentType:   job.DocumentType,
		Prompt:         job.Prompt,
		Language:       job.Language,
		Style:          job.Style,
		OutputPath:     outputPath,
	}, progress)
	if err != nil {
		emitProgress(ctx, progress, progressStepGenerate, "failed", "Document modification failed")
		return ModifyResult{}, fmt.Errorf("document modification failed: %w", err)
	}

	emitProgress(ctx, progress, progressStepWriteFile, "running", "Writing modified document")

	if err := os.MkdirAll(job.OutputDir, 0o755); err != nil {
		return ModifyResult{}, fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(outputPath, result.Bytes, 0o644); err != nil {
		return ModifyResult{}, fmt.Errorf("write output file: %w", err)
	}

	emitProgress(ctx, progress, progressStepWriteFile, "completed", "Modified document saved")

	return ModifyResult{
		FilePath:          outputPath,
		DocumentType:      string(job.DocumentType),
		RouterPath:        result.ResultMeta.RouterPath,
		OpsApplied:        result.ResultMeta.OpsApplied,
		OpsFailed:         result.ResultMeta.OpsFailed,
		LLMCalls:          result.ResultMeta.LLMCalls,
		AttentionRequired: result.ResultMeta.AttentionRequired,
		Warnings:          result.ResultMeta.Warnings,
	}, nil
}

func RenderModifyResult(w io.Writer, result ModifyResult, jsonOutput bool) error {
	if jsonOutput {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	if _, err := fmt.Fprintf(w, "Modification completed. Saved to %s\n", result.FilePath); err != nil {
		return err
	}
	if result.OpsApplied > 0 {
		if _, err := fmt.Fprintf(w, "Operations applied: %d\n", result.OpsApplied); err != nil {
			return err
		}
	}
	if result.OpsFailed > 0 {
		if _, err := fmt.Fprintf(w, "Operations failed: %d\n", result.OpsFailed); err != nil {
			return err
		}
	}
	if result.AttentionRequired {
		if _, err := fmt.Fprintf(w, "Attention: some operations may need manual review\n"); err != nil {
			return err
		}
	}
	for _, warning := range result.Warnings {
		if strings.TrimSpace(warning) == "" {
			continue
		}
		if _, err := fmt.Fprintf(w, "Warning: %s\n", warning); err != nil {
			return err
		}
	}
	return nil
}
