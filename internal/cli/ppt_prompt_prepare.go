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
	if job.DocumentType != engine.DocumentTypePPTX || llm == nil || job.Mode != generateengine.ModeFast {
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
		if detectPromptPreparationScenario(job.Topic, basePrompt) == "explainer" && !job.StyleSpecified {
			job.Style = ""
		}
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
		if detectPromptPreparationScenario(job.Topic, basePrompt) == "explainer" && !job.StyleSpecified {
			job.Style = ""
		}
		return job, nil
	}
	if strings.TrimSpace(result.Prompt) == "" || samePromptMeaning(basePrompt, result.Prompt) {
		emitProgress(ctx, progress, progressStepPlanPrepare, "completed", "Prompt expansion determined the brief was already usable")
		if detectPromptPreparationScenario(job.Topic, basePrompt) == "explainer" && !job.StyleSpecified {
			job.Style = ""
		}
		return job, nil
	}

	envelope.Prompt = strings.TrimSpace(result.Prompt)
	job.Prompt = marshalPreparedPrompt(job.Prompt, envelope)
	if strings.TrimSpace(job.OriginalPrompt) == "" {
		job.OriginalPrompt = basePrompt
	}
	if detectPromptPreparationScenario(job.Topic, basePrompt) == "explainer" && !job.StyleSpecified {
		job.Style = ""
	}
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
	audienceLocked := job.Audience != "" || hasExplicitPPTAudienceSignal(basePrompt)
	pageLocked := hasExplicitPPTPageSignal(basePrompt)
	styleLocked := job.StyleSpecified || hasExplicitPPTStyleSignal(basePrompt)
	imageLocked := !job.EnableImages || hasExplicitPPTImageSignal(basePrompt)
	requestedStyle := ""
	if job.StyleSpecified {
		requestedStyle = job.Style
	}

	additionalRequirements := `- Keep the rewritten brief in ` + userLanguage + `.
- Preserve the original subject and intent exactly.
- Add missing guidance for audience fit, storyline, slide density, image usage, and the closing slide style.
- Do not invent statistics, history, product claims, or any precise facts that the user did not provide.`
	if scenario == "explainer" {
		additionalRequirements += `
- This is an explainer/game-style PPT. Keep it direct and beginner-friendly instead of consulting-style.
- The rewritten brief must explicitly cover: audience familiarity, a 6-8 slide target, short titles and short card headings, a rule to preserve complete visible wording and split/reflow instead of clipping text, image strategy, and a non-business ending style.
- Prefer this default slide arc: Cover -> What It Is -> Core Ways to Play -> Why It Stands Out -> Example or Gameplay Visual -> Who It Suits -> How to Start.
- Do not ask for or imply agenda, contents, chapter divider, rollout, owner, milestone, decision, or executive-summary slides.
- Default visual direction is editorial-light.
- When images are enabled, allow one cover hero image and keep the total image budget at 2-3 strong related visuals, concentrated on the cover and the example/gameplay slide.
- When images are disabled, merge the example/gameplay material into the body and keep the deck close to 6 slides.
- For Minecraft or voxel-sandbox topics, any fallback visual should explicitly prefer blocky voxel terrain, crafting, biomes, shelters, and cubic building, while avoiding hand-painted fantasy scenes, workshop illustration, and corporate-style diagrams.`
	} else {
		additionalRequirements += `
- For business, product, market, or operations topics, keep a professional decision-oriented deck style.
- Keep card headings short and readable, and prefer splitting dense content into an extra slide over shrinking or clipping visible text.`
	}

	lockRules := fmt.Sprintf(`Existing user constraints you must preserve:
- audience_already_specified=%t
- page_count_already_specified=%t
- style_already_specified=%t
- image_preference_already_specified=%t
- cli_images_enabled=%t

If a value is already specified, keep it and do not replace it with a new default. Only fill what is missing.`, audienceLocked, pageLocked, styleLocked, imageLocked, job.EnableImages)

	return engine.StructuredCompletionRequest{
		Messages: []engine.LLMMessage{
			{
				Role:    "system",
				Content: "You improve underspecified PPT generation requests for OfficeCLI. Preserve intent, add only missing guidance, never invent facts, and return JSON that matches the schema exactly.",
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

%s

Requirements:
%s`, strings.TrimSpace(job.Topic), strings.TrimSpace(basePrompt), blankAsNotSpecified(job.Language), blankAsNotSpecified(job.Audience), blankAsNotSpecified(requestedStyle), userLanguage, scenario, lockRules, additionalRequirements),
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
	if length <= 72 && !hasPPTPromptDetailSignal(prompt) {
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

func hasExplicitPPTAudienceSignal(prompt string) bool {
	normalized := strings.ToLower(strings.TrimSpace(prompt))
	return containsAnyText(normalized,
		"for beginners", "for kids", "for parents", "for players", "for teachers", "for students", "for leadership", "for exec", "for internal team",
		"面向", "给新手", "给入门", "给学生", "给老师", "给家长", "给玩家", "适合谁", "受众", "观众")
}

func hasExplicitPPTPageSignal(prompt string) bool {
	normalized := strings.ToLower(strings.TrimSpace(prompt))
	return containsAnyText(normalized,
		"slide", "slides", "page", "pages", "6-8", "6 页", "7 页", "8 页", "6页", "7页", "8页", "页左右", "页以内")
}

func hasExplicitPPTStyleSignal(prompt string) bool {
	normalized := strings.ToLower(strings.TrimSpace(prompt))
	return containsAnyText(normalized,
		"editorial-light", "tech-contrast", "executive-dark", "training-manual", "editorial", "magazine", "minimal", "professional", "playful",
		"简洁", "专业", "杂志感", "编辑感", "科技感", "商务风", "轻量", "清爽")
}

func hasExplicitPPTImageSignal(prompt string) bool {
	normalized := strings.ToLower(strings.TrimSpace(prompt))
	return containsAnyText(normalized,
		"with image", "with images", "no image", "no images", "hero image", "visual example", "image-heavy",
		"带图", "带图片", "不要图", "不要图片", "无图", "封面图", "示意图", "视觉页")
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
	message := "The PPT request was automatically expanded with audience, storyline, and density guidance because the original prompt was too brief."
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
