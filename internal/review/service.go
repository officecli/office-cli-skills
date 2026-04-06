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
		return nil, fmt.Errorf("review 目前只支持 pptx")
	}
	if strings.TrimSpace(req.RuntimeMode) != "" && strings.TrimSpace(req.RuntimeMode) != "external" {
		return nil, fmt.Errorf("runtime_mode=%s 暂不支持 review", strings.TrimSpace(req.RuntimeMode))
	}
	if strings.TrimSpace(req.LLMProvider) != "" && strings.TrimSpace(req.LLMProvider) != "openai" {
		return nil, fmt.Errorf("provider=%s 暂不支持 review", strings.TrimSpace(req.LLMProvider))
	}
	deck, err := os.ReadFile(req.FilePath)
	if err != nil {
		return nil, fmt.Errorf("读取本地 PPT 失败：%w", err)
	}

	s.emit(ctx, "review_lint", "running", "正在执行 PPT 结构检查")
	structure, err := lintPPTX(req.FilePath, deck)
	if err != nil {
		s.emit(ctx, "review_lint", "failed", "PPT 结构检查失败")
		return nil, fmt.Errorf("PPT 结构检查失败：%w", err)
	}
	s.emit(ctx, "review_lint", "completed", "PPT 结构检查完成")

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
		result.Warnings = append(result.Warnings, "已显式关闭视觉评审，当前结果仅包含结构检查。")
		return finalizeResult(result, nil), nil
	}
	if s.converter == nil || s.visual == nil {
		result.Warnings = append(result.Warnings, "视觉评审组件未启用，当前结果仅包含结构检查。")
		return finalizeResult(result, nil), nil
	}

	s.emit(ctx, "review_pdf", "running", "正在将 PPT 转换为 PDF")
	pdfPath, err := s.converter.Convert(ctx, req.FilePath)
	if err != nil {
		s.emit(ctx, "review_pdf", "failed", "PPT 转 PDF 失败，已降级为结构检查")
		result.Warnings = append(result.Warnings, fmt.Sprintf("视觉评审已跳过：%s；当前为结构检查结果。", err.Error()))
		return finalizeResult(result, nil), nil
	}
	s.emit(ctx, "review_pdf", "completed", "PPT 转 PDF 完成")

	s.emit(ctx, "review_visual", "running", "正在执行视觉质量评审")
	visual, err := s.visual.ReviewPDF(ctx, pdfPath, structure)
	if err != nil {
		s.emit(ctx, "review_visual", "failed", "视觉评审失败，已降级为结构检查")
		result.Warnings = append(result.Warnings, fmt.Sprintf("视觉评审失败：%s；当前为结构检查结果。", err.Error()))
		return finalizeResult(result, nil), nil
	}
	s.emit(ctx, "review_visual", "completed", "视觉质量评审完成")

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
			result.Summary = "结构检查未发现明显问题。"
		} else {
			result.Summary = fmt.Sprintf("共发现 %d 个需要优先修正的问题。", len(result.Issues))
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
