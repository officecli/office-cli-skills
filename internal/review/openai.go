package review

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultReviewModel = "gpt-5.4-mini"

type OpenAIReviewer struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewOpenAIReviewer(baseURL, apiKey, model string, timeoutSec int) *OpenAIReviewer {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = defaultReviewModel
	}
	timeout := 60 * time.Second
	if timeoutSec > 0 {
		timeout = time.Duration(timeoutSec) * time.Second
	}
	return &OpenAIReviewer{
		baseURL: baseURL,
		apiKey:  strings.TrimSpace(apiKey),
		model:   model,
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *OpenAIReviewer) ReviewPDF(ctx context.Context, pdfPath string, structure StructureReport) (*VisualResult, error) {
	fileID, err := c.uploadPDF(ctx, pdfPath)
	if err != nil {
		return nil, err
	}
	return c.requestReview(ctx, fileID, structure)
}

func (c *OpenAIReviewer) uploadPDF(ctx context.Context, pdfPath string) (string, error) {
	file, err := os.Open(pdfPath)
	if err != nil {
		return "", fmt.Errorf("打开 PDF 失败：%w", err)
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("purpose", "user_data"); err != nil {
		return "", err
	}
	part, err := writer.CreateFormFile("file", filepath.Base(pdfPath))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("写入上传内容失败：%w", err)
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/files", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("上传 PDF 到 OpenAI 失败：status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("解析 files 响应失败：%w", err)
	}
	if strings.TrimSpace(payload.ID) == "" {
		return "", fmt.Errorf("files 响应缺少 file id")
	}
	return payload.ID, nil
}

func (c *OpenAIReviewer) requestReview(ctx context.Context, fileID string, structure StructureReport) (*VisualResult, error) {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"score", "summary", "strengths", "issues"},
		"properties": map[string]any{
			"score":   map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
			"summary": map[string]any{"type": "string"},
			"strengths": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"issues": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"severity", "code", "title", "message", "slide_numbers", "suggestion"},
					"properties": map[string]any{
						"severity": map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
						"code":     map[string]any{"type": "string"},
						"title":    map[string]any{"type": "string"},
						"message":  map[string]any{"type": "string"},
						"slide_numbers": map[string]any{
							"type":  "array",
							"items": map[string]any{"type": "integer", "minimum": 1},
						},
						"suggestion": map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	request := map[string]any{
		"model": c.model,
		"input": []map[string]any{
			{
				"role": "system",
				"content": []map[string]any{{
					"type": "input_text",
					"text": visualSystemPrompt(),
				}},
			},
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type":    "input_file",
						"file_id": fileID,
					},
					{
						"type": "input_text",
						"text": visualUserPrompt(structure),
					},
				},
			},
		},
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "ppt_quality_review",
				"strict": true,
				"schema": schema,
			},
		},
	}

	raw, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OpenAI visual review 失败：status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	text, err := extractResponseText(body)
	if err != nil {
		return nil, err
	}
	var parsed VisualResult
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, fmt.Errorf("解析 visual review JSON 失败：%w", err)
	}
	parsed.Score = clamp(parsed.Score, 0, 100)
	parsed.Strengths = compactStrings(parsed.Strengths, 4)
	parsed.Issues = sortIssues(parsed.Issues)
	return &parsed, nil
}

func extractResponseText(body []byte) (string, error) {
	var payload struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("解析 responses 响应失败：%w", err)
	}
	if strings.TrimSpace(payload.OutputText) != "" {
		return strings.TrimSpace(payload.OutputText), nil
	}
	for _, output := range payload.Output {
		for _, content := range output.Content {
			if strings.TrimSpace(content.Text) != "" {
				return strings.TrimSpace(content.Text), nil
			}
		}
	}
	return "", fmt.Errorf("responses 响应缺少文本结果")
}

func visualSystemPrompt() string {
	return strings.TrimSpace(`你是一名严格的 PPT 质量审稿人。

请从以下五个维度给出 0-100 的总分，并输出结构化结论：
1. 叙事与受众匹配（25）
2. 信息层级与可读性（25）
3. 版式一致性与对齐（20）
4. 单页信息密度控制（15）
5. 图表/数据表达清晰度（15）

要求：
- 只输出 JSON，不要输出额外解释。
- 问题必须具体，尽量指出页码。
- severity 只允许 high / medium / low。
- strengths 控制在 2-4 条。
- issues 只保留最影响成品质量的问题。`)
}

func visualUserPrompt(structure StructureReport) string {
	return fmt.Sprintf("请评估这份 PPT 的视觉与表达质量。结构 lint 仅作为辅助上下文，不要机械复述。\n\n结构分：%d\n结构摘要：%s\n结构问题数：%d\n", structure.Score, structure.Summary, len(structure.Issues))
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
