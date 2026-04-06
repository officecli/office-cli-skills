package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func RenderResult(w io.Writer, result GenerateResult, jsonOutput bool) error {
	if jsonOutput {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	if _, err := fmt.Fprintf(w, "生成完成！已保存至 %s\n", result.FilePath); err != nil {
		return err
	}
	if result.Published {
		if _, err := fmt.Fprintf(w, "在线访问地址：%s；访问密码：%s\n", result.AccessURL, result.Password); err != nil {
			return err
		}
		if strings.TrimSpace(result.ExpiresAt) != "" {
			if _, err := fmt.Fprintf(w, "链接有效期至：%s\n", result.ExpiresAt); err != nil {
				return err
			}
		}
	}
	for _, warning := range result.Warnings {
		if strings.TrimSpace(warning) == "" {
			continue
		}
		if _, err := fmt.Fprintf(w, "提示：%s\n", warning); err != nil {
			return err
		}
	}
	return nil
}

func RenderReviewResult(w io.Writer, result ReviewResult, jsonOutput bool) error {
	if jsonOutput {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	if _, err := fmt.Fprintf(w, "评估完成：总分 %d（%s）\n", result.OverallScore, result.Status); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "结构分：%d；视觉分：%d；已使用视觉评审：%t\n", result.StructureScore, result.VisualScore, result.UsedVisual); err != nil {
		return err
	}
	if strings.TrimSpace(result.Summary) != "" {
		if _, err := fmt.Fprintf(w, "结论：%s\n", result.Summary); err != nil {
			return err
		}
	}
	for _, issue := range topReviewIssues(result.Issues, 3) {
		line := issue.Title
		if strings.TrimSpace(issue.Message) != "" {
			line = fmt.Sprintf("%s：%s", issue.Title, issue.Message)
		}
		if len(issue.SlideNumbers) > 0 {
			line = fmt.Sprintf("%s（页码 %v）", line, issue.SlideNumbers)
		}
		if _, err := fmt.Fprintf(w, "问题：%s\n", line); err != nil {
			return err
		}
	}
	for _, warning := range result.Warnings {
		if strings.TrimSpace(warning) == "" {
			continue
		}
		if _, err := fmt.Fprintf(w, "提示：%s\n", warning); err != nil {
			return err
		}
	}
	return nil
}

func topReviewIssues(items []ReviewIssue, limit int) []ReviewIssue {
	if limit <= 0 || len(items) <= limit {
		return append([]ReviewIssue(nil), items...)
	}
	return append([]ReviewIssue(nil), items[:limit]...)
}
