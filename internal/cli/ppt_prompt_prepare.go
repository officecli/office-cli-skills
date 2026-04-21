package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/officecli/officecli/engine"
	generateengine "github.com/officecli/officecli/engine/generate"
)

const pptPromptEnrichmentSchema = `{
  "type":"object",
  "additionalProperties":false,
  "required":["prompt","assumptions"],
  "properties":{
    "prompt":{"type":"string"},
    "assumptions":{
      "type":"array",
      "items":{"type":"string"}
    }
  }
}`

type pptPromptEnrichmentResult struct {
	Prompt      string   `json:"prompt"`
	Assumptions []string `json:"assumptions"`
}

func (a *App) preparePPTPrompt(ctx context.Context, llm GeneratorLLMClient, job GenerateJob, progress engine.ProgressEmitter) (GenerateJob, error) {
	if job.DocumentType != engine.DocumentTypePPTX || llm == nil {
		return job, nil
	}

	envelope, _, err := generateengine.ParsePromptEnvelope(job.Prompt)
	if err != nil {
		return job, err
	}
	basePrompt := strings.TrimSpace(envelope.Prompt)
	if basePrompt == "" {
		basePrompt = strings.TrimSpace(job.Topic)
	}
	if !shouldEnrichPPTPrompt(job.Topic, basePrompt) {
		return job, nil
	}

	emitProgress(ctx, progress, progressStepPlanPrepare, "running", "Expanding a short PPT brief into a fuller generation prompt")
	result, err := enrichPPTPrompt(ctx, llm, job, basePrompt)
	if err != nil {
		emitProgress(ctx, progress, progressStepPlanPrepare, "completed", "Prompt expansion skipped and generation will continue")
		job.Warnings = append(job.Warnings, engine.GenerateIssue{
			Code:    "WARN_PROMPT_ENRICHMENT_FALLBACK",
			Field:   "prompt",
			Message: "The PPT request looked too brief, but automatic prompt expansion failed, so generation continued with the original wording.",
		})
		return job, nil
	}
	if strings.TrimSpace(result.Prompt) == "" || samePromptMeaning(basePrompt, result.Prompt) {
		emitProgress(ctx, progress, progressStepPlanPrepare, "completed", "Prompt expansion determined the brief was already usable")
		return job, nil
	}

	if strings.TrimSpace(job.OriginalPrompt) == "" {
		job.OriginalPrompt = job.Prompt
	}
	envelope.Prompt = strings.TrimSpace(result.Prompt)
	job.Prompt = marshalPreparedPrompt(job.Prompt, envelope)
	job.Warnings = append(job.Warnings, engine.GenerateIssue{
		Code:    "WARN_PROMPT_ENRICHED",
		Field:   "prompt",
		Message: buildPromptEnrichedWarning(result.Assumptions),
	})
	emitProgress(ctx, progress, progressStepPlanPrepare, "completed", "PPT prompt expansion completed")
	return job, nil
}

func enrichPPTPrompt(ctx context.Context, llm GeneratorLLMClient, job GenerateJob, basePrompt string) (pptPromptEnrichmentResult, error) {
	req := buildPPTPromptEnrichmentRequest(job, basePrompt)
	response, err := llm.CompleteStructured(ctx, req)
	if err == nil {
		if result, decodeErr := decodePPTPromptEnrichment(response); decodeErr == nil {
			return result, nil
		}
	}
	response, err = llm.CompleteJSON(ctx, req.Messages)
	if err != nil {
		return pptPromptEnrichmentResult{}, err
	}
	return decodePPTPromptEnrichment(response)
}

func buildPPTPromptEnrichmentRequest(job GenerateJob, basePrompt string) engine.StructuredCompletionRequest {
	userLanguage := detectPromptLanguageName(basePrompt + " " + job.Topic)
	scenario := detectPromptPreparationScenario(job.Topic, basePrompt)
	return engine.StructuredCompletionRequest{
		Messages: []engine.LLMMessage{
			{
				Role:    "system",
				Content: "You improve underspecified PPT generation requests for OfficeCLI. Preserve the original intent, add only framing guidance, never invent factual claims, and return JSON that matches the schema exactly.",
			},
			{
				Role: "user",
				Content: fmt.Sprintf(`Rewrite the PPT request below into a fuller internal generation brief.

Topic: %s
Current prompt: %s
Requested deck language: %s
Requested audience: %s
Requested style: %s
User prompt language: %s
Scenario: %s

Requirements:
- Keep the rewritten brief in %s.
- Preserve the original subject and intent exactly.
- Add missing guidance for audience fit, storyline, slide density, image usage, and the closing slide style.
- Ask for short card headings and readable body copy, and explicitly avoid crowded cards or clipped text.
- Do not invent statistics, history, product claims, or any precise facts that the user did not provide.
- For game, hobby, culture, science, education, or general explainer topics, avoid executive-summary, rollout, owner, milestone, or next-step business framing. Prefer what it is, why it stands out, core mechanics or examples, who it suits, how to start, and key takeaways.
- For business, product, market, or operations topics, keep a professional decision-oriented deck style.
- Output one rewritten prompt string that is detailed enough for direct PPT generation, plus a short list of assumptions you added.`, strings.TrimSpace(job.Topic), strings.TrimSpace(basePrompt), blankAsNotSpecified(job.Language), blankAsNotSpecified(job.Audience), blankAsNotSpecified(job.Style), userLanguage, scenario, userLanguage),
			},
		},
		Schema: engine.StructuredSchema{
			Name:        "ppt_prompt_enrichment",
			Description: "Expanded PPT generation brief for an underspecified prompt.",
			JSONSchema:  []byte(pptPromptEnrichmentSchema),
			Strict:      true,
		},
	}
}

func decodePPTPromptEnrichment(raw string) (pptPromptEnrichmentResult, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var result pptPromptEnrichmentResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return pptPromptEnrichmentResult{}, err
	}
	result.Prompt = strings.TrimSpace(result.Prompt)
	if result.Prompt == "" {
		return pptPromptEnrichmentResult{}, fmt.Errorf("enriched prompt is empty")
	}
	out := make([]string, 0, len(result.Assumptions))
	for _, item := range result.Assumptions {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	result.Assumptions = out
	return result, nil
}

func shouldEnrichPPTPrompt(topic, prompt string) bool {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return false
	}
	if samePromptMeaning(topic, prompt) {
		return true
	}
	length := utf8.RuneCountInString(prompt)
	if length <= 28 {
		return true
	}
	if isGenericPPTPrompt(prompt) {
		return true
	}
	if length <= 64 && !hasPPTPromptDetailSignal(prompt) {
		return true
	}
	return false
}

func isGenericPPTPrompt(prompt string) bool {
	normalized := strings.ToLower(strings.TrimSpace(prompt))
	return containsAnyText(normalized,
		"介绍 ", "介绍这", "介绍一下", "简单介绍", "概述", "概览",
		"ppt about", "deck about", "slides about", "introduction to", "overview of", "tell me about", "make a ppt", "create a ppt",
		"game introduction", "product introduction", "介绍这款游戏", "介绍这个游戏", "游戏介绍")
}

func hasPPTPromptDetailSignal(prompt string) bool {
	normalized := strings.ToLower(strings.TrimSpace(prompt))
	return containsAnyText(normalized,
		"audience", "for ", "include", "emphasize", "compare", "timeline", "history", "feature", "features", "use case", "why", "how",
		"面向", "适合", "包含", "重点", "突出", "对比", "历史", "玩法", "机制", "亮点", "入门", "步骤", "原因", "影响", "总结")
}

func detectPromptPreparationScenario(topic, prompt string) string {
	normalized := strings.ToLower(strings.TrimSpace(topic + " " + prompt))
	switch {
	case containsAnyText(normalized, "market", "business review", "operations", "fundraising", "roadshow", "pricing", "saas", "市场", "经营", "运营", "融资", "路演", "定价"):
		return "business"
	case containsAnyText(normalized, "game", "minecraft", "游戏", "玩法", "movie", "anime", "文化", "science", "科普", "history", "历史", "museum", "book"):
		return "explainer"
	default:
		return "general"
	}
}

func detectPromptLanguageName(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "English"
	}
	han := 0
	latin := 0
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.IsLetter(r):
			latin++
		}
	}
	if han > latin/2 {
		return "Simplified Chinese"
	}
	return "English"
}

func marshalPreparedPrompt(original string, envelope generateengine.PromptEnvelope) string {
	trimmed := strings.TrimSpace(original)
	if strings.HasPrefix(trimmed, "{") && strings.Contains(trimmed, `"prompt"`) {
		data, err := json.Marshal(envelope)
		if err == nil {
			return string(data)
		}
	}
	return strings.TrimSpace(envelope.Prompt)
}

func samePromptMeaning(left, right string) bool {
	return normalizePromptComparison(left) == normalizePromptComparison(right)
}

func normalizePromptComparison(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), unicode.Is(unicode.Han, r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

func buildPromptEnrichedWarning(assumptions []string) string {
	message := "The PPT request was automatically expanded with audience, storyline, and layout guidance because the original prompt was too brief."
	if len(assumptions) == 0 {
		return message
	}
	trimmed := make([]string, 0, 2)
	for _, item := range assumptions {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		trimmed = append(trimmed, item)
		if len(trimmed) >= 2 {
			break
		}
	}
	if len(trimmed) == 0 {
		return message
	}
	return message + " Added assumptions: " + strings.Join(trimmed, "; ") + "."
}

func blankAsNotSpecified(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "not specified"
	}
	return value
}

func containsAnyText(content string, parts ...string) bool {
	for _, part := range parts {
		if part != "" && strings.Contains(content, strings.ToLower(part)) {
			return true
		}
	}
	return false
}
