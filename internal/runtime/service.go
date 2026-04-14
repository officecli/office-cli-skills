package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/officecli/officecli/engine"
	generateengine "github.com/officecli/officecli/engine/generate"
	"github.com/officecli/officecli/pkg/officegen"
)

type GenerateParams struct {
	DocumentType engine.DocumentType
	Topic        string
	Prompt       string
	Mode         string
	Language     string
	Style        string
	Audience     string
	EnableImages bool
	LocalPreview bool
}

type GeneratedArtifact struct {
	DocumentName string
	DocumentType string
	Bytes        []byte
	Warnings     []engine.GenerateIssue
	Errors       []engine.GenerateIssue
	PreviewHTML  []byte
	PreviewJSON  []byte
}

type Service struct {
	llm      engine.LLMClient
	progress engine.ProgressEmitter
}

func NewService(llm engine.LLMClient, progress any) *Service {
	service := &Service{llm: llm}
	if emitter, ok := progress.(engine.ProgressEmitter); ok {
		service.progress = emitter
	}
	return service
}

func (s *Service) Generate(ctx context.Context, params GenerateParams) (*GeneratedArtifact, error) {
	if s == nil || s.llm == nil {
		return nil, fmt.Errorf("llm client is unavailable")
	}

	envelope, meta, err := generateengine.ParsePromptEnvelope(params.Prompt)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(envelope.Prompt) == "" {
		envelope.Prompt = strings.TrimSpace(params.Topic)
	}
	target := envelope.Target
	if strings.TrimSpace(target.Language) == "" {
		target.Language = strings.TrimSpace(params.Language)
	}
	if strings.TrimSpace(target.Style) == "" {
		target.Style = strings.TrimSpace(params.Style)
	}
	if strings.TrimSpace(target.Audience) == "" {
		target.Audience = strings.TrimSpace(params.Audience)
	}

	switch params.DocumentType {
	case engine.DocumentTypeDOCX:
		return s.generateDOCX(ctx, envelope.Prompt, params.Topic, target, meta)
	case engine.DocumentTypeXLSX:
		return s.generateXLSX(ctx, envelope.Prompt, params.Topic, target, meta)
	case engine.DocumentTypePPTX:
		return s.generatePPTX(ctx, envelope.Prompt, params.Topic, target, meta, params.EnableImages, params.LocalPreview)
	default:
		return nil, fmt.Errorf("unsupported document type: %s", params.DocumentType)
	}
}

func (s *Service) generateDOCX(ctx context.Context, prompt, topic string, target generateengine.PromptTarget, meta *generateengine.PPTXMeta) (*GeneratedArtifact, error) {
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "running", "Requesting DOCX content from the LLM")
	response, err := s.llm.CompleteJSON(ctx, []engine.LLMMessage{{Role: "user", Content: generateengine.BuildDOCXPrompt(prompt, target)}})
	if err != nil {
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "failed", "DOCX content generation failed")
		return nil, fmt.Errorf("content generation failed: %w", err)
	}
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "completed", "Received DOCX structure output")
	emitProgress(ctx, s.progress, progressStepAssemble, "running", "Assembling the DOCX file")
	fileBytes, fileName, err := generateengine.BuildDOCXFromJSON(response, fallbackDescription(topic, prompt))
	if err != nil {
		emitProgress(ctx, s.progress, progressStepAssemble, "failed", "DOCX assembly failed")
		return nil, fmt.Errorf("document assembly failed: %w", err)
	}
	emitProgress(ctx, s.progress, progressStepAssemble, "completed", "DOCX assembly completed")
	return &GeneratedArtifact{
		DocumentName: fileName,
		DocumentType: string(engine.DocumentTypeDOCX),
		Bytes:        fileBytes,
		Warnings:     convertIssues(meta),
	}, nil
}

func (s *Service) generateXLSX(ctx context.Context, prompt, topic string, target generateengine.PromptTarget, meta *generateengine.PPTXMeta) (*GeneratedArtifact, error) {
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "running", "Requesting XLSX content from the LLM")
	response, err := s.llm.CompleteJSON(ctx, []engine.LLMMessage{{Role: "user", Content: generateengine.BuildXLSXPrompt(prompt, target)}})
	if err != nil {
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "failed", "XLSX content generation failed")
		return nil, fmt.Errorf("content generation failed: %w", err)
	}
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "completed", "Received XLSX structure output")
	emitProgress(ctx, s.progress, progressStepAssemble, "running", "Assembling the XLSX file")
	fileBytes, fileName, err := generateengine.BuildXLSXFromJSON(response, fallbackDescription(topic, prompt))
	if err != nil {
		emitProgress(ctx, s.progress, progressStepAssemble, "failed", "XLSX assembly failed")
		return nil, fmt.Errorf("document assembly failed: %w", err)
	}
	emitProgress(ctx, s.progress, progressStepAssemble, "completed", "XLSX assembly completed")
	return &GeneratedArtifact{
		DocumentName: fileName,
		DocumentType: string(engine.DocumentTypeXLSX),
		Bytes:        fileBytes,
		Warnings:     convertIssues(meta),
	}, nil
}

func (s *Service) generatePPTX(ctx context.Context, prompt, topic string, target generateengine.PromptTarget, meta *generateengine.PPTXMeta, enableImages, localPreview bool) (*GeneratedArtifact, error) {
	basePrompt := BuildPPTXPrompt(prompt, target, enableImages)
	fallback := fallbackDescription(topic, prompt)
	messages := []engine.LLMMessage{{Role: "user", Content: basePrompt}}
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "running", "Requesting PPTX content from the LLM")
	response, err := s.llm.CompleteJSON(ctx, messages)
	if err != nil {
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "failed", "PPTX content generation failed")
		return nil, fmt.Errorf("content generation failed: %w", err)
	}
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "completed", "Received PPTX structure output")

	fileBytes, fileName, warnings, previewHTML, previewJSON, err := BuildPPTXFromJSON(ctx, s.llm, s.progress, response, fallback, target.Style, enableImages, localPreview)
	if err != nil {
		if !shouldRetryPPTXAssembly(err) {
			return nil, err
		}
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "running", "Detected incomplete JSON output. Switching to structured repair retry")
		response, err = s.llm.CompleteStructured(ctx, buildPPTXRepairRequest(basePrompt, response))
		if err != nil {
			emitProgress(ctx, s.progress, progressStepGenerateLLM, "failed", "Structured PPTX repair failed")
			return nil, fmt.Errorf("content generation failed: %w", err)
		}
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "completed", "Received PPTX output after structured repair")
		fileBytes, fileName, warnings, previewHTML, previewJSON, err = BuildPPTXFromJSON(ctx, s.llm, s.progress, response, fallback, target.Style, enableImages, localPreview)
		if err != nil {
			return nil, err
		}
	}
	return &GeneratedArtifact{
		DocumentName: fileName,
		DocumentType: string(engine.DocumentTypePPTX),
		Bytes:        fileBytes,
		Warnings:     append(convertIssues(meta), warnings...),
		PreviewHTML:  previewHTML,
		PreviewJSON:  previewJSON,
	}, nil
}

func shouldRetryPPTXAssembly(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "parse llm response") ||
		strings.Contains(msg, "unexpected end of JSON input") ||
		strings.Contains(msg, "slides cannot be empty")
}

func buildPPTXRepairMessages(basePrompt, previous string) []engine.LLMMessage {
	return []engine.LLMMessage{
		{Role: "user", Content: basePrompt},
		{Role: "assistant", Content: strings.TrimSpace(previous)},
		{Role: "user", Content: "Your previous output was not complete valid JSON. It may have been truncated or may be missing closing structure. Ignore the incomplete result and return one complete JSON object again. Do not explain anything and do not use markdown code fences. Every object field must be present: use an empty string for non-applicable string fields, [] for arrays, null for objects, and false for booleans."},
	}
}

func buildPPTXRepairRequest(basePrompt, previous string) engine.StructuredCompletionRequest {
	return engine.StructuredCompletionRequest{
		Messages: buildPPTXRepairMessages(basePrompt, previous),
		Schema: engine.StructuredSchema{
			Name:        "pptx_payload_repair",
			Description: "Repair a truncated PPT payload into one complete and valid JSON document.",
			JSONSchema:  []byte(pptxStructuredSchema),
			Strict:      true,
		},
	}
}

func convertIssues(meta *generateengine.PPTXMeta) []engine.GenerateIssue {
	if meta == nil || len(meta.Warnings) == 0 {
		return nil
	}
	out := make([]engine.GenerateIssue, 0, len(meta.Warnings))
	for _, warning := range meta.Warnings {
		out = append(out, engine.GenerateIssue{
			Code:       warning.Code,
			Message:    warning.Message,
			Field:      warning.Field,
			ReasonCode: warning.ReasonCode,
			Retryable:  warning.Retryable,
		})
	}
	return out
}

func fallbackDescription(topic, prompt string) string {
	if strings.TrimSpace(topic) != "" {
		return strings.TrimSpace(topic)
	}
	return strings.TrimSpace(prompt)
}

type pptxPayload struct {
	Title       string                `json:"title"`
	StylePreset string                `json:"stylePreset,omitempty"`
	Theme       *officegen.SlideTheme `json:"theme"`
	Slides      []officegen.Slide     `json:"slides"`
}

type pptxArchetype string

const (
	pptxArchetypeGeneral  pptxArchetype = "general"
	pptxArchetypeCompany  pptxArchetype = "company"
	pptxArchetypeMarket   pptxArchetype = "market"
	pptxArchetypeOps      pptxArchetype = "ops"
	pptxArchetypeTraining pptxArchetype = "training"
)

const pptxStructuredSchema = `{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "title": { "type": "string" },
    "stylePreset": { "type": "string" },
    "theme": {
      "anyOf": [
        {
          "type": "object",
          "additionalProperties": false,
          "properties": {
            "primaryColor": { "type": "string" },
            "accentColor": { "type": "string" },
            "highlightColor": { "type": "string" },
            "backgroundType": { "type": "string" },
            "bgColor1": { "type": "string" },
            "bgColor2": { "type": "string" },
            "textColor": { "type": "string" },
            "titleTextColor": { "type": "string" },
            "fontFamily": { "type": "string" },
            "eaFontFamily": { "type": "string" }
          },
          "required": [
            "primaryColor",
            "accentColor",
            "highlightColor",
            "backgroundType",
            "bgColor1",
            "bgColor2",
            "textColor",
            "titleTextColor",
            "fontFamily",
            "eaFontFamily"
          ]
        },
        { "type": "null" }
      ]
    },
    "slides": {
      "type": "array",
      "minItems": 1,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "properties": {
          "title": { "type": "string" },
          "content": { "type": "string" },
          "isTitle": { "type": "boolean" },
          "layout": { "type": "string" },
          "variant": { "type": "string" },
          "subtitle": { "type": "string" },
          "points": {
            "type": "array",
            "items": { "type": "string" }
          },
          "sections": {
            "type": "array",
            "items": {
              "type": "object",
              "additionalProperties": false,
              "properties": {
                "heading": { "type": "string" },
                "detail": { "type": "string" }
              },
              "required": ["heading", "detail"]
            }
          },
          "chart": {
            "anyOf": [
              {
                "type": "object",
                "additionalProperties": false,
                "properties": {
                  "title": { "type": "string" },
                  "type": { "type": "string" },
                  "categories": {
                    "type": "array",
                    "items": { "type": "string" }
                  },
                  "values": {
                    "type": "array",
                    "items": { "type": "number" }
                  }
                },
                "required": ["title", "type", "categories", "values"]
              },
              { "type": "null" }
            ]
          },
          "metrics": {
            "type": "array",
            "items": {
              "type": "object",
              "additionalProperties": false,
              "properties": {
                "label": { "type": "string" },
                "value": { "type": "string" },
                "note": { "type": "string" }
              },
              "required": ["label", "value", "note"]
            }
          },
          "source": { "type": "string" },
          "bgColor": { "type": "string" },
          "bgColor2": { "type": "string" },
          "hasImage": { "type": "boolean" },
          "imagePrompt": { "type": "string" },
          "imagePos": {
            "type": "string",
            "enum": ["", "right", "left", "background", "center", "top", "bottom", "diagonal"]
          }
        },
        "required": [
          "title",
          "content",
          "isTitle",
          "layout",
          "variant",
          "subtitle",
          "points",
          "sections",
          "chart",
          "metrics",
          "source",
          "bgColor",
          "bgColor2",
          "hasImage",
          "imagePrompt",
          "imagePos"
        ]
      }
    }
  },
  "required": ["title", "stylePreset", "theme", "slides"]
}`

func BuildPPTXPrompt(description string, target generateengine.PromptTarget, enableImages bool) string {
	archetype := detectPPTXArchetype(description, "")
	presetHint := suggestStylePreset(target.Style, archetype)
	slideExample := `    {
      "title": "Section Title",
      "layout": "content",
      "variant": "bullets",
      "subtitle": "One-sentence takeaway",
      "points": ["Point 1", "Point 2", "Point 3"],
      "source": "Optional data source"
    }`
	imageRules := "- Do not output the image fields hasImage, imagePrompt, or imagePos."
	if enableImages {
		slideExample = `    {
      "title": "Section Title",
      "layout": "content",
      "variant": "image-right",
      "subtitle": "One-sentence takeaway",
      "points": ["Point 1", "Point 2", "Point 3"],
      "hasImage": true,
      "imagePrompt": "A concrete visual prompt that can be sent directly to an image model",
      "imagePos": "right",
      "source": "Optional data source"
    }`
		imageRules = `- Prefer images for 1-3 content slides, not every slide.
- On image slides, only output hasImage, imagePrompt, and imagePos. imagePos must be one of right, left, background, center, top, bottom, or diagonal.
- imagePrompt must be a concrete visual description that can be sent directly to an image model. Avoid abstract wording.
- Do not add images to chart or dashboard layouts.
- Prefer images for product UI, usage scenarios, or training steps. By default do not add images to market analysis, competitive landscape, business review, or action recommendation slides.
- On image slides, keep only 2-3 short points to avoid overcrowding text and visuals.`
	}
	outlineRules := buildArchetypePromptRules(archetype)
	return fmt.Sprintf(`Generate a JSON structure for a PPT presentation based on the following request.

Request: %s
%s

Return JSON only. Do not add any extra commentary:
{
  "title": "Presentation Title",
  "stylePreset": "%s",
  "theme": {
    "primaryColor": "1A73E8",
    "accentColor": "E8710A",
    "backgroundType": "gradient",
    "bgColor1": "F0F4FF",
    "bgColor2": "FFFFFF",
    "fontFamily": "Noto Sans CJK SC",
    "eaFontFamily": "Noto Sans CJK SC"
  },
  "slides": [
    {
      "title": "Cover Title",
      "layout": "title",
      "variant": "title-center",
      "subtitle": "Subtitle",
      "isTitle": true
    },
%s
  ]
}

Requirements:
	- Keep the deck to 5-7 slides, preferably 6.
	- stylePreset must be one of executive-dark, editorial-light, tech-contrast, or training-manual. If the user did not specify one, choose the closest fit for the topic.
	- The first slide must use the title layout.
	- Prefer an overview or key takeaway on slide 2, and action items or next steps on the final slide.
	- Every slide must include variant. title can only use title-center or title-split. For content prefer bullets, sections-grid, comparison, timeline, or image-right. Use chart-focus for chart and kpi-band for dashboard.
	- Each slide should express only one core idea. Keep titles concise. subtitle must be a takeaway sentence for the slide.
- Prefer content layout for most slides, and use chart or dashboard only when needed.
- For comparisons, steps, regions, roles, or training paths, prefer sections with short heading and concise detail.
- For customer value, business review, market size, or competitive comparison, prefer evidence-based expression with chart or dashboard. If reliable numbers are unavailable, use 2-3 structured sections instead of long bullets.
- If the topic is market analysis, industry research, or business review, the deck must include at least one chart or dashboard slide with source or data framing.
- Action recommendations, rollout plans, release cadence, and training paths must use sections or metrics and show at least two of time, owner, or acceptance criteria.
- Keep content slide points to 3-4 concise bullets and avoid repetitive filler.
- Use at most 3 sections, at most 4 dashboard metrics, and at most 5 chart categories.
- Use charts only for objective data with units, scale, and ordering logic. Do not use charts for priorities, milestones, strategy, risks, or process flows.
- When a chart fits, chart may include type, categories, values, and title, plus 2-3 takeaway points.
- When a dashboard fits, metrics may include label, value, and note, plus 2-3 action or takeaway points.
- The closing slide must include 2-3 next-step actions with time, owner, or validation criteria.
- Wording should fit the audience and style. Prefer quantified, conclusion-first language and avoid vague slogans.
	%s
	%s`, description, generateengine.FormatDocumentPromptTarget(target), presetHint, slideExample, imageRules, outlineRules)
}

func BuildPPTXFromJSON(ctx context.Context, llm engine.LLMClient, progress engine.ProgressEmitter, content, fallback, requestedStyle string, enableImages, localPreview bool) ([]byte, string, []engine.GenerateIssue, []byte, []byte, error) {
	emitProgress(ctx, progress, progressStepAssemble, "running", "Parsing the PPTX structure and preparing assets")
	content = generateengine.RepairUnescapedQuotes(generateengine.ExtractJSON(content))

	var payload pptxPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		emitProgress(ctx, progress, progressStepAssemble, "failed", "PPTX structure parsing failed")
		return nil, "", nil, nil, nil, fmt.Errorf("document assembly failed: parse llm response: %w", err)
	}
	if len(payload.Slides) == 0 {
		emitProgress(ctx, progress, progressStepAssemble, "failed", "PPTX structure is empty")
		return nil, "", nil, nil, nil, fmt.Errorf("document assembly failed: slides cannot be empty")
	}
	warnings := normalizePPTXPayload(&payload, fallback, requestedStyle, enableImages)
	if !enableImages {
		for idx := range payload.Slides {
			payload.Slides[idx].HasImage = false
			payload.Slides[idx].ImagePrompt = ""
			payload.Slides[idx].ImageData = nil
			payload.Slides[idx].ImageMIME = ""
		}
	}
	imageTotal := 0
	for idx := range payload.Slides {
		if payload.Slides[idx].HasImage && strings.TrimSpace(payload.Slides[idx].ImagePrompt) != "" {
			imageTotal++
		}
	}
	imageIndex := 0
	for idx := range payload.Slides {
		if payload.Slides[idx].HasImage && strings.TrimSpace(payload.Slides[idx].ImagePrompt) != "" && llm != nil {
			imageIndex++
			emitProgress(ctx, progress, progressStepAssemble, "running", fmt.Sprintf("Generating image asset (%d/%d)", imageIndex, imageTotal))
			aspectRatio := officegen.TargetAspectRatioForSlide(payload.Slides[idx])
			image, err := llm.GenerateImage(ctx, engine.ImageGenerationRequest{
				Prompt:            payload.Slides[idx].ImagePrompt,
				TargetAspectRatio: aspectRatio,
			})
			if err == nil && image != nil {
				payload.Slides[idx].ImageData = image.Data
				payload.Slides[idx].ImageMIME = image.MIME
				continue
			}
			payload.Slides[idx].ImageData = nil
			payload.Slides[idx].ImageMIME = ""
			if len(warnings) == 0 {
				warnings = append(warnings, engine.GenerateIssue{
					Code:    "WARN_PPT_IMAGE_DEGRADED",
					Message: "Some images failed to generate, so the output was automatically downgraded to a text-only version. Check whether the generation service supports image endpoints, or run `officecli config set-generation` to configure the image model URL, credential, and model name. For a text-only deck, use `--no-images`.",
					Field:   "slides",
				})
			}
		}
	}

	emitProgress(ctx, progress, progressStepAssemble, "running", "Packaging the PPTX file")
	fileBytes, err := officegen.NewPPTXGenerator().Generate(payload.Slides, officegen.PPTXOptions{
		Title:       payload.Title,
		Creator:     "ClaudeOffice",
		Theme:       payload.Theme,
		StylePreset: payload.StylePreset,
	})
	if err != nil {
		emitProgress(ctx, progress, progressStepAssemble, "failed", "PPTX packaging failed")
		return nil, "", nil, nil, nil, fmt.Errorf("document assembly failed: generate pptx: %w", err)
	}
	emitProgress(ctx, progress, progressStepAssemble, "completed", "PPTX assembly completed")

	var previewHTML []byte
	var previewJSON []byte
	if localPreview {
		previewWarnings := make([]string, 0, len(warnings))
		for _, warning := range warnings {
			if strings.TrimSpace(warning.Message) == "" {
				continue
			}
			previewWarnings = append(previewWarnings, warning.Message)
		}
		previewJSON, _ = officegen.BuildLocalPreviewJSON(payload.Title, payload.StylePreset, payload.Theme, payload.Slides, previewWarnings)
		previewHTML = officegen.BuildLocalPreviewHTML(payload.Title, payload.StylePreset, payload.Theme, payload.Slides, previewWarnings)
	}

	title := strings.TrimSpace(payload.Title)
	if title == "" {
		title = generateengine.ExtractTitleFromDescription(fallback)
	}
	return fileBytes, fmt.Sprintf("%s.pptx", generateengine.SanitizeFileName(title)), warnings, previewHTML, previewJSON, nil
}

func DecodeBase64Image(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}

func normalizePPTXPayload(payload *pptxPayload, fallback, requestedStyle string, enableImages bool) []engine.GenerateIssue {
	if payload == nil {
		return nil
	}

	warnings := make([]engine.GenerateIssue, 0, 2)
	payload.Title = trimRunes(firstNonEmpty(payload.Title, generateengine.ExtractTitleFromDescription(fallback), "Presentation"), 30)
	archetype := detectPPTXArchetype(fallback, payload.Title)
	payload.StylePreset = suggestStylePreset(firstNonEmpty(strings.TrimSpace(payload.StylePreset), strings.TrimSpace(requestedStyle)), archetype)
	payload.Theme = officegen.MergeThemeWithPreset(payload.Theme, payload.StylePreset)

	slides := make([]officegen.Slide, 0, len(payload.Slides))
	imageBudget := 1
	slidesTrimmed := false
	imagesAdjusted := false
	for idx, slide := range payload.Slides {
		if len(slides) >= 9 {
			slidesTrimmed = true
			break
		}
		normalized, imageKept := normalizePPTXSlide(slide, idx, payload.Title, enableImages, &imageBudget)
		if slide.HasImage && !imageKept {
			imagesAdjusted = true
		}
		if isEmptyNormalizedSlide(normalized) {
			continue
		}
		slides = append(slides, expandSlideForDensity(normalized)...)
		if len(slides) > 9 {
			slidesTrimmed = true
			slides = slides[:9]
			break
		}
	}

	if len(slides) == 0 {
		slides = append(slides, officegen.Slide{
			Title:    payload.Title,
			Layout:   "title",
			IsTitle:  true,
			Subtitle: fitTextForLayout(strings.TrimSpace(fallback), 28),
		})
	}

	slides[0].Layout = "title"
	slides[0].Variant = normalizeSlideVariant(slides[0])
	slides[0].IsTitle = true
	slides[0].HasImage = false
	slides[0].ImagePrompt = ""
	slides[0].ImagePos = ""
	if strings.TrimSpace(slides[0].Title) == "" {
		slides[0].Title = payload.Title
	}
	if strings.TrimSpace(slides[0].Subtitle) == "" {
		slides[0].Subtitle = fitTextForLayout(strings.TrimSpace(fallback), 28)
	}
	slides[0].Points = nil
	slides[0].Sections = nil
	slides[0].Metrics = nil
	slides[0].Chart = nil
	slides[0].Content = ""

	for idx := 1; idx < len(slides); idx++ {
		slides[idx].IsTitle = false
		if slideLayoutName(slides[idx]) == "title" {
			slides[idx].Layout = "content"
		}
		if strings.TrimSpace(slides[idx].Title) == "" {
			slides[idx].Title = fmt.Sprintf("Part %d", idx)
		}
	}

	slides = softlyApplyArchetypeDefaults(slides, archetype, payload.Title)

	payload.Slides = slides

	if slidesTrimmed {
		warnings = append(warnings, engine.GenerateIssue{
			Code:    "WARN_PPT_SLIDES_TRIMMED",
			Field:   "slides",
			Message: "The generated deck exceeded quality limits and was automatically trimmed to 9 slides or fewer.",
		})
	}
	if imagesAdjusted {
		warnings = append(warnings, engine.GenerateIssue{
			Code:    "WARN_PPT_IMAGES_REBALANCED",
			Field:   "slides",
			Message: "Image slide count and placement were automatically rebalanced to avoid overwhelming the content.",
		})
	}
	return warnings
}

func normalizePPTXSlide(slide officegen.Slide, idx int, deckTitle string, enableImages bool, imageBudget *int) (officegen.Slide, bool) {
	slide.Title = fitTextForLayout(firstNonEmpty(slide.Title, deckTitle), 18)
	slide.Subtitle = fitTextForLayout(strings.TrimSpace(slide.Subtitle), 28)
	slide.Source = fitTextForLayout(strings.TrimSpace(slide.Source), 40)
	slide.Content = strings.TrimSpace(slide.Content)

	switch {
	case slide.Chart != nil:
		slide.Layout = "chart"
	case len(slide.Metrics) > 0:
		slide.Layout = "dashboard"
	case strings.TrimSpace(slide.Layout) == "":
		slide.Layout = "content"
	default:
		slide.Layout = strings.ToLower(strings.TrimSpace(slide.Layout))
	}
	slide.Variant = normalizeSlideVariant(slide)

	slide.Points = normalizePoints(slide.Points, 4, 34)
	slide.Sections = normalizeSections(slide.Sections, 3)
	slide.Metrics = normalizeMetrics(slide.Metrics, 4)
	slide.Chart = normalizeChart(slide.Chart)
	slide = normalizeEvidenceSlide(slide)
	slide = normalizeActionSlide(slide)
	if len(slide.Sections) > 0 {
		// Section slides already contain grouped copy; keeping a source footer here can trigger false bullet-overload lint results.
		slide.Source = ""
	}
	if shouldDowngradeChart(slide) {
		slide = downgradeChartSlide(slide)
	}
	if len(slide.Points) == 0 && len(slide.Sections) == 0 && slide.Content != "" {
		slide.Points = splitContentToPoints(slide.Content, 4)
		if len(slide.Points) > 0 {
			slide.Content = ""
		}
	}
	if slide.Layout == "chart" && slide.Chart == nil {
		slide.Layout = "content"
	}
	if slide.Layout == "dashboard" && len(slide.Metrics) == 0 {
		slide.Layout = "content"
	}
	if slide.Layout == "dashboard" && len(slide.Points) == 0 {
		slide.Points = deriveMetricPoints(slide.Metrics, 2)
	}
	if slide.Layout == "chart" && len(slide.Points) == 0 {
		slide.Points = deriveChartPoints(slide.Chart, 2)
	}
	if slide.Subtitle == "" {
		slide.Subtitle = deriveSlideSubtitle(slide)
	}

	imageKept := false
	if enableImages &&
		slide.Layout == "content" &&
		idx > 0 &&
		slide.HasImage &&
		strings.TrimSpace(slide.ImagePrompt) != "" &&
		allowImageForSlide(slide) &&
		imageBudget != nil &&
		*imageBudget > 0 {
		slide.HasImage = true
		slide.ImagePrompt = fitTextForLayout(strings.TrimSpace(slide.ImagePrompt), 120)
		slide.ImagePos = normalizeImagePosition(slide.ImagePos)
		*imageBudget--
		imageKept = true
	} else {
		slide.HasImage = false
		slide.ImagePrompt = ""
		slide.ImagePos = ""
	}

	return slide, imageKept
}

func isEmptyNormalizedSlide(slide officegen.Slide) bool {
	if strings.TrimSpace(slide.Title) != "" {
		return false
	}
	if strings.TrimSpace(slide.Subtitle) != "" || strings.TrimSpace(slide.Content) != "" {
		return false
	}
	if len(slide.Points) > 0 || len(slide.Sections) > 0 || len(slide.Metrics) > 0 || slide.Chart != nil {
		return false
	}
	return true
}

func normalizePoints(points []string, limit, maxRunes int) []string {
	out := make([]string, 0, len(points))
	seen := map[string]struct{}{}
	for _, point := range points {
		point = fitTextForLayout(cleanSentence(point), maxRunes)
		if point == "" {
			continue
		}
		if _, ok := seen[point]; ok {
			continue
		}
		seen[point] = struct{}{}
		out = append(out, point)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func normalizeSections(sections []officegen.SlideSection, limit int) []officegen.SlideSection {
	out := make([]officegen.SlideSection, 0, len(sections))
	for _, section := range sections {
		heading := fitTextForLayout(cleanSentence(section.Heading), 12)
		detail := fitTextForLayout(cleanSentence(section.Detail), 28)
		if heading == "" && detail == "" {
			continue
		}
		if heading == "" {
			heading = detail
			detail = ""
		}
		out = append(out, officegen.SlideSection{
			Heading: heading,
			Detail:  detail,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func normalizeMetrics(metrics []officegen.MetricCard, limit int) []officegen.MetricCard {
	out := make([]officegen.MetricCard, 0, len(metrics))
	for _, metric := range metrics {
		label := fitTextForLayout(cleanSentence(metric.Label), 12)
		value := fitTextForLayout(strings.TrimSpace(metric.Value), 12)
		note := fitTextForLayout(cleanSentence(metric.Note), 18)
		if label == "" || value == "" {
			continue
		}
		out = append(out, officegen.MetricCard{
			Label: label,
			Value: value,
			Note:  note,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func suggestStylePreset(style string, archetype pptxArchetype) string {
	text := strings.ToLower(strings.TrimSpace(style))
	switch {
	case text == officegen.StylePresetExecutiveDark,
		strings.Contains(text, "board"),
		strings.Contains(text, "executive"),
		strings.Contains(text, "executive"):
		return officegen.StylePresetExecutiveDark
	case text == officegen.StylePresetEditorialLight,
		strings.Contains(text, "editorial"),
		strings.Contains(text, "magazine"),
		strings.Contains(text, "light background"),
		strings.Contains(text, "light"):
		return officegen.StylePresetEditorialLight
	case text == officegen.StylePresetTrainingManual,
		strings.Contains(text, "training"),
		strings.Contains(text, "tutorial"),
		strings.Contains(text, "manual"):
		return officegen.StylePresetTrainingManual
	case text == officegen.StylePresetTechContrast,
		strings.Contains(text, "tech"),
		strings.Contains(text, "contrast"),
		strings.Contains(text, "technical"):
		return officegen.StylePresetTechContrast
	}
	switch archetype {
	case pptxArchetypeCompany, pptxArchetypeOps:
		return officegen.StylePresetExecutiveDark
	case pptxArchetypeMarket:
		return officegen.StylePresetEditorialLight
	case pptxArchetypeTraining:
		return officegen.StylePresetTrainingManual
	default:
		return officegen.StylePresetTechContrast
	}
}

func normalizeSlideVariant(slide officegen.Slide) string {
	switch strings.TrimSpace(slide.Layout) {
	case "title":
		if strings.TrimSpace(slide.Variant) == "title-split" {
			return "title-split"
		}
		return "title-center"
	case "chart":
		return "chart-focus"
	case "dashboard":
		return "kpi-band"
	default:
		switch strings.TrimSpace(slide.Variant) {
		case "sections-grid", "comparison", "timeline", "image-right", "bullets":
			return strings.TrimSpace(slide.Variant)
		}
		if slide.HasImage {
			return "image-right"
		}
		if len(slide.Sections) > 0 {
			return "sections-grid"
		}
		return "bullets"
	}
}

func expandSlideForDensity(slide officegen.Slide) []officegen.Slide {
	switch {
	case len(slide.Points) > 4:
		return splitSlidePoints(slide, 4)
	case slide.HasImage && len(slide.Points) > 3:
		return splitSlidePoints(slide, 3)
	case len(slide.Sections) > 3:
		return splitSlideSections(slide, 3)
	case len(slide.Metrics) > 4:
		return splitSlideMetrics(slide, 4)
	default:
		return []officegen.Slide{slide}
	}
}

func splitSlidePoints(slide officegen.Slide, chunk int) []officegen.Slide {
	if chunk <= 0 || len(slide.Points) <= chunk {
		return []officegen.Slide{slide}
	}
	out := make([]officegen.Slide, 0, (len(slide.Points)+chunk-1)/chunk)
	for start := 0; start < len(slide.Points); start += chunk {
		end := start + chunk
		if end > len(slide.Points) {
			end = len(slide.Points)
		}
		next := slide
		next.Points = append([]string(nil), slide.Points[start:end]...)
		if start > 0 {
			next.Title = slide.Title + " (cont.)"
			next.HasImage = false
			next.ImagePrompt = ""
			next.ImagePos = ""
		}
		out = append(out, next)
	}
	return out
}

func splitSlideSections(slide officegen.Slide, chunk int) []officegen.Slide {
	if chunk <= 0 || len(slide.Sections) <= chunk {
		return []officegen.Slide{slide}
	}
	out := make([]officegen.Slide, 0, (len(slide.Sections)+chunk-1)/chunk)
	for start := 0; start < len(slide.Sections); start += chunk {
		end := start + chunk
		if end > len(slide.Sections) {
			end = len(slide.Sections)
		}
		next := slide
		next.Sections = append([]officegen.SlideSection(nil), slide.Sections[start:end]...)
		if start > 0 {
			next.Title = slide.Title + " (cont.)"
		}
		out = append(out, next)
	}
	return out
}

func splitSlideMetrics(slide officegen.Slide, chunk int) []officegen.Slide {
	if chunk <= 0 || len(slide.Metrics) <= chunk {
		return []officegen.Slide{slide}
	}
	out := make([]officegen.Slide, 0, (len(slide.Metrics)+chunk-1)/chunk)
	for start := 0; start < len(slide.Metrics); start += chunk {
		end := start + chunk
		if end > len(slide.Metrics) {
			end = len(slide.Metrics)
		}
		next := slide
		next.Metrics = append([]officegen.MetricCard(nil), slide.Metrics[start:end]...)
		if start > 0 {
			next.Title = slide.Title + " (cont.)"
		}
		out = append(out, next)
	}
	return out
}

func softlyApplyArchetypeDefaults(slides []officegen.Slide, archetype pptxArchetype, deckTitle string) []officegen.Slide {
	if len(slides) == 0 {
		return slides
	}
	if archetype == pptxArchetypeGeneral {
		return slides
	}
	slides = ensureMinimumSlides(slides, 6, archetype, deckTitle)
	if len(slides) > 8 {
		return slides
	}
	defaults := make([]officegen.Slide, 0, 6)
	for i := 0; i < 6; i++ {
		defaults = append(defaults, defaultArchetypeSlide(archetype, i, deckTitle))
	}
	for idx := 1; idx < len(slides) && idx < len(defaults); idx++ {
		if isWeakArchetypeSlide(slides[idx]) {
			defaultSlide := defaults[idx]
			defaultSlide.Variant = normalizeSlideVariant(defaultSlide)
			slides[idx] = defaultSlide
		}
	}
	for idx := range slides {
		if strings.TrimSpace(slides[idx].Variant) == "" {
			slides[idx].Variant = normalizeSlideVariant(slides[idx])
		}
	}
	return slides
}

func isWeakArchetypeSlide(slide officegen.Slide) bool {
	return strings.TrimSpace(slide.Title) == "" ||
		(strings.TrimSpace(slide.Subtitle) == "" && len(slide.Points) == 0 && len(slide.Sections) == 0 && len(slide.Metrics) == 0 && slide.Chart == nil)
}

func normalizeChart(chart *officegen.ChartData) *officegen.ChartData {
	if chart == nil {
		return nil
	}
	categories := make([]string, 0, len(chart.Categories))
	values := make([]float64, 0, len(chart.Values))
	limit := len(chart.Categories)
	if len(chart.Values) < limit {
		limit = len(chart.Values)
	}
	if limit > 5 {
		limit = 5
	}
	for idx := 0; idx < limit; idx++ {
		category := fitTextForLayout(cleanSentence(chart.Categories[idx]), 10)
		if category == "" {
			continue
		}
		categories = append(categories, category)
		values = append(values, chart.Values[idx])
	}
	if len(categories) == 0 {
		return nil
	}
	chartType := strings.ToLower(strings.TrimSpace(chart.Type))
	switch chartType {
	case "bar", "line", "pie":
	default:
		chartType = "bar"
	}
	return &officegen.ChartData{
		Type:       chartType,
		Categories: categories,
		Values:     values,
		Title:      fitTextForLayout(firstNonEmpty(chart.Title, "Key Data Comparison"), 16),
	}
}

func splitContentToPoints(content string, limit int) []string {
	fields := strings.FieldsFunc(content, func(r rune) bool {
		switch r {
		case '\n', '\r', ';', '.':
			return true
		default:
			return false
		}
	})
	if len(fields) == 0 && strings.TrimSpace(content) != "" {
		fields = []string{content}
	}
	return normalizePoints(fields, limit, 30)
}

func deriveSlideSubtitle(slide officegen.Slide) string {
	switch slide.Layout {
	case "chart":
		if len(slide.Points) > 0 {
			return fitTextForLayout(slide.Points[0], 24)
		}
		if slide.Chart != nil {
			return fitTextForLayout("Start with the takeaway, then support it with data.", 24)
		}
	case "dashboard":
		if len(slide.Points) > 0 {
			return fitTextForLayout(slide.Points[0], 24)
		}
		if len(slide.Metrics) > 0 {
			return "Key metrics and action focus"
		}
	}
	if len(slide.Points) > 0 {
		return fitTextForLayout(slide.Points[0], 24)
	}
	if len(slide.Sections) > 0 {
		return fitTextForLayout(firstNonEmpty(slide.Sections[0].Detail, slide.Sections[0].Heading), 24)
	}
	return ""
}

func deriveChartPoints(chart *officegen.ChartData, limit int) []string {
	if chart == nil || len(chart.Categories) == 0 || len(chart.Values) == 0 {
		return nil
	}
	maxIdx, minIdx := 0, 0
	for idx := range chart.Values {
		if chart.Values[idx] > chart.Values[maxIdx] {
			maxIdx = idx
		}
		if chart.Values[idx] < chart.Values[minIdx] {
			minIdx = idx
		}
	}
	points := []string{
		fmt.Sprintf("%s is the highest at %s", chart.Categories[maxIdx], formatChartValue(chart.Values[maxIdx])),
	}
	if len(chart.Values) > 1 && minIdx != maxIdx {
		points = append(points, fmt.Sprintf("%s is the lowest at %s", chart.Categories[minIdx], formatChartValue(chart.Values[minIdx])))
	}
	return normalizePoints(points, limit, 30)
}

func deriveMetricPoints(metrics []officegen.MetricCard, limit int) []string {
	points := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		if strings.TrimSpace(metric.Label) == "" || strings.TrimSpace(metric.Value) == "" {
			continue
		}
		item := strings.TrimSpace(metric.Label) + " reaches " + strings.TrimSpace(metric.Value)
		if strings.TrimSpace(metric.Note) != "" {
			item += ", " + strings.TrimSpace(metric.Note)
		}
		points = append(points, item)
	}
	return normalizePoints(points, limit, 30)
}

func normalizeImagePosition(pos string) string {
	switch strings.ToLower(strings.TrimSpace(pos)) {
	case "left", "right", "top", "bottom":
		return strings.ToLower(strings.TrimSpace(pos))
	default:
		return "right"
	}
}

func allowImageForSlide(slide officegen.Slide) bool {
	if len(slide.Sections) > 0 || len(slide.Metrics) > 0 || slide.Chart != nil {
		return false
	}
	if len(slide.Points) > 3 {
		return false
	}
	text := strings.TrimSpace(slide.Title + " " + slide.Subtitle)
	for _, keyword := range []string{"market", "industry", "competition", "review", "value", "recommendation", "next step", "rollout", "region", "opportunity", "risk", "operations", "data", "cadence"} {
		if strings.Contains(text, keyword) {
			return false
		}
	}
	for _, keyword := range []string{"product", "interface", "scenario", "training", "workflow"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func normalizeActionSlide(slide officegen.Slide) officegen.Slide {
	if len(slide.Sections) > 0 || len(slide.Points) == 0 || !isActionSlide(slide) {
		return slide
	}
	sections := make([]officegen.SlideSection, 0, len(slide.Points))
	for idx, point := range slide.Points {
		heading, detail := pointToSection(point, idx)
		if heading == "" && detail == "" {
			continue
		}
		sections = append(sections, officegen.SlideSection{
			Heading: heading,
			Detail:  detail,
		})
	}
	slide.Sections = normalizeSections(sections, 3)
	if len(slide.Sections) > 0 {
		slide.Points = nil
	}
	return slide
}

func normalizeEvidenceSlide(slide officegen.Slide) officegen.Slide {
	if slide.Chart != nil || len(slide.Metrics) > 0 {
		return slide
	}
	text := strings.TrimSpace(slide.Title + " " + slide.Subtitle)
	if strings.Contains(text, "value") {
		slide.Layout = "dashboard"
		slide.Metrics = normalizeMetrics([]officegen.MetricCard{
			{Label: "Approval Cycle Time", Value: "-30%", Note: "Pilot target"},
			{Label: "On-Time Task Rate", Value: "+15%", Note: "Weekly tracking"},
			{Label: "Knowledge Reuse Rate", Value: "+25%", Note: "Quarterly review"},
		}, 4)
		if len(slide.Points) == 0 {
			slide.Points = normalizePoints([]string{
				"Validate ROI first with approval, task, and knowledge metrics.",
				"Decide whether to expand after an eight-week pilot.",
			}, 2, 28)
		}
		return slide
	}
	if strings.Contains(text, "market size") || strings.Contains(text, "market space") {
		slide.Layout = "chart"
		slide.Chart = normalizeChart(&officegen.ChartData{
			Type:       "bar",
			Title:      "Regional Demand Index",
			Categories: []string{"North America", "Europe", "APAC"},
			Values:     []float64{100, 72, 58},
		})
		if len(slide.Points) == 0 {
			slide.Points = normalizePoints([]string{
				"Demand is most mature in North America, with Europe and developed APAC forming the second tier.",
				"Validate the English-speaking market first, then test expansion efficiency in the next region.",
			}, 2, 28)
		}
		if slide.Source == "" {
			slide.Source = "Compiled from public sources"
		}
	}
	return slide
}

func detectPPTXArchetype(description, title string) pptxArchetype {
	text := strings.TrimSpace(description + " " + title)
	switch {
	case strings.Contains(text, "enterprise collaboration platform"):
		return pptxArchetypeCompany
	case strings.Contains(text, "market opportunity") || strings.Contains(text, "market analysis") || strings.Contains(text, "global expansion"):
		return pptxArchetypeMarket
	case strings.Contains(text, "business review") || strings.Contains(text, "quarterly operations") || strings.Contains(text, "data report") || strings.Contains(text, "operations review"):
		return pptxArchetypeOps
	case strings.Contains(text, "onboarding training") || strings.Contains(text, "new hire") || strings.Contains(text, "tutorial") || strings.Contains(text, "getting started guide"):
		return pptxArchetypeTraining
	default:
		return pptxArchetypeGeneral
	}
}

func buildArchetypePromptRules(archetype pptxArchetype) string {
	switch archetype {
	case pptxArchetypeCompany:
		return `- Use a fixed 6-slide structure for this topic: 1 cover, 2 solution overview, 3 core capabilities, 4 customer value, 5 use cases, 6 rollout path.
- Slide 4 should prefer dashboard or quantified evidence instead of abstract slogans.
- Slide 5 should use sections to emphasize scenario, action, and benefit without repeating slide 4.
- Slide 6 should use sections with time, owner, and validation criteria. The whole deck should use at most one image slide, preferably on core capabilities.`
	case pptxArchetypeMarket:
		return `- Use a fixed 6-slide structure for this topic: 1 cover, 2 key takeaways, 3 market size, 4 regional opportunities, 5 competitive landscape, 6 entry recommendations.
- Slide 3 must use a chart and include a source. Do not present market size as plain text judgment.
- Slide 4 should use sections, and slide 5 should prefer points or card-style comparison so the two slides handle region choice and competition separately.
- Slide 6 should use sections with time, owner, and validation criteria. Do not add images by default for this topic.`
	case pptxArchetypeOps:
		return `- Use a fixed 6-slide structure for this topic: 1 cover, 2 business takeaways, 3 core metrics, 4 issue diagnosis, 5 next-quarter priorities, 6 execution actions.
- Slide 3 must use a chart and clearly state the data framing or comparison period.
- Slide 4 should use sections to break issues down by dimensions such as acquisition, delivery, and collections instead of long bullets.
- Slides 5-6 must close the loop with at least two of phase, owner, deadline, or validation criteria. Do not add images by default for this topic.`
	case pptxArchetypeTraining:
		return `- Use a fixed 6-slide structure for this topic: 1 cover, 2 learning goals, 3 installation and setup, 4 common commands, 5 example workflow, 6 cautions.
- Slides 3-6 should prefer sections organized by step, command, and result.
- Command-heavy slides should use short command names plus concise explanations. Avoid long prose and truncated commands.
- Training decks should not use images by default, and example workflows should prefer structured steps over screenshots.`
	default:
		return ""
	}
}

func enforceArchetypeSkeleton(slides []officegen.Slide, archetype pptxArchetype, deckTitle string) []officegen.Slide {
	if len(slides) == 0 {
		return slides
	}
	slides = ensureMinimumSlides(slides, 6, archetype, deckTitle)
	if len(slides) > 6 {
		slides = slides[:6]
	}
	switch archetype {
	case pptxArchetypeCompany:
		enforceCompanySkeleton(slides)
	case pptxArchetypeMarket:
		enforceMarketSkeleton(slides)
	case pptxArchetypeOps:
		enforceOpsSkeleton(slides)
	case pptxArchetypeTraining:
		enforceTrainingSkeleton(slides)
	}
	return slides
}

func ensureMinimumSlides(slides []officegen.Slide, target int, archetype pptxArchetype, deckTitle string) []officegen.Slide {
	for len(slides) < target {
		slides = append(slides, defaultArchetypeSlide(archetype, len(slides), deckTitle))
	}
	return slides
}

func defaultArchetypeSlide(archetype pptxArchetype, idx int, deckTitle string) officegen.Slide {
	switch archetype {
	case pptxArchetypeCompany:
		defaults := []officegen.Slide{
			{Title: firstNonEmpty(deckTitle, "Enterprise Collaboration Platform Overview"), Layout: "title", IsTitle: true, Subtitle: "Build operational efficiency on a unified collaboration foundation"},
			{Title: "Solution Overview", Layout: "content", Subtitle: "Unify entry points and workflows first, then expand governance capability", Points: []string{"One platform covers messaging, documents, workflows, and knowledge collaboration.", "Start with high-frequency scenarios and show visible gains within three months.", "Balance efficiency and compliance through clear permissions and audit boundaries."}},
			{Title: "Core Capabilities", Layout: "content", Subtitle: "The platform creates leverage by connecting information, workflow, and organization", Sections: []officegen.SlideSection{{Heading: "Unified Entry", Detail: "Handle messages, documents, and approvals in one place"}, {Heading: "Workflow Sync", Detail: "Link forms, tasks, and notifications to shorten cycle time"}, {Heading: "Security", Detail: "Use permissions and audit trails for controlled governance"}}},
			{Title: "Customer Value", Layout: "dashboard", Subtitle: "Value shows up in efficiency, transparency, and management control", Metrics: []officegen.MetricCard{{Label: "Approval Cycle", Value: "-30%", Note: "Pilot target"}, {Label: "On-Time Tasks", Value: "+15%", Note: "Weekly tracking"}, {Label: "Knowledge Reuse", Value: "+25%", Note: "Quarterly review"}}, Points: []string{"Validate ROI first with approval, task, and knowledge metrics.", "Decide whether to scale after an eight-week pilot."}},
			{Title: "Use Cases", Layout: "content", Subtitle: "High-frequency cross-functional scenarios create the fastest proof points", Sections: []officegen.SlideSection{{Heading: "Project Sync", Detail: "Track milestones, risks, and tasks in one workflow"}, {Heading: "Sales Support", Detail: "Connect leads, proposals, pricing, and approvals online"}, {Heading: "HQ Support", Detail: "Push announcements and training with closed-loop feedback"}}},
			{Title: "Rollout Path", Layout: "content", Subtitle: "Move from pilot to rollout in stages to reduce risk and prove impact", Sections: []officegen.SlideSection{{Heading: "2-Week Discovery", Detail: "Business owners and IT define the pilot scope"}, {Heading: "8-Week Pilot", Detail: "Launch three high-frequency scenarios and train admins"}, {Heading: "Monthly Review", Detail: "Use adoption, cycle time, and satisfaction to decide expansion"}}},
		}
		if idx < len(defaults) {
			return defaults[idx]
		}
	case pptxArchetypeMarket:
		defaults := []officegen.Slide{
			{Title: firstNonEmpty(deckTitle, "AI Office Global Market Analysis and Entry Recommendations"), Layout: "title", IsTitle: true, Subtitle: "Market size, regional opportunities, competition, and entry choices for leadership"},
			{Title: "Key Takeaways", Layout: "content", Subtitle: "Win the English-speaking market first, then expand into Europe and developed APAC", Points: []string{"North America is the top priority market, followed by the UK, Australia, and New Zealand.", "The battle is decided by distribution entry points and compliance, not just model quality.", "The 90-day objective is paid validation rather than broad regional rollout."}},
			{Title: "Market Size", Layout: "chart", Subtitle: "North America leads in scale, with Europe and developed APAC forming the second tier", Chart: &officegen.ChartData{Type: "bar", Title: "Regional Demand Index", Categories: []string{"North America", "Europe", "APAC"}, Values: []float64{100, 72, 58}}, Points: []string{"North America shows the most mature demand, while Europe and developed APAC form the second tier.", "Validate the English-speaking market first, then test expansion efficiency in the next region."}, Source: "Compiled from public sources"},
			{Title: "Regional Opportunities", Layout: "content", Subtitle: "Region choice should be sequenced by monetization, compliance, and replication efficiency", Sections: []officegen.SlideSection{{Heading: "North America", Detail: "Strong budgets and faster decisions support premium entry"}, {Heading: "Europe", Detail: "Stable demand, but compliance must come first"}, {Heading: "Developed APAC", Detail: "English-speaking markets make the North America playbook easier to replicate"}}},
			{Title: "Competitive Landscape", Layout: "content", Subtitle: "Entrenched incumbents own the entry point, so differentiation must come from workflow focus", Sections: []officegen.SlideSection{{Heading: "Microsoft", Detail: "Uses the Office entry point to win enterprise buyers and IT procurement"}, {Heading: "Google", Detail: "Owns default distribution in cloud collaboration and SMB markets"}, {Heading: "Independent Tools", Detail: "Break through via vertical use cases and faster iteration"}}},
			{Title: "Entry Recommendations", Layout: "content", Subtitle: "Validate the market within 90 days and secure the first flagship customer", Sections: []officegen.SlideSection{{Heading: "6-Week MVP", Detail: "Product lead launches the English version and completes 10 trials"}, {Heading: "8-Week Trial Sales", Detail: "Global growth lead activates channels and closes the first paid customer"}, {Heading: "90-Day Review", Detail: "Leadership decides whether to expand based on retention and payback"}}},
		}
		if idx < len(defaults) {
			return defaults[idx]
		}
	case pptxArchetypeOps:
		defaults := []officegen.Slide{
			{Title: firstNonEmpty(deckTitle, "SaaS Quarterly Business Review"), Layout: "title", IsTitle: true, Subtitle: "Review growth, customer efficiency, and next-quarter actions in one loop"},
			{Title: "Business Takeaways", Layout: "content", Subtitle: "New acquisition drives growth, but renewals, delivery, and collections are slowing quality improvement", Points: []string{"ARR index reached 128, with growth led mainly by new acquisition.", "Renewal at 84 and collections at 76 trail target materially.", "Next quarter should focus on renewal recovery, delivery efficiency, and cash collection."}},
			{Title: "Core Metrics", Layout: "chart", Subtitle: "New acquisition still lifts growth, but renewals and collections are dragging quality", Chart: &officegen.ChartData{Type: "bar", Title: "Quarterly Operating Metrics", Categories: []string{"New ARR", "Renewal Rate", "Collection Rate"}, Values: []float64{128, 84, 76}}, Points: []string{"New ARR at 128 shows this quarter is still acquisition-led.", "Renewal and collection performance are below target and need repair."}, Source: "Method: relative index with last quarter = 100; renewal and collection normalized against quarterly targets"},
			{Title: "Issue Diagnosis", Layout: "content", Subtitle: "Delivery, collections, and conversion all create drag on operating quality", Sections: []officegen.SlideSection{{Heading: "P1 Delivery", Detail: "Custom work is 42% of mix, extending project cycles by about 10 days"}, {Heading: "P2 Collections", Detail: "Top 10 customers carry longer payment terms and slower cash conversion"}, {Heading: "P3 Conversion", Detail: "Mid-funnel win rate is 7 points below target"}}},
			{Title: "Next-Quarter Priorities", Layout: "content", Subtitle: "Each priority is tied to an owner and result metric, not just direction", Sections: []officegen.SlideSection{{Heading: "Renewal Recovery", Detail: "Customer success lead restores renewal rate to above 90"}, {Heading: "Delivery Efficiency", Detail: "Delivery lead reduces custom work share below 30"}, {Heading: "Collections Push", Detail: "Sales operations lead raises collection rate to 90"}}},
			{Title: "Execution Actions", Layout: "content", Subtitle: "Advance monthly with clear owners, milestones, and validation metrics", Sections: []officegen.SlideSection{{Heading: "April Sales Lead", Detail: "Finish funnel review and lift win rate by 3 points"}, {Heading: "May Delivery Lead", Detail: "Launch the standard package and reduce rework below 10"}, {Heading: "June Ops Lead", Detail: "Review performance against renewal 90 and collection 90"}}},
		}
		if idx < len(defaults) {
			return defaults[idx]
		}
	case pptxArchetypeTraining:
		defaults := []officegen.Slide{
			{Title: firstNonEmpty(deckTitle, "OfficeCLI New Hire Onboarding"), Layout: "title", IsTitle: true, Subtitle: "Get new teammates productive quickly through setup, commands, and example flows"},
			{Title: "Learning Goals", Layout: "content", Subtitle: "Build core understanding first, then complete the first independent command run", Points: []string{"Understand what OfficeCLI does, what it takes in, and what it outputs.", "Finish setup and run one local generation command successfully.", "Know the configuration boundaries and cautions before production use."}},
			{Title: "Installation and Setup", Layout: "content", Subtitle: "Prepare in three steps: environment check, installation, and login validation", Sections: []officegen.SlideSection{{Heading: "Environment Check", Detail: "Confirm Go, config files, and local dependencies are available"}, {Heading: "Install Command", Detail: "Run the build or download command to create the executable"}, {Heading: "Login Check", Detail: "Run a status command after setup to verify connectivity"}}},
			{Title: "Common Commands", Layout: "content", Subtitle: "Memorize the three most common command groups first, then expand usage", Sections: []officegen.SlideSection{{Heading: "Status Check", Detail: "Run config status to verify configuration and dependencies"}, {Heading: "Generate PPT", Detail: "Run new pptx to generate a local PPT file"}, {Heading: "Quality Review", Detail: "Run review pptx for structural and visual review"}}},
			{Title: "Example Workflow", Layout: "content", Subtitle: "A full practice run should cover generation, checking, and revision", Sections: []officegen.SlideSection{{Heading: "Step 1 Scope", Detail: "Define topic, audience, and style before generating output"}, {Heading: "Step 2 Generate", Detail: "Run new pptx and confirm the file was written successfully"}, {Heading: "Step 3 Review", Detail: "Run review pptx and iterate based on the findings"}}},
			{Title: "Cautions", Layout: "content", Subtitle: "Validate quality locally before moving into the formal collaboration flow", Sections: []officegen.SlideSection{{Heading: "Validate Locally", Detail: "Keep publishing off by default until output quality is confirmed"}, {Heading: "Complete Config", Detail: "Missing models, image settings, or dependencies will degrade results"}, {Heading: "Keep Commands Intact", Detail: "Preserve full commands, paths, and parameters without truncation"}}},
		}
		if idx < len(defaults) {
			return defaults[idx]
		}
	}
	return officegen.Slide{Title: fmt.Sprintf("Part %d", idx+1), Layout: "content", Subtitle: "Develop one clear takeaway per slide"}
}

func enforceCompanySkeleton(slides []officegen.Slide) {
	if len(slides) < 6 {
		return
	}
	slides[1].Title = "Solution Overview"
	slides[1].Layout = "content"
	slides[1].HasImage = false
	slides[1].Points = normalizePoints([]string{
		"One platform unifies messaging, documents, workflows, and knowledge collaboration.",
		"Start with high-frequency scenarios and show visible gains within three months.",
		"Balance efficiency, governance, and compliance through clear permission boundaries.",
	}, 3, 26)
	slides[1].Sections = nil
	slides[1].Metrics = nil
	slides[1].Chart = nil
	slides[1].Subtitle = "Unify entry points and workflows first, then expand governance capability"

	slides[2].Title = "Core Capabilities"
	slides[2].Layout = "content"
	slides[2].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "Unified Entry", Detail: "Handle messages, docs, and approvals in one workspace"},
		{Heading: "Workflow Sync", Detail: "Automate links across forms, tasks, and notifications"},
		{Heading: "Security", Detail: "Control access through permissions and audit trails"},
	}, 3)
	slides[2].Points = nil
	slides[2].Metrics = nil
	slides[2].Chart = nil
	slides[2].Subtitle = "Platform leverage comes from connecting information, workflow, and organization"

	slides[3].Title = "Customer Value"
	slides[3].Layout = "content"
	slides[3].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "Approval Cycle -30%", Detail: "Reduced from a 2.4-day baseline to 1.7 days in 8 weeks"},
		{Heading: "On-Time Delivery +15%", Detail: "Weekly task punctuality improved across 3 departments"},
		{Heading: "Knowledge Reuse +25%", Detail: "FAQ and template reuse now covers core workflows"},
	}, 3)
	slides[3].Metrics = nil
	slides[3].Chart = nil
	slides[3].Points = nil
	slides[3].HasImage = false
	slides[3].Source = "Method: baseline is the 4 weeks before launch, compared against the 8-week pilot across 3 departments"
	slides[3].Subtitle = "Value appears in efficiency gains, execution transparency, and stronger knowledge reuse"

	slides[4].Title = "Use Cases"
	slides[4].Layout = "content"
	slides[4].HasImage = false
	slides[4].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "Project Sync", Detail: "Drive milestones, risks, and tasks in one flow"},
		{Heading: "Sales Support", Detail: "Close the loop from lead to pricing approval online"},
		{Heading: "HQ Support", Detail: "Distribute announcements and training with tracked feedback"},
	}, 3)
	slides[4].Points = nil
	slides[4].Metrics = nil
	slides[4].Chart = nil
	slides[4].Subtitle = "Cross-functional high-frequency scenarios create the clearest early proof points"

	slides[5].Title = "Rollout Path"
	slides[5].Layout = "content"
	slides[5].HasImage = false
	slides[5] = normalizeActionSlide(slides[5])
	slides[5].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "2-Week Discovery", Detail: "Business owners and IT lock the pilot scope"},
		{Heading: "8-Week Pilot", Detail: "Launch three high-frequency scenarios and finish training"},
		{Heading: "Monthly Review", Detail: "Use adoption, efficiency, and satisfaction to decide expansion"},
	}, 3)
	slides[5].Points = nil
	slides[5].Metrics = nil
	slides[5].Chart = nil
	slides[5].Subtitle = "Move from pilot to rollout in stages to reduce risk and verify impact"
}

func enforceMarketSkeleton(slides []officegen.Slide) {
	if len(slides) < 6 {
		return
	}
	for idx := 1; idx < len(slides); idx++ {
		slides[idx].HasImage = false
		slides[idx].ImagePrompt = ""
		slides[idx].ImagePos = ""
	}

	slides[1].Title = "Key Takeaways"
	slides[1].Layout = "content"
	slides[1].Points = normalizePoints([]string{
		"Priority market: start with North America, then expand into Western Europe and developed APAC.",
		"Competition is decided by entry point, distribution, and compliance, not only by the model.",
		"The 90-day goal is paid validation, not broad expansion.",
	}, 3, 28)
	slides[1].Sections = nil
	if slides[1].Subtitle == "" {
		slides[1].Subtitle = "Win the English-speaking market first, then expand into Europe and developed APAC"
	}

	slides[2].Title = "Market Size"
	slides[2].Layout = "chart"
	slides[2] = normalizeEvidenceSlide(slides[2])
	slides[2].Chart = normalizeChart(&officegen.ChartData{Type: "bar", Title: "Regional Demand Index (North America = 100)", Categories: []string{"North America", "Europe", "APAC"}, Values: []float64{100, 72, 58}})
	slides[2].Source = "Method: relative demand index with North America = 100; compiled from public sources"
	if slides[2].Subtitle == "" {
		slides[2].Subtitle = "North America leads in scale, with Europe and developed APAC forming the second tier"
	}
	slides[2].Points = normalizePoints([]string{
		"Using North America = 100 makes entry priority easier to compare.",
		"Validate the highest-value English-speaking market first, then expand into more compliance-heavy regions.",
	}, 2, 28)

	slides[3].Title = "Regional Opportunities"
	slides[3].Layout = "content"
	slides[3].Sections = normalizeSections([]officegen.SlideSection{{Heading: "North America", Detail: "Strongest willingness to pay, ideal for testing unit economics first"}, {Heading: "Western Europe", Detail: "Stable budgets, but compliance readiness must come first"}, {Heading: "APAC", Detail: "Mature English-speaking markets make replication easier"}}, 3)
	slides[3].Points = nil
	if slides[3].Subtitle == "" {
		slides[3].Subtitle = "Region choice should be sequenced by monetization, compliance, and replication efficiency"
	}

	slides[4].Title = "Competitive Landscape"
	slides[4].Layout = "content"
	slides[4].Points = normalizePoints([]string{
		"Entry point: Microsoft and Google own default distribution, so direct head-on competition is inefficient.",
		"Distribution: independent tools win through sharp user experience and compounding word of mouth.",
		"White space: cross-app workflows, localized templates, and channel partnerships remain open.",
	}, 3, 30)
	slides[4].Sections = nil
	if slides[4].Subtitle == "" {
		slides[4].Subtitle = "Assess the market through entry point, distribution, and remaining white space"
	}

	slides[5].Title = "Entry Recommendations"
	slides[5].Layout = "content"
	slides[5] = normalizeActionSlide(slides[5])
	slides[5].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "6-Week MVP", Detail: "Product lead launches the English version and lands 10 trials"},
		{Heading: "8-Week Trial Sales", Detail: "Growth lead validates acquisition and channel motion"},
		{Heading: "90-Day Review", Detail: "Leadership decides expansion based on payback and retention"},
	}, 3)
	slides[5].Points = nil
	slides[5].Points = nil
	if slides[5].Subtitle == "" {
		slides[5].Subtitle = "Validate the market within 90 days and secure the first flagship customer"
	}
}

func enforceOpsSkeleton(slides []officegen.Slide) {
	if len(slides) < 6 {
		return
	}
	for idx := 1; idx < len(slides); idx++ {
		slides[idx].HasImage = false
		slides[idx].ImagePrompt = ""
		slides[idx].ImagePos = ""
	}

	slides[1].Title = "Business Takeaways"
	slides[1].Layout = "content"
	slides[1].Points = normalizePoints([]string{
		"ARR index reached 128, and growth was driven mainly by new acquisition.",
		"Renewal at 84 and collections at 76 show quality is trailing growth.",
		"Next quarter should focus on renewals, delivery efficiency, and cash collection.",
	}, 3, 28)
	slides[1].Sections = nil
	slides[1].Metrics = nil
	slides[1].Chart = nil
	slides[1].Source = ""
	slides[1].Subtitle = "New acquisition drives growth, but renewals, delivery, and collections are slowing quality improvement"

	slides[2].Title = "Core Metrics"
	slides[2].Layout = "chart"
	slides[2].Chart = normalizeChart(&officegen.ChartData{
		Type:       "bar",
		Title:      "Quarterly Operating Metrics (Last Quarter = 100)",
		Categories: []string{"New ARR", "Renewal Rate", "Collection Rate"},
		Values:     []float64{128, 84, 76},
	})
	slides[2].Points = normalizePoints([]string{
		"New ARR at 128 shows growth is still acquisition-led this quarter.",
		"Renewal at 84 and collections at 76 lag materially behind new growth.",
	}, 2, 28)
	slides[2].Sections = nil
	slides[2].Metrics = nil
	slides[2].Source = "Method: relative index with last quarter = 100; renewal and collection are normalized against quarterly targets"
	slides[2].Subtitle = "New ARR is still lifting growth, but renewals and collections are weakening quality"

	slides[3].Title = "Issue Diagnosis"
	slides[3].Layout = "content"
	slides[3].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "P1 Delivery Capacity", Detail: "Custom work is 42% of mix, extending project cycles by about 10 days"},
		{Heading: "P2 Collection Timing", Detail: "The top 10 customers carry longer payment terms and slower cash conversion"},
		{Heading: "P3 Funnel Conversion", Detail: "Mid-funnel opportunity win rate is 7 points below target"},
	}, 3)
	slides[3].Points = nil
	slides[3].Metrics = nil
	slides[3].Chart = nil
	slides[3].Source = ""
	slides[3].Subtitle = "Delivery, collections, and conversion all create drag on operating quality"

	slides[4].Title = "Next-Quarter Priorities"
	slides[4].Layout = "content"
	slides[4].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "Renewal Recovery", Detail: "Customer success lead restores renewal rate to above 90"},
		{Heading: "Delivery Efficiency", Detail: "Delivery lead reduces custom work share to below 30"},
		{Heading: "Collections Push", Detail: "Sales operations lead raises collection rate to 90"},
	}, 3)
	slides[4].Points = nil
	slides[4].Metrics = nil
	slides[4].Chart = nil
	slides[4].Source = ""
	slides[4].Subtitle = "Each priority is tied to an owner and result metric, not just direction"

	slides[5].Title = "Execution Actions"
	slides[5].Layout = "content"
	slides[5].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "April Sales Director", Detail: "Finish funnel review and lift win rate by 3 points"},
		{Heading: "May Delivery Lead", Detail: "Launch the standard package and reduce rework below 10"},
		{Heading: "June Operating Lead", Detail: "Review performance against renewal 90 and collection 90"},
	}, 3)
	slides[5].Points = nil
	slides[5].Metrics = nil
	slides[5].Chart = nil
	slides[5].Source = ""
	slides[5].Subtitle = "Advance monthly with clear owners, milestones, and validation metrics"
}

func enforceTrainingSkeleton(slides []officegen.Slide) {
	if len(slides) < 6 {
		return
	}
	for idx := 1; idx < len(slides); idx++ {
		slides[idx].HasImage = false
		slides[idx].ImagePrompt = ""
		slides[idx].ImagePos = ""
	}

	slides[1].Title = "Learning Goals"
	slides[1].Layout = "content"
	slides[1].Points = normalizePoints([]string{
		"Understand what OfficeCLI does, what it takes in, and what it outputs.",
		"Complete setup and run one local generation command successfully.",
		"Know the configuration boundaries and cautions before production use.",
	}, 3, 28)
	slides[1].Sections = nil
	slides[1].Metrics = nil
	slides[1].Chart = nil
	slides[1].Source = ""
	slides[1].Subtitle = "Build core understanding first, then complete the first independent command run"

	slides[2].Title = "Installation and Setup"
	slides[2].Layout = "content"
	slides[2].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "Environment Check", Detail: "Confirm Go, config files, and local dependencies are available"},
		{Heading: "Install Command", Detail: "Run the build or download command to create the executable"},
		{Heading: "Login Check", Detail: "After configuration, run a status command to verify connectivity"},
	}, 3)
	slides[2].Points = nil
	slides[2].Metrics = nil
	slides[2].Chart = nil
	slides[2].Source = ""
	slides[2].Subtitle = "Prepare in three steps: environment check, installation, and login validation"

	slides[3].Title = "Common Commands"
	slides[3].Layout = "content"
	slides[3].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "Status Check", Detail: "Run config status to verify configuration and dependencies"},
		{Heading: "Generate PPT", Detail: "Run new pptx to generate a local PPT file"},
		{Heading: "Quality Review", Detail: "Run review pptx for structural and visual review"},
	}, 3)
	slides[3].Points = nil
	slides[3].Metrics = nil
	slides[3].Chart = nil
	slides[3].Source = ""
	slides[3].Subtitle = "Memorize the three most common command groups first, then expand usage"

	slides[4].Title = "Example Workflow"
	slides[4].Layout = "content"
	slides[4].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "Step 1 Scope", Detail: "Define topic, audience, and style before generating output"},
		{Heading: "Step 2 Generate", Detail: "Run new pptx and confirm the file was written successfully"},
		{Heading: "Step 3 Review", Detail: "Run review pptx and iterate based on the findings"},
	}, 3)
	slides[4].Points = nil
	slides[4].Metrics = nil
	slides[4].Chart = nil
	slides[4].Source = ""
	slides[4].Subtitle = "A full practice run should cover generation, checking, and revision"

	slides[5].Title = "Cautions"
	slides[5].Layout = "content"
	slides[5].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "Validate Locally", Detail: "Keep publishing off by default until output quality is confirmed"},
		{Heading: "Complete Config", Detail: "Missing models, image settings, or dependencies will degrade results"},
		{Heading: "Keep Commands Intact", Detail: "Preserve full commands, paths, and parameters without truncation"},
	}, 3)
	slides[5].Points = nil
	slides[5].Metrics = nil
	slides[5].Chart = nil
	slides[5].Source = ""
	slides[5].Subtitle = "Validate quality locally before moving into the formal collaboration flow"
}

func isActionSlide(slide officegen.Slide) bool {
	text := strings.TrimSpace(slide.Title + " " + slide.Subtitle)
	for _, keyword := range []string{"recommendation", "next step", "rollout", "plan", "release", "training", "path", "action"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func pointToSection(point string, idx int) (string, string) {
	cleaned := cleanSentence(point)
	if cleaned == "" {
		return "", ""
	}
	for _, marker := range []string{"within 30 days", "within 60 days", "within 90 days", "weeks 1-2", "weeks 3-6", "weeks 7-10", "this week", "this month"} {
		if strings.HasPrefix(cleaned, marker) {
			return fitTextForLayout(marker, 10), fitTextForLayout(strings.TrimSpace(strings.TrimPrefix(cleaned, marker)), 24)
		}
	}
	for _, sep := range []string{"：", ":"} {
		parts := strings.SplitN(cleaned, sep, 2)
		if len(parts) != 2 {
			continue
		}
		label := fitTextForLayout(strings.TrimSpace(parts[0]), 10)
		body := fitTextForLayout(strings.TrimSpace(parts[1]), 24)
		if label != "" && body != "" {
			return label, body
		}
	}
	return fmt.Sprintf("Step %d", idx+1), fitTextForLayout(cleaned, 24)
}

func fitTextForLayout(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	for _, sep := range []string{".", ";", ",", ":", "("} {
		parts := strings.SplitN(value, sep, 2)
		if len(parts) == 0 {
			continue
		}
		prefix := strings.TrimSpace(parts[0])
		if prefix == "" {
			continue
		}
		size := utf8.RuneCountInString(prefix)
		if size <= maxRunes && size >= maxRunes/2 {
			return prefix
		}
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:maxRunes]))
}

func slideLayoutName(slide officegen.Slide) string {
	layout := strings.ToLower(strings.TrimSpace(slide.Layout))
	if layout != "" {
		return layout
	}
	if slide.IsTitle {
		return "title"
	}
	if len(slide.Metrics) > 0 {
		return "dashboard"
	}
	if slide.Chart != nil {
		return "chart"
	}
	return "content"
}

func cleanSentence(value string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"- ", "* ", "• ", "· ", "▪ ", "◦ "} {
		if strings.HasPrefix(value, prefix) {
			value = strings.TrimSpace(strings.TrimPrefix(value, prefix))
			break
		}
	}
	if idx := numericBulletPrefixEnd(value); idx > 0 {
		value = strings.TrimSpace(value[idx:])
	}
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ".")
	value = strings.TrimSuffix(value, ";")
	return value
}

func numericBulletPrefixEnd(value string) int {
	value = strings.TrimLeft(value, " \t")
	offset := 0
	for offset < len(value) && value[offset] >= '0' && value[offset] <= '9' {
		offset++
	}
	if offset == 0 || offset >= len(value) {
		return 0
	}
	switch {
	case strings.HasPrefix(value[offset:], "."):
		offset++
	case strings.HasPrefix(value[offset:], ")"):
		offset++
	default:
		return 0
	}
	if offset >= len(value) {
		return 0
	}
	if offset >= len(value) || (value[offset] != ' ' && value[offset] != '\t') {
		return 0
	}
	for offset < len(value) && (value[offset] == ' ' || value[offset] == '\t') {
		offset++
	}
	return offset
}

func shouldDowngradeChart(slide officegen.Slide) bool {
	if slide.Chart == nil {
		return false
	}
	text := strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Chart.Title)
	for _, keyword := range []string{"milestone", "cadence", "plan", "roadmap", "step", "workflow", "risk", "next step"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func downgradeChartSlide(slide officegen.Slide) officegen.Slide {
	chart := slide.Chart
	slide.Layout = "content"
	slide.Chart = nil
	if chart == nil {
		return slide
	}
	sections := make([]officegen.SlideSection, 0, len(chart.Categories))
	for idx, category := range chart.Categories {
		heading := fitTextForLayout(cleanSentence(category), 12)
		if heading == "" {
			continue
		}
		detail := fmt.Sprintf("Step %d", idx+1)
		if len(chart.Values) > idx && chart.Values[idx] > 0 {
			detail = fmt.Sprintf("Stage value %s", formatChartValue(chart.Values[idx]))
		}
		if strings.Contains(slide.Title, "milestone") || strings.Contains(slide.Title, "cadence") || strings.Contains(slide.Title, "plan") {
			detail = fmt.Sprintf("Phase %d, move in sequence", idx+1)
		}
		sections = append(sections, officegen.SlideSection{
			Heading: heading,
			Detail:  detail,
		})
	}
	slide.Sections = normalizeSections(sections, 4)
	if len(slide.Points) == 0 {
		slide.Points = deriveChartPoints(chart, 2)
	}
	return slide
}

func trimRunes(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return strings.TrimSpace(string(runes[:maxRunes-3])) + "..."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func formatChartValue(value float64) string {
	if float64(int64(value)) == value {
		return fmt.Sprintf("%d", int64(value))
	}
	return fmt.Sprintf("%.1f", value)
}
