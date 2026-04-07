package review

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultReviewModel = "gpt-5.4-mini"

type OpenAIReviewer struct {
	baseURL   string
	apiKey    string
	model     string
	client    *http.Client
	rasterize func(ctx context.Context, pdfPath string, maxPages int) ([]visualPageImage, error)
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
		baseURL:   baseURL,
		apiKey:    strings.TrimSpace(apiKey),
		model:     model,
		client:    &http.Client{Timeout: timeout},
		rasterize: rasterizePDFPagesWithSoffice,
	}
}

func (c *OpenAIReviewer) ReviewPDF(ctx context.Context, pdfPath string, structure StructureReport) (*VisualResult, error) {
	fileID, err := c.uploadPDF(ctx, pdfPath)
	if err != nil {
		if shouldRetryInlinePDF(err) {
			data, readErr := os.ReadFile(pdfPath)
			if readErr != nil {
				return nil, fmt.Errorf("打开 PDF 失败：%w", readErr)
			}
			visual, inlineErr := c.requestInlineReview(ctx, filepath.Base(pdfPath), data, structure)
			if inlineErr == nil {
				return visual, nil
			}
			return c.requestImageReview(ctx, pdfPath, structure, inlineErr)
		}
		return c.requestImageReview(ctx, pdfPath, structure, err)
	}
	visual, err := c.requestReview(ctx, []map[string]any{
		{
			"type":    "input_file",
			"file_id": fileID,
		},
	}, structure)
	if err == nil {
		return visual, nil
	}
	return c.requestImageReview(ctx, pdfPath, structure, err)
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

func (c *OpenAIReviewer) requestInlineReview(ctx context.Context, fileName string, pdfData []byte, structure StructureReport) (*VisualResult, error) {
	if strings.TrimSpace(fileName) == "" {
		fileName = "deck.pdf"
	}
	return c.requestReview(ctx, []map[string]any{
		{
			"type":      "input_file",
			"filename":  fileName,
			"file_data": "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(pdfData),
		},
	}, structure)
}

func (c *OpenAIReviewer) requestReview(ctx context.Context, fileInputs []map[string]any, structure StructureReport) (*VisualResult, error) {
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
				"content": append(fileInputs, map[string]any{
					"type": "input_text",
					"text": visualUserPrompt(structure),
				}),
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
	parsed, err := parseVisualResultJSON(text)
	if err != nil {
		return nil, fmt.Errorf("解析 visual review JSON 失败：%w", err)
	}
	return &parsed, nil
}

func (c *OpenAIReviewer) requestImageReview(ctx context.Context, pdfPath string, structure StructureReport, upstreamErr error) (*VisualResult, error) {
	if c == nil || c.rasterize == nil {
		return nil, upstreamErr
	}
	images, err := c.rasterize(ctx, pdfPath, 8)
	if err != nil {
		if upstreamErr != nil {
			return nil, fmt.Errorf("%w；图片回退失败：%v", upstreamErr, err)
		}
		return nil, err
	}
	if len(images) == 0 {
		if upstreamErr != nil {
			return nil, upstreamErr
		}
		return nil, fmt.Errorf("未生成可用于视觉评审的页面图片")
	}
	visual, err := c.requestChatImageReview(ctx, images, structure)
	if err != nil && upstreamErr != nil {
		return nil, fmt.Errorf("%w；图片回退失败：%v", upstreamErr, err)
	}
	return visual, err
}

func (c *OpenAIReviewer) requestChatImageReview(ctx context.Context, images []visualPageImage, structure StructureReport) (*VisualResult, error) {
	content := make([]map[string]any, 0, len(images)+1)
	content = append(content, map[string]any{
		"type": "text",
		"text": visualImageUserPrompt(structure, len(images)),
	})
	for _, image := range images {
		mime := strings.TrimSpace(image.MIME)
		if mime == "" {
			mime = "image/png"
		}
		content = append(content, map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(image.Data),
			},
		})
	}
	payload := map[string]any{
		"model": c.model,
		"messages": []map[string]any{
			{
				"role":    "system",
				"content": visualSystemPrompt(),
			},
			{
				"role":    "user",
				"content": content,
			},
		},
		"response_format": map[string]any{
			"type": "json_object",
		},
		"stream": true,
	}
	text, err := c.chatCompletionStream(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("chat visual review 失败：%w", err)
	}
	parsed, err := parseVisualResultJSON(text)
	if err != nil {
		return nil, fmt.Errorf("解析 chat visual review JSON 失败：%w", err)
	}
	return &parsed, nil
}

func (c *OpenAIReviewer) chatCompletionStream(ctx context.Context, payload map[string]any) (string, error) {
	resp, err := c.postStream(ctx, c.baseURL+"/chat/completions", payload)
	if err != nil {
		return "", err
	}
	defer resp.Close()

	reader := bufio.NewReader(resp)
	var content strings.Builder
	var eventData []string

	flushEvent := func() error {
		if len(eventData) == 0 {
			return nil
		}
		payload := strings.Join(eventData, "\n")
		eventData = eventData[:0]
		if strings.TrimSpace(payload) == "" {
			return nil
		}
		if strings.TrimSpace(payload) == "[DONE]" {
			return io.EOF
		}

		var chunk struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return fmt.Errorf("decode streaming chat response: %w", err)
		}
		if chunk.Error != nil {
			return fmt.Errorf("streaming chat response failed: %s", strings.TrimSpace(chunk.Error.Message))
		}
		for _, choice := range chunk.Choices {
			content.WriteString(choice.Delta.Content)
		}
		return nil
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			if flushErr := flushEvent(); flushErr != nil {
				if flushErr == io.EOF {
					break
				}
				return "", flushErr
			}
		} else if strings.HasPrefix(trimmed, "data:") {
			eventData = append(eventData, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		}
		if err == io.EOF {
			if flushErr := flushEvent(); flushErr != nil && flushErr != io.EOF {
				return "", flushErr
			}
			break
		}
	}

	if content.Len() == 0 {
		return "", fmt.Errorf("chat response is empty")
	}
	return content.String(), nil
}

func (c *OpenAIReviewer) postStream(ctx context.Context, url string, payload map[string]any) (io.ReadCloser, error) {
	body, _, err := c.doPost(ctx, url, payload)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (c *OpenAIReviewer) doPost(ctx context.Context, url string, payload map[string]any) (io.ReadCloser, func(), error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.apiKey))
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	closeBody := func() {
		_ = resp.Body.Close()
	}
	if resp.StatusCode < 300 {
		return resp.Body, closeBody, nil
	}
	body, err := io.ReadAll(resp.Body)
	closeBody()
	if err != nil {
		return nil, nil, err
	}
	return nil, nil, fmt.Errorf("llm request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func shouldRetryInlinePDF(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "status=404") ||
		strings.Contains(msg, "404 page not found") ||
		strings.Contains(msg, "files 响应缺少 file id")
}

func extractResponseText(body []byte) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("解析 responses 响应失败：%w", err)
	}
	if text := pickResponseText(payload["output_text"]); text != "" {
		return text, nil
	}
	if outputs, ok := payload["output"].([]any); ok {
		for _, output := range outputs {
			outputMap, ok := output.(map[string]any)
			if !ok {
				continue
			}
			if text := pickResponseText(outputMap["text"]); text != "" {
				return text, nil
			}
			contents, ok := outputMap["content"].([]any)
			if !ok {
				continue
			}
			for _, content := range contents {
				contentMap, ok := content.(map[string]any)
				if !ok {
					continue
				}
				if text := pickResponseText(contentMap["text"]); text != "" {
					return text, nil
				}
				if text := pickResponseText(contentMap["value"]); text != "" {
					return text, nil
				}
			}
		}
	}
	if choices, ok := payload["choices"].([]any); ok {
		for _, choice := range choices {
			choiceMap, ok := choice.(map[string]any)
			if !ok {
				continue
			}
			message, ok := choiceMap["message"].(map[string]any)
			if !ok {
				continue
			}
			if text := pickResponseText(message["content"]); text != "" {
				return text, nil
			}
		}
	}
	return "", fmt.Errorf("responses 响应缺少文本结果")
}

func pickResponseText(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"value", "text"} {
			if text := pickResponseText(typed[key]); text != "" {
				return text
			}
		}
	case []any:
		for _, item := range typed {
			if text := pickResponseText(item); text != "" {
				return text
			}
		}
	}
	return ""
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

func visualImageUserPrompt(structure StructureReport, pageCount int) string {
	return fmt.Sprintf("下面按页顺序提供这份 PPT 的页面截图，共 %d 页。请基于整套页面截图评估视觉与表达质量，页码按截图顺序从 1 开始。结构 lint 仅作为辅助上下文，不要机械复述。\n\n结构分：%d\n结构摘要：%s\n结构问题数：%d\n\n必须只输出一个 JSON 对象，字段固定为 score、summary、strengths、issues。", pageCount, structure.Score, structure.Summary, len(structure.Issues))
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

func parseVisualResultJSON(text string) (VisualResult, error) {
	var strict VisualResult
	if err := json.Unmarshal([]byte(text), &strict); err == nil {
		strict.Score = clamp(strict.Score, 0, 100)
		strict.Strengths = compactStrings(strict.Strengths, 4)
		strict.Issues = sortIssues(strict.Issues)
		strict.Summary = strings.TrimSpace(strict.Summary)
		return strict, nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return VisualResult{}, err
	}
	result := VisualResult{
		Score:     clamp(asInt(payload["score"]), 0, 100),
		Summary:   firstNonEmptyString(payload["summary"], payload["overall"], payload["message"]),
		Strengths: asStringSlice(payload["strengths"]),
	}
	if result.Summary == "" {
		if summaryMap, ok := payload["summary"].(map[string]any); ok {
			result.Summary = firstNonEmptyString(summaryMap["overall"], summaryMap["summary"], summaryMap["message"])
		}
	}
	if items, ok := payload["issues"].([]any); ok {
		result.Issues = make([]Issue, 0, len(items))
		for idx, item := range items {
			issueMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			message := firstNonEmptyString(issueMap["message"], issueMap["problem"], issueMap["detail"])
			title := firstNonEmptyString(issueMap["title"], issueMap["problem"], issueMap["message"])
			code := firstNonEmptyString(issueMap["code"], issueMap["type"])
			if code == "" {
				code = fmt.Sprintf("VISUAL_ISSUE_%d", idx+1)
			}
			result.Issues = append(result.Issues, Issue{
				Severity:     firstNonEmptyString(issueMap["severity"], "medium"),
				Code:         code,
				Title:        trimRunesForReview(title, 24),
				Message:      message,
				SlideNumbers: asIntSlice(issueMap["slide_numbers"], issueMap["pages"], issueMap["page"]),
				Suggestion:   firstNonEmptyString(issueMap["suggestion"], issueMap["fix"], issueMap["recommendation"]),
			})
		}
	}
	result.Score = clamp(result.Score, 0, 100)
	result.Strengths = compactStrings(result.Strengths, 4)
	result.Issues = sortIssues(result.Issues)
	result.Summary = strings.TrimSpace(result.Summary)
	return result, nil
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case map[string]any:
			for _, key := range []string{"overall", "summary", "message", "text", "value"} {
				if got := firstNonEmptyString(typed[key]); got != "" {
					return got
				}
			}
		}
	}
	return ""
}

func asStringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text := firstNonEmptyString(item); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func asInt(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		return 0
	}
}

func asIntSlice(values ...any) []int {
	out := make([]int, 0, 2)
	for _, value := range values {
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				if parsed := asInt(item); parsed > 0 {
					out = append(out, parsed)
				}
			}
		default:
			if parsed := asInt(typed); parsed > 0 {
				out = append(out, parsed)
			}
		}
	}
	return out
}

func trimRunesForReview(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) == 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return strings.TrimSpace(string(runes[:limit]))
}
