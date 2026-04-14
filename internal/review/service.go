package review

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	statusExcellent = "excellent"
	statusGood      = "good"
	statusNeedsWork = "needs_work"
	statusPartial   = "partial"
)

func (s *Service) Review(ctx context.Context, req Request) (*Result, error) {
	if strings.TrimSpace(req.DocumentType) != "pptx" {
		return nil, fmt.Errorf("review is currently only supported for pptx")
	}
	if strings.TrimSpace(req.RuntimeMode) != "" && strings.TrimSpace(req.RuntimeMode) != "external" {
		return nil, fmt.Errorf("review does not currently support runtime_mode=%s", strings.TrimSpace(req.RuntimeMode))
	}
	if strings.TrimSpace(req.LLMProvider) != "" && strings.TrimSpace(req.LLMProvider) != "openai" {
		return nil, fmt.Errorf("review does not currently support provider=%s", strings.TrimSpace(req.LLMProvider))
	}
	deck, err := os.ReadFile(req.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read local PPT: %w", err)
	}

	s.emit(ctx, "review_lint", "running", "Running PPT structural checks")
	structure, err := lintPPTX(req.FilePath, deck)
	if err != nil {
		s.emit(ctx, "review_lint", "failed", "PPT structural checks failed")
		return nil, fmt.Errorf("PPT structural checks failed: %w", err)
	}
	s.emit(ctx, "review_lint", "completed", "PPT structural checks completed")

	result := &Result{
		Status:         statusPartial,
		DocumentType:   "pptx",
		FilePath:       req.FilePath,
		OverallScore:   structure.Score,
		StructureScore: structure.Score,
		Summary:        structure.Summary,
		Strengths:      append([]string(nil), structure.Strengths...),
		Issues:         append([]Issue(nil), structure.Issues...),
	}

	if !req.EnableVisual {
		result.Warnings = append(result.Warnings, "Visual review was disabled explicitly. The result only contains structural checks.")
		return finalizeResult(result, nil), nil
	}
	if s.converter == nil || s.visual == nil {
		result.Warnings = append(result.Warnings, "Visual review is not available. The result only contains structural checks.")
		return finalizeResult(result, nil), nil
	}

	s.emit(ctx, "review_pdf", "running", "Converting PPT to PDF")
	pdfPath, err := s.converter.Convert(ctx, req.FilePath)
	if err != nil {
		s.emit(ctx, "review_pdf", "failed", "PPT to PDF conversion failed. Falling back to structural checks")
		result.Warnings = append(result.Warnings, fmt.Sprintf("Visual review was skipped: %s. Returning structural checks only.", err.Error()))
		return finalizeResult(result, nil), nil
	}
	s.emit(ctx, "review_pdf", "completed", "PPT to PDF conversion completed")

	s.emit(ctx, "review_visual", "running", "Running visual quality review")
	visual, err := s.visual.ReviewPDF(ctx, pdfPath, structure)
	if err != nil {
		s.emit(ctx, "review_visual", "failed", "Visual review failed. Falling back to structural checks")
		result.Warnings = append(result.Warnings, fmt.Sprintf("Visual review failed: %s. Returning structural checks only.", err.Error()))
		return finalizeResult(result, nil), nil
	}
	s.emit(ctx, "review_visual", "completed", "Visual quality review completed")

	result.UsedVisual = true
	result.VisualScore = visual.Score
	result.OverallScore = clamp(int(float64(visual.Score)*0.7+float64(structure.Score)*0.3+0.5), 0, 100)
	result.Status = scoreStatus(result.OverallScore)
	result.Summary = strings.TrimSpace(visual.Summary)
	result.Strengths = compactStrings(append(result.Strengths, visual.Strengths...), 4)
	result.Issues = sortIssues(append(result.Issues, visual.Issues...))
	return finalizeResult(result, visual), nil
}

func finalizeResult(result *Result, visual *VisualResult) *Result {
	if result == nil {
		return nil
	}
	if !result.UsedVisual {
		result.Status = statusPartial
		result.VisualScore = 0
		result.OverallScore = clamp(result.StructureScore, 0, 100)
	}
	if strings.TrimSpace(result.Summary) == "" {
		if result.UsedVisual && visual != nil && strings.TrimSpace(visual.Summary) != "" {
			result.Summary = strings.TrimSpace(visual.Summary)
		} else if len(result.Issues) == 0 {
			result.Summary = "Structural checks did not find any obvious issues."
		} else {
			result.Summary = fmt.Sprintf("Found %d issues that should be addressed first.", len(result.Issues))
		}
	}
	result.Strengths = compactStrings(result.Strengths, 4)
	result.Issues = sortIssues(result.Issues)
	result.Warnings = compactStrings(result.Warnings, 4)
	return result
}

func scoreStatus(score int) string {
	switch {
	case score >= 85:
		return statusExcellent
	case score >= 70:
		return statusGood
	default:
		return statusNeedsWork
	}
}

func (s *Service) emit(ctx context.Context, step, status, content string) {
	if s == nil || s.reporter == nil {
		return
	}
	s.reporter.EmitReviewProgress(ctx, step, status, strings.TrimSpace(content))
}

func topIssues(items []Issue, limit int) []Issue {
	if limit <= 0 || len(items) <= limit {
		return append([]Issue(nil), items...)
	}
	out := append([]Issue(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		return severityRank(out[i].Severity) > severityRank(out[j].Severity)
	})
	return out[:limit]
}
