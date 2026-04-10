package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/officecli/officecli/engine"
)

type reviewProgressReporter struct {
	emitter engine.ProgressEmitter
}

func (r reviewProgressReporter) EmitReviewProgress(ctx context.Context, step, status, content string) {
	emitProgress(ctx, r.emitter, step, status, content)
}

func (a *App) runReview(ctx context.Context, cfg Config, args []string) error {
	job, err := BuildReviewJob(args)
	if err != nil {
		return err
	}
	progress := NewProgressRenderer(a.Stdout, job.JSONOutput, isTerminalWriter(a.Stdout))
	defer progress.Close()
	result, err := a.executeReviewJob(ctx, cfg, job, progress)
	if err != nil {
		return err
	}
	if err := RenderReviewResult(a.Stdout, *result, job.JSONOutput); err != nil {
		return err
	}
	if job.FailBelow > 0 && result.OverallScore < job.FailBelow {
		return fmt.Errorf("review failed: overall score %d is below threshold %d", result.OverallScore, job.FailBelow)
	}
	return nil
}

func (a *App) executeReviewJob(ctx context.Context, cfg Config, job ReviewJob, progress engine.ProgressEmitter) (*ReviewResult, error) {
	if a == nil || a.newReviewer == nil {
		return nil, fmt.Errorf("reviewer is unavailable")
	}
	reviewer, err := a.newReviewer(cfg, progress)
	if err != nil {
		return nil, err
	}
	return reviewer.Review(ctx, ReviewRequest{
		FilePath:      job.FilePath,
		DocumentType:  job.DocumentType,
		EnableVisual:  job.EnableVisual,
		FailBelow:     job.FailBelow,
		RuntimeMode:   string(cfg.RuntimeModeOrDefault()),
		LLMProvider:   cfg.LLM.Provider,
		LLMBaseURL:    cfg.LLM.BaseURL,
		LLMAPIKey:     cfg.LLM.APIKey,
		ReviewModel:   cfg.LLM.ReviewModel,
		LLMTimeoutSec: cfg.LLM.TimeoutSec,
	})
}

func (a *App) buildReviewJobFromRequest(req bridgeInvokeParams) (ReviewJob, error) {
	if strings.TrimSpace(req.Tool) == "" {
		req.Tool = bridgeToolOfficeReview
	}
	if req.Tool != bridgeToolOfficeReview && req.Tool != bridgeToolOfficeScore {
		return ReviewJob{}, fmt.Errorf("unsupported tool: %s", req.Tool)
	}
	if strings.ToLower(strings.TrimSpace(req.Args.DocumentType)) != "pptx" {
		return ReviewJob{}, fmt.Errorf("review currently supports only pptx")
	}
	filePath := strings.TrimSpace(req.Args.FilePath)
	if filePath == "" {
		return ReviewJob{}, fmt.Errorf("file_path is required")
	}
	enableVisual := true
	if req.Args.EnableVisual != nil {
		enableVisual = *req.Args.EnableVisual
	}
	failBelow := 0
	if req.Args.FailBelow != nil {
		failBelow = *req.Args.FailBelow
	}
	if failBelow < 0 || failBelow > 100 {
		return ReviewJob{}, fmt.Errorf("invalid fail_below: %d", failBelow)
	}
	return ReviewJob{
		DocumentType: "pptx",
		FilePath:     filePath,
		EnableVisual: enableVisual,
		FailBelow:    failBelow,
		JSONOutput:   true,
	}, nil
}
