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
	if _, err := fmt.Fprintf(w, "Generation completed. Saved to %s\n", result.FilePath); err != nil {
		return err
	}
	if strings.TrimSpace(result.LocalPreviewPath) != "" {
		if _, err := fmt.Fprintf(w, "Local preview: %s\n", result.LocalPreviewPath); err != nil {
			return err
		}
	}
	if strings.TrimSpace(result.LocalPreviewDataPath) != "" {
		if _, err := fmt.Fprintf(w, "Local preview data: %s\n", result.LocalPreviewDataPath); err != nil {
			return err
		}
	}
	if result.Published {
		if _, err := fmt.Fprintf(w, "Preview URL: %s; password: %s\n", result.AccessURL, result.Password); err != nil {
			return err
		}
		if strings.TrimSpace(result.ExpiresAt) != "" {
			if _, err := fmt.Fprintf(w, "Preview expires at: %s\n", result.ExpiresAt); err != nil {
				return err
			}
		}
	}
	if accessSummary := formatAccessSummary(result); strings.TrimSpace(accessSummary) != "" {
		if _, err := fmt.Fprintf(w, "Access: %s\n", accessSummary); err != nil {
			return err
		}
	}
	for _, warning := range nonQuotaWarnings(result.Warnings) {
		if strings.TrimSpace(warning) == "" {
			continue
		}
		if _, err := fmt.Fprintf(w, "Warning: %s\n", warning); err != nil {
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
	if _, err := fmt.Fprintf(w, "Review completed: total score %d (%s)\n", result.OverallScore, result.Status); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Structure score: %d; visual score: %d; visual review enabled: %t\n", result.StructureScore, result.VisualScore, result.UsedVisual); err != nil {
		return err
	}
	if strings.TrimSpace(result.Summary) != "" {
		if _, err := fmt.Fprintf(w, "Summary: %s\n", result.Summary); err != nil {
			return err
		}
	}
	for _, issue := range topReviewIssues(result.Issues, 3) {
		line := issue.Title
		if strings.TrimSpace(issue.Message) != "" {
			line = fmt.Sprintf("%s: %s", issue.Title, issue.Message)
		}
		if len(issue.SlideNumbers) > 0 {
			line = fmt.Sprintf("%s (slides %v)", line, issue.SlideNumbers)
		}
		if _, err := fmt.Fprintf(w, "Issue: %s\n", line); err != nil {
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

func topReviewIssues(items []ReviewIssue, limit int) []ReviewIssue {
	if limit <= 0 || len(items) <= limit {
		return append([]ReviewIssue(nil), items...)
	}
	return append([]ReviewIssue(nil), items[:limit]...)
}

func formatAccessSummary(result GenerateResult) string {
	if len(quotaWarnings(result.Warnings)) == 0 {
		return ""
	}
	mode := strings.ToLower(strings.TrimSpace(result.AccessMode))
	switch mode {
	case "hosted":
		return fmt.Sprintf("hosted mode; %d credits remaining", result.CreditBalance)
	case "paid", "reward", "free":
		parts := []string{
			fmt.Sprintf("%s mode", mode),
			fmt.Sprintf("%d generations remaining", result.Remaining),
		}
		if result.FreeRemaining > 0 && mode != "free" {
			parts = append(parts, fmt.Sprintf("trial %d", result.FreeRemaining))
		}
		if result.RewardRemaining > 0 && mode != "reward" {
			parts = append(parts, fmt.Sprintf("reward %d", result.RewardRemaining))
		}
		if result.PaidQuotaRemaining > 0 && mode != "paid" {
			parts = append(parts, fmt.Sprintf("paid key %d", result.PaidQuotaRemaining))
		}
		return strings.Join(parts, "; ")
	default:
		fallback := make([]string, 0, len(result.Warnings))
		for _, warning := range quotaWarnings(result.Warnings) {
			trimmed := strings.TrimSpace(strings.TrimSuffix(warning, "."))
			if trimmed != "" {
				fallback = append(fallback, trimmed)
			}
		}
		return strings.Join(fallback, "; ")
	}
}

func nonQuotaWarnings(warnings []string) []string {
	if len(warnings) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if isQuotaWarning(warning) {
			continue
		}
		filtered = append(filtered, warning)
	}
	return filtered
}

func quotaWarnings(warnings []string) []string {
	if len(warnings) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if isQuotaWarning(warning) {
			filtered = append(filtered, warning)
		}
	}
	return filtered
}

func isQuotaWarning(warning string) bool {
	trimmed := strings.TrimSpace(warning)
	if trimmed == "" {
		return false
	}
	for _, prefix := range []string{
		"Current mode: ",
		"Free trial quota on this machine: ",
		"Reward quota: ",
		"Paid quota on current key: ",
	} {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}
