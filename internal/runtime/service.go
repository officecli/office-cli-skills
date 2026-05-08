package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/officecli/officecli/engine"
	generateengine "github.com/officecli/officecli/engine/generate"
	"github.com/officecli/officecli/pkg/officegen"
)

type GenerateParams struct {
	DocumentType    engine.DocumentType
	Topic           string
	Prompt          string
	SourceFilePath  string
	Mode            string
	Language        string
	Style           string
	Audience        string
	EnableImages    bool
	ImageQuality    string
	ImageRatio      string
	ReferenceImages []engine.ImageReference
	LocalPreview    bool
}

type PPTXBuildOptions struct {
	ImageQuality      string
	CreditBalanceSink func(int)
}

type GeneratedArtifact struct {
	DocumentName        string
	DocumentType        string
	Bytes               []byte
	Warnings            []engine.GenerateIssue
	Errors              []engine.GenerateIssue
	PreviewHTML         []byte
	PreviewJSON         []byte
	HostedCreditBalance *int
	AccessMode          string
	Remaining           int
	FreeRemaining       int
	RewardRemaining     int
	PaidQuotaRemaining  int
}

type Service struct {
	llm      engine.LLMClient
	imageLLM engine.LLMClient
	progress engine.ProgressEmitter
}

func NewService(llm engine.LLMClient, progress any) *Service {
	service := &Service{llm: llm}
	if emitter, ok := progress.(engine.ProgressEmitter); ok {
		service.progress = emitter
	}
	return service
}

func (s *Service) WithImageLLM(llm engine.LLMClient) *Service {
	if s != nil {
		s.imageLLM = llm
	}
	return s
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
	case engine.DocumentTypeReport:
		return s.generateReport(ctx, envelope.Prompt, params.Topic, params.SourceFilePath, target, meta)
	case engine.DocumentTypePPTX:
		return s.generatePPTX(ctx, envelope.Prompt, params.Topic, target, meta, params.EnableImages, params.LocalPreview, params.ImageQuality)
	case engine.DocumentTypeIMG:
		return s.generateIMG(ctx, envelope.Prompt, params.Topic, target, params.ImageRatio, params.ReferenceImages, meta)
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

func (s *Service) generateReport(ctx context.Context, prompt, topic, sourceFilePath string, target generateengine.PromptTarget, meta *generateengine.PPTXMeta) (*GeneratedArtifact, error) {
	emitProgress(ctx, s.progress, progressStepAssemble, "running", "Reading workbook data for report generation")
	sheets, err := loadWorkbookSheetsFromFile(sourceFilePath)
	if err != nil {
		emitProgress(ctx, s.progress, progressStepAssemble, "failed", "Workbook read failed")
		return nil, fmt.Errorf("read report workbook: %w", err)
	}
	baseReport := officegen.BuildReportFromWorkbook(topic, sheets)
	baseReportJSON, err := json.Marshal(baseReport)
	if err != nil {
		emitProgress(ctx, s.progress, progressStepAssemble, "failed", "Report draft preparation failed")
		return nil, fmt.Errorf("marshal base report: %w", err)
	}
	emitProgress(ctx, s.progress, progressStepAssemble, "completed", "Workbook data prepared for report generation")

	emitProgress(ctx, s.progress, progressStepGenerateLLM, "running", "Requesting workbook-backed report narrative from the LLM")
	response, err := s.llm.CompleteJSON(ctx, []engine.LLMMessage{{
		Role: "user",
		Content: generateengine.BuildWorkbookReportPrompt(
			prompt,
			target,
			buildWorkbookSummary(sheets),
			string(baseReportJSON),
		),
	}})
	if err != nil {
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "failed", "Workbook-backed report narrative generation failed")
		return nil, fmt.Errorf("content generation failed: %w", err)
	}
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "completed", "Received workbook-backed report structure output")
	emitProgress(ctx, s.progress, progressStepAssemble, "running", "Assembling the report HTML")
	fileBytes, fileName, err := generateengine.BuildReportFromJSON(response, fallbackDescription(topic, prompt))
	if err != nil {
		emitProgress(ctx, s.progress, progressStepAssemble, "failed", "Report assembly failed")
		return nil, fmt.Errorf("document assembly failed: %w", err)
	}
	emitProgress(ctx, s.progress, progressStepAssemble, "completed", "Report assembly completed")
	return &GeneratedArtifact{
		DocumentName: fileName,
		DocumentType: string(engine.DocumentTypeReport),
		Bytes:        fileBytes,
		Warnings:     convertIssues(meta),
	}, nil
}

func (s *Service) generateIMG(ctx context.Context, prompt, topic string, target generateengine.PromptTarget, ratio string, references []engine.ImageReference, meta *generateengine.PPTXMeta) (*GeneratedArtifact, error) {
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "running", "Requesting image generation from the OfficeCLI server")
	image, err := s.llm.GenerateImage(ctx, engine.ImageGenerationRequest{
		Prompt:            buildImageGenerationPrompt(prompt, target),
		TargetAspectRatio: imageAspectRatio(ratio),
		ReferenceImages:   append([]engine.ImageReference(nil), references...),
	})
	if err != nil {
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "failed", "Image generation failed")
		return nil, fmt.Errorf("image generation failed: %w", err)
	}
	if image == nil || len(image.Data) == 0 {
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "failed", "Image generation returned empty data")
		return nil, fmt.Errorf("image generation returned empty data")
	}
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "completed", "Image generation completed")

	title := strings.TrimSpace(topic)
	if title == "" {
		title = generateengine.ExtractTitleFromDescription(prompt)
	}
	if title == "" {
		title = "image"
	}
	return &GeneratedArtifact{
		DocumentName:        fmt.Sprintf("%s%s", generateengine.SanitizeFileName(title), imageExtensionFromMIME(image.MIME)),
		DocumentType:        string(engine.DocumentTypeIMG),
		Bytes:               image.Data,
		Warnings:            convertIssues(meta),
		HostedCreditBalance: image.CreditBalance,
		AccessMode:          image.AccessMode,
		Remaining:           image.Remaining,
		FreeRemaining:       image.FreeRemaining,
		RewardRemaining:     image.RewardRemaining,
		PaidQuotaRemaining:  image.PaidQuotaRemaining,
	}, nil
}

func buildImageGenerationPrompt(prompt string, target generateengine.PromptTarget) string {
	parts := []string{strings.TrimSpace(prompt)}
	if strings.TrimSpace(target.Style) != "" {
		parts = append(parts, "Style: "+strings.TrimSpace(target.Style))
	}
	if strings.TrimSpace(target.Audience) != "" {
		parts = append(parts, "Audience/context: "+strings.TrimSpace(target.Audience))
	}
	if strings.TrimSpace(target.Language) != "" {
		parts = append(parts, "Language/text requirement: "+strings.TrimSpace(target.Language))
	}
	out := strings.TrimSpace(strings.Join(parts, "\n"))
	if out == "" {
		return "Generate an image."
	}
	return out
}

func imageAspectRatio(value string) float64 {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "landscape":
		return 16.0 / 9.0
	case "portrait":
		return 9.0 / 16.0
	default:
		return 1
	}
}

func imageExtensionFromMIME(mime string) string {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/png", "":
		return ".png"
	default:
		return ".png"
	}
}

func (s *Service) generatePPTX(ctx context.Context, prompt, topic string, target generateengine.PromptTarget, meta *generateengine.PPTXMeta, enableImages, localPreview bool, imageQuality string) (*GeneratedArtifact, error) {
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

	imageLLM := s.llm
	if normalizePPTXImageQuality(imageQuality) == "premium" {
		imageLLM = s.imageLLM
	}
	var hostedCreditBalance *int
	buildOptions := PPTXBuildOptions{
		ImageQuality: imageQuality,
		CreditBalanceSink: func(balance int) {
			value := balance
			hostedCreditBalance = &value
		},
	}
	fileBytes, fileName, warnings, previewHTML, previewJSON, err := BuildPPTXFromJSONWithOptions(ctx, imageLLM, s.progress, response, fallback, target.Style, enableImages, localPreview, buildOptions)
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
		hostedCreditBalance = nil
		fileBytes, fileName, warnings, previewHTML, previewJSON, err = BuildPPTXFromJSONWithOptions(ctx, imageLLM, s.progress, response, fallback, target.Style, enableImages, localPreview, buildOptions)
		if err != nil {
			return nil, err
		}
	}
	return &GeneratedArtifact{
		DocumentName:        fileName,
		DocumentType:        string(engine.DocumentTypePPTX),
		Bytes:               fileBytes,
		Warnings:            append(convertIssues(meta), warnings...),
		PreviewHTML:         previewHTML,
		PreviewJSON:         previewJSON,
		HostedCreditBalance: hostedCreditBalance,
	}, nil
}

func shouldRetryPPTXAssembly(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "parse llm response") ||
		strings.Contains(msg, "parse pptx deck spec") ||
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
	pptxArchetypeGeneral   pptxArchetype = "general"
	pptxArchetypeCompany   pptxArchetype = "company"
	pptxArchetypeMarket    pptxArchetype = "market"
	pptxArchetypeOps       pptxArchetype = "ops"
	pptxArchetypeProject   pptxArchetype = "project"
	pptxArchetypeTraining  pptxArchetype = "training"
	pptxArchetypeExplainer pptxArchetype = "explainer"
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
          "narrativeRole": { "type": "string" },
          "sectionIndex": { "type": "integer" },
          "sectionTitle": { "type": "string" },
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
          },
          "visuals": {
            "type": "array",
            "items": {
              "type": "object",
              "additionalProperties": false,
              "properties": {
                "label": { "type": "string" },
                "prompt": { "type": "string" },
                "caption": { "type": "string" }
              },
              "required": ["label", "prompt", "caption"]
            }
          }
        },
        "required": [
          "title",
          "content",
          "isTitle",
          "layout",
          "variant",
          "narrativeRole",
          "sectionIndex",
          "sectionTitle",
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
          "imagePos",
          "visuals"
        ]
      }
    }
  },
  "required": ["title", "stylePreset", "theme", "slides"]
}`

func BuildPPTXPrompt(description string, target generateengine.PromptTarget, enableImages bool) string {
	archetype := detectPPTXArchetype(description, "")
	presetHint := suggestStylePreset(target.Style, archetype, description)
	slideExample := `    {
      "role": "summary",
      "layout": "content",
      "variant": "sections-grid",
      "headline": "Section Title",
      "takeaway": "One-sentence takeaway",
      "blocks": [
        {"type": "sections", "sections": [
          {"heading": "Point 1", "detail": "Concise supporting detail"},
          {"heading": "Point 2", "detail": "Concise supporting detail"},
          {"heading": "Point 3", "detail": "Concise supporting detail"}
        ]}
      ],
      "source": "Optional data source"
    }`
	imageRules := "- Do not output visual objects or image fields."
	if enableImages {
		slideExample = `    {
      "role": "analysis",
      "layout": "gallery",
      "variant": "gallery",
      "headline": "Section Title",
      "takeaway": "One-sentence takeaway",
      "blocks": [
        {"type": "bullets", "items": ["Point 1", "Point 2"]},
        {"type": "sections", "sections": [
          {"heading": "Point 1", "detail": "Concise supporting detail"}
        ]}
      ],
      "visual": {"kind": "image", "position": "right", "prompt": "A concrete visual prompt that can be sent directly to an image model"},
      "source": "Optional data source"
    }`
		imageRules = `- Use images sparingly. Prefer at most one hero image slide plus at most one gallery slide in the whole deck.
- On hero-image slides, use visual.kind=image with visual.prompt and visual.position. visual.position must be one of right, left, background, center, top, bottom, or diagonal.
- visual.prompt must be a concrete visual description that can be sent directly to an image model. Avoid abstract wording.
- For gallery slides, use a visual image for the page theme and keep blocks concise; the renderer may rebalance image placement.
- Do not add images to chart, dashboard, toc, or closing layouts.
- Prefer images for title-cover hero visuals, product UI, usage scenarios, or training steps. By default do not add images to executive-summary, market analysis, competitive landscape, business review, quantified evidence, or action recommendation slides.
- On image slides, keep only 1-2 short points so text remains secondary to the visual.`
	}
	outlineRules := buildArchetypePromptRules(archetype)
	return fmt.Sprintf(`Generate a JSON structure for a PPT presentation based on the following request.

Request: %s
%s

Return JSON only. Do not add any extra commentary:
{
  "title": "Presentation Title",
  "subtitle": "Optional one-sentence deck framing",
  "stylePreset": "%s",
  "slides": [
    {
      "role": "cover",
      "layout": "title",
      "variant": "title-center",
      "headline": "Cover Title",
      "takeaway": "One-sentence takeaway",
      "blocks": []
    },
%s
  ]
}

Requirements:
	- Keep the deck to 6-10 slides, usually 7-9.
	- stylePreset must be one of executive-dark, editorial-light, explainer-voxel-light, tech-contrast, or training-manual. If the user did not specify one, choose the closest fit for the topic.
	- Use role/headline/takeaway/blocks/visual as the preferred semantic schema. The renderer will convert this into editable PPT text, shapes, charts, and image assets.
	- Do not output theme, bgColor, bgColor2, font, text color, or shape color fields. The renderer owns design tokens, contrast, and layout geometry.
	- The first slide must use role=cover and the title layout.
	- The deck should read like a real presentation, not a flat list of content pages. Prefer a storyline that fits the topic instead of reusing one generic scaffold.
	- For business decks, slide 2 should usually be a toc page, an early slide should read as an executive summary or key takeaways page, and the final slide should read as recommendation, decision, or one clear next action. Only use rollout-style endings when the request explicitly asks for rollout, implementation cadence, or milestones.
	- For game, hobby, culture, science, education, or general explainer decks, use the early slides to clarify what the topic is, why it stands out, or how it works, and avoid forcing a business-rollout ending.
	- Every slide must include role, headline, takeaway, blocks, layout, and variant. Use roles from this set only: cover, toc, chapter, summary, evidence, analysis, action, closing.
	- Use layouts from this set only: title, content, chart, dashboard, toc, chapter, gallery, comparison, timeline, closing.
	- title can only use title-center or title-split. toc should use toc. chapter should use chapter. gallery should use gallery. comparison should use comparison. timeline should use timeline. closing may use closing-decision-banner, closing-rollout-strip, closing-starter-guidance, closing-takeaway, closing-checklist, or closing-cards-light. For generic content prefer bullets, sections-grid, or image-right. Use chart-focus for chart and kpi-band for dashboard.
	- Each slide should express only one core idea. Keep headlines concise. takeaway must be a slide-level conclusion sentence.
- Prefer blocks over flat prose. Use block.type=sections for grouped ideas, bullets/actions/next_steps for concise action lists, metrics/kpis for KPI cards, chart/evidence for objective chart data, and narrative/paragraph only for one short paragraph.
- Prefer content layout for most slides, and use chart or dashboard only when needed.
- toc slides should list 3-6 agenda items. chapter slides should be concise separators with minimal text.
- comparison, timeline, and closing should rely on sections rather than long bullets.
- For comparisons, steps, regions, roles, or training paths, prefer sections with short heading and concise detail.
- For customer value, business review, market size, or competitive comparison, prefer evidence-based expression with chart or dashboard. If reliable numbers are unavailable, use 2-3 structured sections instead of long bullets.
- If the topic is market analysis, industry research, or business review, the deck must include at least one chart or dashboard slide with source or data framing.
- Action recommendations, rollout plans, release cadence, and training paths must use sections or metrics and show at least two of time, owner, or acceptance criteria.
- Keep content slide points to 3-4 concise bullets and avoid repetitive filler.
- Use at most 3 sections, at most 4 dashboard metrics, and at most 5 chart categories.
- Use charts only for objective data with units, scale, and ordering logic. Do not use charts for priorities, milestones, strategy, risks, or process flows.
- When a chart fits, chart may include type, categories, values, and title, plus 2-3 takeaway points.
- When a dashboard fits, metrics may include label, value, and note, plus 2-3 action or takeaway points.
- For business topics, the closing slide should end on one clear recommendation or decision ask with at most 2 supporting action chips. Only use rollout strips or 2-3 action steps when the request explicitly asks for rollout, implementation cadence, or milestones.
- For game, hobby, culture, science, education, or general explainer topics, the closing slide should end on a takeaway, who-it-suits summary, or how-to-start guidance, and must not sound like owners, milestones, rollout, or validation criteria.
- Wording should fit the audience and style. Prefer quantified, conclusion-first language for business topics, and plain, vivid, example-led language for explainer topics. Avoid vague slogans.
	%s
	%s`, description, generateengine.FormatDocumentPromptTarget(target), presetHint, slideExample, imageRules, outlineRules)
}

func BuildPPTXFromJSON(ctx context.Context, llm engine.LLMClient, progress engine.ProgressEmitter, content, fallback, requestedStyle string, enableImages, localPreview bool) ([]byte, string, []engine.GenerateIssue, []byte, []byte, error) {
	return BuildPPTXFromJSONWithOptions(ctx, llm, progress, content, fallback, requestedStyle, enableImages, localPreview, PPTXBuildOptions{})
}

func BuildPPTXFromJSONWithOptions(ctx context.Context, llm engine.LLMClient, progress engine.ProgressEmitter, content, fallback, requestedStyle string, enableImages, localPreview bool, options PPTXBuildOptions) ([]byte, string, []engine.GenerateIssue, []byte, []byte, error) {
	emitProgress(ctx, progress, progressStepAssemble, "running", "Parsing the PPTX structure and preparing assets")
	payloadPtr, err := parsePPTXPayload(content, fallback, requestedStyle, enableImages)
	if err != nil {
		emitProgress(ctx, progress, progressStepAssemble, "failed", "PPTX structure parsing failed")
		return nil, "", nil, nil, nil, fmt.Errorf("document assembly failed: %w", err)
	}
	payload := *payloadPtr
	if len(payload.Slides) == 0 {
		emitProgress(ctx, progress, progressStepAssemble, "failed", "PPTX structure is empty")
		return nil, "", nil, nil, nil, fmt.Errorf("document assembly failed: slides cannot be empty")
	}
	imageQuality := normalizePPTXImageQuality(options.ImageQuality)
	warnings := normalizePPTXPayloadWithOptions(&payload, fallback, requestedStyle, enableImages, imageQuality)
	if !enableImages {
		for idx := range payload.Slides {
			payload.Slides[idx].HasImage = false
			payload.Slides[idx].ImagePrompt = ""
			payload.Slides[idx].ImageData = nil
			payload.Slides[idx].ImageMIME = ""
			payload.Slides[idx].Visuals = nil
		}
	}
	imageTotal := 0
	for idx := range payload.Slides {
		if payload.Slides[idx].HasImage && strings.TrimSpace(payload.Slides[idx].ImagePrompt) != "" {
			imageTotal++
		}
		for visualIdx := range payload.Slides[idx].Visuals {
			if strings.TrimSpace(payload.Slides[idx].Visuals[visualIdx].Prompt) != "" {
				imageTotal++
			}
		}
	}
	imageIndex := 0
	firstImageFailure := ""
	var latestCreditBalance *int
	for idx := range payload.Slides {
		if payload.Slides[idx].HasImage && strings.TrimSpace(payload.Slides[idx].ImagePrompt) != "" && llm != nil {
			imageIndex++
			emitProgress(ctx, progress, progressStepAssemble, "running", fmt.Sprintf("Generating image asset (%d/%d)", imageIndex, imageTotal))
			aspectRatio := officegen.TargetAspectRatioForSlide(payload.Slides[idx])
			image, err := llm.GenerateImage(ctx, engine.ImageGenerationRequest{
				Prompt:            buildPPTXImagePrompt(payload.Slides[idx].ImagePrompt, imageQuality),
				TargetAspectRatio: aspectRatio,
			})
			if err == nil && image != nil {
				payload.Slides[idx].ImageData = image.Data
				payload.Slides[idx].ImageMIME = image.MIME
				if image.CreditBalance != nil {
					value := *image.CreditBalance
					latestCreditBalance = &value
					if options.CreditBalanceSink != nil {
						options.CreditBalanceSink(value)
					}
				}
				continue
			}
			payload.Slides[idx].ImageData = nil
			payload.Slides[idx].ImageMIME = ""
			if firstImageFailure == "" && err != nil {
				firstImageFailure = summarizeImageGenerationError(err)
			}
			if !hasWarningCode(warnings, "WARN_PPT_IMAGE_DEGRADED") {
				warnings = append(warnings, engine.GenerateIssue{
					Code:    "WARN_PPT_IMAGE_DEGRADED",
					Message: "Some images failed to generate, so the output was automatically downgraded to a text-only version. Check whether the generation service supports image endpoints, or run `officecli config set-generation` to configure the image model URL, credential, and model name. For a text-only deck, use `--no-images`.",
					Field:   "slides",
				})
			}
		}
		for visualIdx := range payload.Slides[idx].Visuals {
			if llm == nil || strings.TrimSpace(payload.Slides[idx].Visuals[visualIdx].Prompt) == "" {
				continue
			}
			imageIndex++
			emitProgress(ctx, progress, progressStepAssemble, "running", fmt.Sprintf("Generating image asset (%d/%d)", imageIndex, imageTotal))
			image, err := llm.GenerateImage(ctx, engine.ImageGenerationRequest{
				Prompt:            buildPPTXImagePrompt(payload.Slides[idx].Visuals[visualIdx].Prompt, imageQuality),
				TargetAspectRatio: 16.0 / 9.0,
			})
			if err == nil && image != nil {
				payload.Slides[idx].Visuals[visualIdx].ImageData = image.Data
				payload.Slides[idx].Visuals[visualIdx].ImageMIME = image.MIME
				if image.CreditBalance != nil {
					value := *image.CreditBalance
					latestCreditBalance = &value
					if options.CreditBalanceSink != nil {
						options.CreditBalanceSink(value)
					}
				}
				continue
			}
			payload.Slides[idx].Visuals[visualIdx].ImageData = nil
			payload.Slides[idx].Visuals[visualIdx].ImageMIME = ""
			if firstImageFailure == "" && err != nil {
				firstImageFailure = summarizeImageGenerationError(err)
			}
			if !hasWarningCode(warnings, "WARN_PPT_IMAGE_DEGRADED") {
				warnings = append(warnings, engine.GenerateIssue{
					Code:    "WARN_PPT_IMAGE_DEGRADED",
					Message: "Some images failed to generate, so the output was automatically downgraded to a text-only version. Check whether the generation service supports image endpoints, or run `officecli config set-generation` to configure the image model URL, credential, and model name. For a text-only deck, use `--no-images`.",
					Field:   "slides",
				})
			}
		}
	}
	warnings = finalizePPTImageResults(&payload, fallback, warnings, firstImageFailure, imageQuality)
	if imageQuality == "premium" && latestCreditBalance != nil {
		warnings = upsertWarning(warnings, engine.GenerateIssue{
			Code:    "INFO_PPT_HOSTED_IMAGE_CREDITS",
			Message: fmt.Sprintf("Premium PPT images used hosted image generation. %d credits remaining.", *latestCreditBalance),
			Field:   "image_quality",
		})
	}

	emitProgress(ctx, progress, progressStepAssemble, "running", "Packaging the PPTX file")
	fileBytes, err := officegen.NewPPTXGenerator().Generate(payload.Slides, officegen.PPTXOptions{
		Title:       payload.Title,
		Creator:     "OfficeCLI",
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
	return normalizePPTXPayloadWithOptions(payload, fallback, requestedStyle, enableImages, "standard")
}

func normalizePPTXPayloadWithOptions(payload *pptxPayload, fallback, requestedStyle string, enableImages bool, imageQuality string) []engine.GenerateIssue {
	if payload == nil {
		return nil
	}
	imageQuality = normalizePPTXImageQuality(imageQuality)

	const maxSlides = 10
	warnings := make([]engine.GenerateIssue, 0, 3)
	payload.Title = cleanVisibleText(firstNonEmpty(payload.Title, generateengine.ExtractTitleFromDescription(fallback), "Presentation"))
	archetype := detectPPTXArchetype(fallback, payload.Title)
	explicitStyle := strings.TrimSpace(requestedStyle)
	if explicitStyle != "" {
		payload.StylePreset = suggestStylePreset(explicitStyle, archetype, fallback+" "+payload.Title)
	} else {
		payload.StylePreset = suggestStylePreset("", archetype, fallback+" "+payload.Title)
	}
	if archetype == pptxArchetypeExplainer && explainerShouldUseVoxelLight(payload.StylePreset, requestedStyle) {
		payload.StylePreset = officegen.StylePresetExplainerVoxel
	}
	if archetype == pptxArchetypeExplainer || explicitStyle == "" {
		payload.Theme = officegen.MergeThemeWithPreset(nil, payload.StylePreset)
	} else {
		payload.Theme = officegen.MergeThemeWithPreset(payload.Theme, payload.StylePreset)
	}

	slides := make([]officegen.Slide, 0, len(payload.Slides))
	coverImageBudget := 0
	closingImageBudget := 0
	imageBudget := 1
	galleryBudget := 1
	visualBudget := 4
	if enableImages {
		coverImageBudget = 1
	}
	if archetype == pptxArchetypeExplainer {
		coverImageBudget = 1
		closingImageBudget = 1
		imageBudget = 0
		visualBudget = 2
	}
	if imageQuality == "premium" {
		coverImageBudget = 1
		closingImageBudget = 0
		imageBudget = 1
		galleryBudget = 0
		visualBudget = 1
	}
	slidesTrimmed := false
	imagesAdjusted := false
	for idx, slide := range payload.Slides {
		if len(slides) >= maxSlides {
			slidesTrimmed = true
			break
		}
		normalized, imageKept, visualsKept := normalizePPTXSlide(slide, idx, payload.Title, archetype, enableImages, &coverImageBudget, &closingImageBudget, &imageBudget, &galleryBudget, &visualBudget)
		if slide.HasImage && !imageKept {
			imagesAdjusted = true
		}
		if len(slide.Visuals) > 0 && visualsKept == 0 {
			imagesAdjusted = true
		}
		if isEmptyNormalizedSlide(normalized) {
			continue
		}
		slides = append(slides, expandSlideForDensity(normalized)...)
		if len(slides) > maxSlides {
			slidesTrimmed = true
			slides = slides[:maxSlides]
			break
		}
	}

	if len(slides) == 0 {
		slides = append(slides, officegen.Slide{
			Title:         payload.Title,
			Layout:        "title",
			IsTitle:       true,
			NarrativeRole: "cover",
			Subtitle:      cleanVisibleText(strings.TrimSpace(fallback)),
		})
	}

	if archetype == pptxArchetypeExplainer {
		slides = buildExplainerDeck(slides, payload.Title, enableImages)
	} else {
		slides[0].Layout = "title"
		slides[0].Variant = normalizeSlideVariant(slides[0])
		slides[0].IsTitle = true
		slides[0].NarrativeRole = "cover"
		slides[0].Visuals = nil
		if strings.TrimSpace(slides[0].Title) == "" {
			slides[0].Title = payload.Title
		}
		if strings.TrimSpace(slides[0].Subtitle) == "" {
			slides[0].Subtitle = cleanVisibleText(strings.TrimSpace(fallback))
		}
		if enableImages {
			slides[0].HasImage = true
			if imageQuality == "premium" {
				slides[0].ImagePos = "right"
				slides[0].Variant = "title-split"
			} else {
				slides[0].ImagePos = "background"
			}
			slides[0].ImagePrompt = fitTextForLayout(strings.TrimSpace(firstNonEmpty(slides[0].ImagePrompt, buildFallbackImagePrompt(slides[0], payload.Title))), 240)
		} else {
			slides[0].HasImage = false
			slides[0].ImagePrompt = ""
			slides[0].ImagePos = ""
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
		if archetype == pptxArchetypeProject {
			slides = buildProjectPlanDeck(slides, payload.Title)
			slidesTrimmed = false
		} else {
			slides = rebalanceNarrativeSlides(slides, payload.Title, archetype, maxSlides)
			slides = applyNarrativeScaffold(slides, payload.Title, archetype, maxSlides)
			slides = diversifyBusinessLayouts(slides, archetype)
			slides = reduceAdjacentVariantRepetition(slides)
		}
		if len(slides) > maxSlides {
			slidesTrimmed = true
			slides = slides[:maxSlides]
		}
	}
	slides = applyCoverImageDefaults(slides, payload.Title, enableImages, imageQuality)
	slides = compactDeckTextDensity(slides, 230)
	slides = reduceAdjacentVariantRepetition(slides)

	payload.Slides = slides

	if slidesTrimmed {
		warnings = append(warnings, engine.GenerateIssue{
			Code:    "WARN_PPT_SLIDES_TRIMMED",
			Field:   "slides",
			Message: "The generated deck exceeded quality limits and was automatically trimmed to 10 slides or fewer.",
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

func hasWarningCode(items []engine.GenerateIssue, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}

func finalizePPTImageResults(payload *pptxPayload, fallback string, warnings []engine.GenerateIssue, firstFailure, imageQuality string) []engine.GenerateIssue {
	if payload == nil {
		return warnings
	}
	archetype := detectPPTXArchetype(fallback, payload.Title)
	requested := 0
	succeeded := 0
	for idx := range payload.Slides {
		slide := &payload.Slides[idx]
		if slide.HasImage && strings.TrimSpace(slide.ImagePrompt) != "" {
			requested++
			if len(slide.ImageData) > 0 {
				succeeded++
			} else {
				slide.HasImage = false
				slide.ImagePrompt = ""
				slide.ImagePos = ""
				slide.ImageMIME = ""
				slide.Variant = normalizeSlideVariant(*slide)
			}
		}
		filteredVisuals := make([]officegen.SlideVisual, 0, len(slide.Visuals))
		for _, visual := range slide.Visuals {
			if strings.TrimSpace(visual.Prompt) == "" {
				continue
			}
			requested++
			if len(visual.ImageData) > 0 {
				succeeded++
				filteredVisuals = append(filteredVisuals, visual)
			}
		}
		if len(filteredVisuals) != len(slide.Visuals) {
			slide.Visuals = filteredVisuals
		}
	}
	if archetype == pptxArchetypeExplainer {
		applyExplainerImageOutcome(payload)
	}
	failed := requested - succeeded
	if failed <= 0 {
		return removeWarningCode(warnings, "WARN_PPT_IMAGE_DEGRADED")
	}
	message := "Some images failed to generate, so the output was automatically downgraded to a text-only version. Check whether the generation service supports image endpoints, or run `officecli config set-generation` to configure the image model URL, credential, and model name. For a text-only deck, use `--no-images`."
	if succeeded > 0 {
		message = "Some images failed to generate, but successfully generated visuals were kept in the deck. Check whether the generation service supports image endpoints, or run `officecli config set-generation` to configure the image model URL, credential, and model name."
	}
	if normalizePPTXImageQuality(imageQuality) == "premium" {
		message = "Premium PPT images failed to generate through the hosted image route, so the deck was generated without premium images. Check that hosted image generation is enabled for this key and that hosted credits are sufficient, or run `officecli config set-license` / purchase hosted credits. For a text-only deck, use `--no-images`."
		if succeeded > 0 {
			message = "Some premium PPT images failed through the hosted image route, but successfully generated visuals were kept in the deck. Check that hosted image generation is enabled for this key and that hosted credits are sufficient."
		}
	}
	if firstFailure != "" {
		message += " First image error: " + firstFailure
	}
	return upsertWarning(warnings, engine.GenerateIssue{
		Code:    "WARN_PPT_IMAGE_DEGRADED",
		Message: message,
		Field:   "slides",
	})
}

func summarizeImageGenerationError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.Join(strings.Fields(err.Error()), " ")
	const maxLen = 260
	if len(message) > maxLen {
		message = message[:maxLen-3] + "..."
	}
	return message
}

func applyExplainerImageOutcome(payload *pptxPayload) {
	if payload == nil {
		return
	}
	for idx := range payload.Slides {
		slide := &payload.Slides[idx]
		if strings.TrimSpace(slide.Title) != "Example / Gameplay Visual" {
			if !slide.HasImage {
				slide.Variant = normalizeSlideVariant(*slide)
			}
			continue
		}
		switch len(slide.Visuals) {
		case 0:
			slide.Layout = "content"
			slide.Variant = "bullets-callout"
			slide.HasImage = false
			slide.ImagePrompt = ""
			slide.ImagePos = ""
			slide.Points = normalizePoints(firstNonEmptySlice(slide.Points, []string{
				"Notice how the world is built from readable blocks and clear landmarks.",
				"Notice how exploring, gathering, and building connect into one simple loop.",
			}), 2, 0)
			slide.Sections = nil
		case 1:
			visual := slide.Visuals[0]
			slide.Layout = "content"
			slide.Variant = "image-right-focus"
			slide.HasImage = true
			slide.ImageData = visual.ImageData
			slide.ImageMIME = visual.ImageMIME
			slide.ImagePrompt = visual.Prompt
			slide.ImagePos = "right"
			slide.Points = normalizePoints(firstNonEmptySlice(slide.Points, []string{
				cleanVisibleText(firstNonEmpty(visual.Caption, visual.Label)),
				"Use this visual to connect the blocky world, building tools, and the survival loop.",
			}), 2, 0)
			slide.Sections = nil
			slide.Visuals = nil
		default:
			if len(slide.Visuals) > 2 {
				slide.Visuals = slide.Visuals[:2]
			}
			slide.Layout = "gallery"
			if strings.TrimSpace(slide.Variant) == "" || slide.Variant == "gallery" {
				slide.Variant = "gallery-duo"
			}
			slide.HasImage = false
			slide.ImagePrompt = ""
			slide.ImagePos = ""
		}
		slide.Content = ""
		slide.Metrics = nil
		slide.Chart = nil
		slide.Source = ""
	}
}

func removeWarningCode(items []engine.GenerateIssue, code string) []engine.GenerateIssue {
	if len(items) == 0 {
		return items
	}
	out := make([]engine.GenerateIssue, 0, len(items))
	for _, item := range items {
		if item.Code == code {
			continue
		}
		out = append(out, item)
	}
	return out
}

func upsertWarning(items []engine.GenerateIssue, warning engine.GenerateIssue) []engine.GenerateIssue {
	for idx := range items {
		if items[idx].Code == warning.Code {
			items[idx] = warning
			return items
		}
	}
	return append(items, warning)
}

func applyCoverImageDefaults(slides []officegen.Slide, deckTitle string, enableImages bool, imageQuality string) []officegen.Slide {
	if len(slides) == 0 || !enableImages {
		return slides
	}
	slides[0].HasImage = true
	if normalizePPTXImageQuality(imageQuality) == "premium" {
		slides[0].ImagePos = "right"
		if strings.TrimSpace(slides[0].Variant) == "" || strings.Contains(strings.TrimSpace(slides[0].Variant), "center") {
			slides[0].Variant = "title-split"
		}
	} else {
		slides[0].ImagePos = "background"
	}
	if strings.TrimSpace(slides[0].ImagePrompt) == "" {
		slides[0].ImagePrompt = fitTextForLayout(strings.TrimSpace(buildFallbackImagePrompt(slides[0], deckTitle)), 240)
	}
	return slides
}

func normalizePPTXImageQuality(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "premium":
		return "premium"
	default:
		return "standard"
	}
}

func buildPPTXImagePrompt(prompt, imageQuality string) string {
	prompt = strings.TrimSpace(prompt)
	if normalizePPTXImageQuality(imageQuality) != "premium" {
		return prompt
	}
	const noTextConstraint = "no text, no letters, no words, no UI labels, no charts with labels, no typography"
	lower := strings.ToLower(prompt)
	if strings.Contains(lower, noTextConstraint) {
		return prompt
	}
	if prompt == "" {
		return noTextConstraint
	}
	return prompt + ". Hard constraint: " + noTextConstraint + "."
}

func firstNonEmptySlice(primary, fallback []string) []string {
	for _, item := range primary {
		if strings.TrimSpace(item) != "" {
			return primary
		}
	}
	return fallback
}

func normalizePPTXSlide(slide officegen.Slide, idx int, deckTitle string, archetype pptxArchetype, enableImages bool, coverImageBudget, closingImageBudget, imageBudget, galleryBudget, visualBudget *int) (officegen.Slide, bool, int) {
	slide.Title = cleanVisibleText(firstNonEmpty(slide.Title, deckTitle))
	slide.Subtitle = fitTextForLayout(cleanVisibleText(strings.TrimSpace(slide.Subtitle)), 86)
	slide.SectionTitle = cleanVisibleText(strings.TrimSpace(slide.SectionTitle))
	slide.Source = fitTextForLayout(strings.TrimSpace(slide.Source), 48)
	slide.Content = strings.TrimSpace(slide.Content)
	slide.NarrativeRole = normalizeNarrativeRole(slide.NarrativeRole)
	slide.Visuals = normalizeSlideVisuals(slide.Visuals, 4)

	switch {
	case slideLayoutName(slide) != "":
		slide.Layout = slideLayoutName(slide)
	case slide.Chart != nil:
		slide.Layout = "chart"
	case len(slide.Metrics) > 0:
		slide.Layout = "dashboard"
	case strings.TrimSpace(slide.Layout) == "":
		slide.Layout = "content"
	default:
		slide.Layout = strings.ToLower(strings.TrimSpace(slide.Layout))
	}
	slide = upgradeSlideLayout(slide)

	slide.Points = normalizePoints(slide.Points, 4, 32)
	slide.Sections = normalizeSections(slide.Sections, 3)
	slide.Metrics = normalizeMetrics(slide.Metrics, 4)
	slide.Chart = normalizeChart(slide.Chart)
	slide = normalizeEvidenceSlide(slide)
	slide = normalizeActionSlide(slide)
	slide = normalizeClosingSlide(slide, archetype)
	slide.Variant = normalizeSlideVariant(slide)
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
	visualsKept := 0
	if enableImages && slide.HasImage && strings.TrimSpace(firstNonEmpty(slide.ImagePrompt, buildFallbackImagePrompt(slide, deckTitle))) != "" {
		allowPrimary := false
		switch {
		case idx == 0 && coverImageBudget != nil && *coverImageBudget > 0:
			allowPrimary = true
			*coverImageBudget--
		case slide.NarrativeRole == "closing" && allowClosingPrimaryImage(slide, archetype) && closingImageBudget != nil && *closingImageBudget > 0:
			allowPrimary = true
			*closingImageBudget--
		case slide.Layout == "content" && allowImageForSlide(slide) && archetype == pptxArchetypeExplainer && visualBudget != nil && *visualBudget > 0:
			allowPrimary = true
			*visualBudget--
		case slide.Layout == "content" && allowImageForSlide(slide) && imageBudget != nil && *imageBudget > 0:
			allowPrimary = true
			*imageBudget--
		}
		if allowPrimary {
			slide.HasImage = true
			slide.ImagePrompt = fitTextForLayout(strings.TrimSpace(firstNonEmpty(slide.ImagePrompt, buildFallbackImagePrompt(slide, deckTitle))), 240)
			if slide.NarrativeRole == "closing" {
				slide.ImagePos = "background"
			}
			slide.ImagePos = normalizeImagePosition(slide.ImagePos)
			imageKept = true
		} else {
			slide.HasImage = false
			slide.ImagePrompt = ""
			slide.ImagePos = ""
		}
	} else {
		slide.HasImage = false
		slide.ImagePrompt = ""
		slide.ImagePos = ""
	}

	if enableImages &&
		slide.Layout == "gallery" &&
		len(slide.Visuals) > 0 &&
		galleryBudget != nil &&
		visualBudget != nil &&
		*galleryBudget > 0 &&
		*visualBudget > 0 {
		allowed := len(slide.Visuals)
		if allowed > *visualBudget {
			allowed = *visualBudget
		}
		slide.Visuals = slide.Visuals[:allowed]
		*galleryBudget--
		*visualBudget -= allowed
		visualsKept = allowed
	} else if slide.Layout == "gallery" {
		slide.Visuals = nil
		if len(slide.Sections) > 0 || len(slide.Points) > 0 {
			slide.Layout = "content"
			slide.Variant = normalizeSlideVariant(slide)
		}
	}

	return slide, imageKept, visualsKept
}

func isEmptyNormalizedSlide(slide officegen.Slide) bool {
	if strings.TrimSpace(slide.Title) != "" {
		return false
	}
	if strings.TrimSpace(slide.Subtitle) != "" || strings.TrimSpace(slide.Content) != "" {
		return false
	}
	if len(slide.Points) > 0 || len(slide.Sections) > 0 || len(slide.Metrics) > 0 || len(slide.Visuals) > 0 || slide.Chart != nil {
		return false
	}
	return true
}

func normalizeNarrativeRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "cover", "toc", "chapter", "summary", "evidence", "analysis", "action", "closing":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeSlideVisuals(visuals []officegen.SlideVisual, limit int) []officegen.SlideVisual {
	out := make([]officegen.SlideVisual, 0, len(visuals))
	for _, visual := range visuals {
		label := cleanVisibleText(visual.Label)
		prompt := fitTextForLayout(strings.TrimSpace(visual.Prompt), 240)
		caption := cleanVisibleText(visual.Caption)
		if prompt == "" {
			continue
		}
		out = append(out, officegen.SlideVisual{
			Label:   label,
			Prompt:  prompt,
			Caption: caption,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func upgradeSlideLayout(slide officegen.Slide) officegen.Slide {
	switch slide.NarrativeRole {
	case "toc":
		slide.Layout = "toc"
	case "chapter":
		slide.Layout = "chapter"
	case "closing":
		slide.Layout = "closing"
	}
	if slide.Layout != "content" {
		return slide
	}
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.SectionTitle))
	switch {
	case len(slide.Visuals) > 0:
		slide.Layout = "gallery"
	case isActionSlide(slide):
		slide.Layout = "closing"
	case len(slide.Sections) >= 2 && containsAny(text, []string{"timeline", "roadmap", "path", "milestone", "cadence", "journey", "history", "phases"}):
		slide.Layout = "timeline"
	case len(slide.Sections) >= 2 && containsAny(text, []string{"compare", "comparison", "versus", "landscape", "difference", "options", "competition"}):
		slide.Layout = "comparison"
	}
	return slide
}

func containsAny(text string, items []string) bool {
	for _, item := range items {
		if strings.Contains(text, item) {
			return true
		}
	}
	return false
}

func normalizePoints(points []string, limit, maxRunes int) []string {
	out := make([]string, 0, len(points))
	seen := map[string]struct{}{}
	for _, point := range points {
		point = cleanVisibleText(point)
		if maxRunes > 0 {
			point = fitTextForLayout(point, maxRunes)
		}
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
		heading := fitTextForLayout(cleanVisibleText(section.Heading), 28)
		detail := fitTextForLayout(cleanVisibleText(section.Detail), 64)
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
		label := cleanVisibleText(metric.Label)
		value := cleanVisibleText(strings.TrimSpace(metric.Value))
		note := cleanVisibleText(metric.Note)
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

func explainerShouldUseVoxelLight(resolvedStyle, requestedStyle string) bool {
	requested := strings.ToLower(strings.TrimSpace(requestedStyle))
	if requested == "" {
		return true
	}
	return requested == officegen.StylePresetTechContrast && strings.EqualFold(strings.TrimSpace(resolvedStyle), officegen.StylePresetTechContrast)
}

func buildExplainerDeck(donors []officegen.Slide, deckTitle string, enableImages bool) []officegen.Slide {
	slides := defaultExplainerSlides(deckTitle, enableImages)
	if len(slides) == 0 {
		return donors
	}
	state := explainerVariantState{}
	var coverDonor officegen.Slide
	if len(donors) > 0 {
		coverDonor = donors[0]
	}
	coverChoice := chooseExplainerCoverChoice(slides[0], coverDonor, enableImages, state)
	slides[0] = mergeExplainerCover(slides[0], coverDonor, deckTitle, enableImages, coverChoice)
	state.record(coverChoice)

	bodyDonors := explainerBodyDonors(donors)
	for idx := 1; idx < len(slides); idx++ {
		var donor officegen.Slide
		switch slides[idx].Title {
		case "What It Is":
			donor = takeExplainerDonor(&bodyDonors, looksLikeWhatItIsSlide, looksLikeGenericExplainerOverviewSlide)
			choice := chooseExplainerWhatItIsChoice(slides[idx], donor, enableImages, state)
			slides[idx] = buildExplainerWhatItIsSlide(slides[idx], donor, deckTitle, enableImages, choice)
			state.record(choice)
		case "Core Ways to Play":
			donor = takeExplainerDonor(&bodyDonors, looksLikePlayLoopSlide)
			choice := chooseExplainerCoreWaysChoice(slides[idx], donor, state)
			slides[idx] = buildExplainerCoreWaysSlide(slides[idx], donor, choice)
			state.record(choice)
		case "Why It Stands Out":
			donor = takeExplainerDonor(&bodyDonors, looksLikeStandoutSlide)
			choice := chooseExplainerStandoutChoice(slides[idx], donor, state)
			slides[idx] = buildExplainerStandoutSlide(slides[idx], donor, choice)
			state.record(choice)
		case "Example / Gameplay Visual":
			donor = takeExplainerDonor(&bodyDonors, looksLikeExampleVisualSlide, looksLikePlayLoopSlide)
			choice := chooseExplainerExampleChoice(slides[idx], donor, deckTitle, state)
			slides[idx] = buildExplainerExampleSlide(slides[idx], donor, deckTitle, choice)
			state.record(choice)
		case "Who It Suits":
			donor = takeExplainerDonor(&bodyDonors, looksLikeAudienceFitSlide)
			choice := chooseExplainerAudienceChoice(slides[idx], donor, state)
			slides[idx] = buildExplainerAudienceSlide(slides[idx], donor, choice)
			state.record(choice)
		case "How to Start":
			donor = takeExplainerDonor(&bodyDonors, looksLikeStarterSlide)
			choice := chooseExplainerStartChoice(slides[idx], donor, state)
			slides[idx] = buildExplainerStartSlide(slides[idx], donor, choice)
			state.record(choice)
		default:
			choice := chooseExplainerWhatItIsChoice(slides[idx], officegen.Slide{}, enableImages, state)
			slides[idx] = buildExplainerWhatItIsSlide(slides[idx], officegen.Slide{}, deckTitle, enableImages, choice)
			state.record(choice)
		}
	}
	for idx := range slides {
		slides[idx].Title = cleanVisibleText(slides[idx].Title)
		slides[idx].Subtitle = cleanVisibleText(slides[idx].Subtitle)
		slides[idx].SectionTitle = cleanVisibleText(slides[idx].SectionTitle)
		slides[idx].Variant = normalizeSlideVariant(slides[idx])
		if idx == 0 {
			slides[idx].Layout = "title"
			slides[idx].IsTitle = true
			slides[idx].NarrativeRole = "cover"
			slides[idx].Points = nil
			slides[idx].Sections = nil
			slides[idx].Metrics = nil
			slides[idx].Chart = nil
			slides[idx].Content = ""
		} else {
			slides[idx].IsTitle = false
		}
		if !enableImages {
			slides[idx].HasImage = false
			slides[idx].ImagePrompt = ""
			slides[idx].ImagePos = ""
			slides[idx].Visuals = nil
		}
	}
	return slides
}

type explainerLayoutChoice struct {
	Layout  string
	Variant string
}

type explainerVariantState struct {
	usedFamilies []string
	usedVariants []string
}

func (s *explainerVariantState) record(choice explainerLayoutChoice) {
	if s == nil {
		return
	}
	if strings.TrimSpace(choice.Layout) != "" {
		s.usedFamilies = append(s.usedFamilies, strings.TrimSpace(choice.Layout))
	}
	if strings.TrimSpace(choice.Variant) != "" {
		s.usedVariants = append(s.usedVariants, strings.TrimSpace(choice.Variant))
	}
}

func chooseExplainerCoverChoice(slot, donor officegen.Slide, enableImages bool, state explainerVariantState) explainerLayoutChoice {
	if !enableImages || !hasReliableExplainerHeroImage(donor) {
		return explainerLayoutChoice{Layout: "title", Variant: "title-center-minimal"}
	}
	if len([]rune(cleanVisibleText(firstNonEmpty(donor.Title, slot.Title)))) > 28 {
		return explainerLayoutChoice{Layout: "title", Variant: "title-split-hero"}
	}
	return explainerLayoutChoice{Layout: "title", Variant: pickUnusedExplainerVariant(state, "title-center-hero", "title-split-hero")}
}

func chooseExplainerWhatItIsChoice(_ officegen.Slide, donor officegen.Slide, enableImages bool, state explainerVariantState) explainerLayoutChoice {
	if enableImages && hasReliableExplainerImage(donor) {
		return explainerLayoutChoice{Layout: "content", Variant: pickUnusedExplainerVariant(state, "image-right-editorial", "image-left-editorial")}
	}
	subtitleLen := utf8.RuneCountInString(strings.TrimSpace(donor.Subtitle))
	longestPoint := longestPointRunes(donor.Points)
	totalPoints := totalRunes(donor.Points...)
	if subtitleLen <= 56 && longestPoint <= 52 {
		return explainerLayoutChoice{Layout: "content", Variant: "bullets-callout"}
	}
	if subtitleLen <= 72 && totalPoints <= 190 {
		return explainerLayoutChoice{Layout: "content", Variant: "bullets-band"}
	}
	return explainerLayoutChoice{Layout: "content", Variant: pickUnusedExplainerVariant(state, "bullets-plain", "bullets-band")}
}

func chooseExplainerCoreWaysChoice(_ officegen.Slide, donor officegen.Slide, state explainerVariantState) explainerLayoutChoice {
	safeFallback := normalizeSections([]officegen.SlideSection{
		{Heading: "Explore", Detail: "Walk through new places and notice how the world opens up."},
		{Heading: "Gather", Detail: "Collect basic materials, food, and tools from the world."},
		{Heading: "Build", Detail: "Turn what you find into shelter, tools, and bigger ideas."},
	}, 3)
	if looksLikeSequentialExplainerSlide(donor) || looksLikePlayLoopSlide(donor) || isEmptyNormalizedSlide(donor) {
		if longestSectionDetailRunes(safeFallback) <= 62 {
			return explainerLayoutChoice{Layout: "timeline", Variant: pickUnusedExplainerVariant(state, "timeline-axis", "timeline-zigzag")}
		}
		return explainerLayoutChoice{Layout: "timeline", Variant: pickUnusedExplainerVariant(state, "timeline-steps", "timeline-axis")}
	}
	return explainerLayoutChoice{Layout: "content", Variant: pickUnusedExplainerVariant(state, "sections-grid-staggered", "sections-grid-3up")}
}

func chooseExplainerStandoutChoice(slot, donor officegen.Slide, state explainerVariantState) explainerLayoutChoice {
	safeComparison := normalizeSections(slot.Sections, 2)
	if looksLikeContrastExplainerSlide(donor) || len(safeComparison) <= 2 {
		if longestSectionDetailRunes(safeComparison) <= 58 {
			return explainerLayoutChoice{Layout: "comparison", Variant: pickUnusedExplainerVariant(state, "comparison-vs-band", "comparison-spotlight", "comparison-columns")}
		}
		return explainerLayoutChoice{Layout: "comparison", Variant: pickUnusedExplainerVariant(state, "comparison-columns", "comparison-spotlight")}
	}
	if longestSectionDetailRunes(slot.Sections) <= 58 {
		return explainerLayoutChoice{Layout: "content", Variant: pickUnusedExplainerVariant(state, "sections-grid-band", "sections-grid-3up")}
	}
	return explainerLayoutChoice{Layout: "content", Variant: pickUnusedExplainerVariant(state, "sections-grid-3up", "sections-grid-staggered")}
}

func chooseExplainerExampleChoice(_ officegen.Slide, donor officegen.Slide, deckTitle string, state explainerVariantState) explainerLayoutChoice {
	visuals := deriveExplainerVisuals(donor, deckTitle)
	switch len(visuals) {
	case 0:
		return explainerLayoutChoice{Layout: "content", Variant: "bullets-band"}
	case 1:
		return explainerLayoutChoice{Layout: "content", Variant: "image-right-focus"}
	default:
		return explainerLayoutChoice{Layout: "gallery", Variant: pickUnusedExplainerVariant(state, "gallery-focus", "gallery-duo", "gallery-filmstrip")}
	}
}

func chooseExplainerAudienceChoice(slot, donor officegen.Slide, state explainerVariantState) explainerLayoutChoice {
	safeFallback := normalizeSections(slot.Sections, 3)
	if len(safeFallback) == 2 && looksLikeContrastExplainerSlide(donor) && longestSectionDetailRunes(safeFallback) <= 80 {
		return explainerLayoutChoice{Layout: "comparison", Variant: pickUnusedExplainerVariant(state, "comparison-spotlight", "comparison-columns")}
	}
	if longestSectionDetailRunes(safeFallback) <= 58 {
		return explainerLayoutChoice{Layout: "content", Variant: pickUnusedExplainerVariant(state, "sections-grid-persona", "sections-grid-staggered")}
	}
	return explainerLayoutChoice{Layout: "content", Variant: pickUnusedExplainerVariant(state, "sections-grid-3up", "sections-grid-staggered")}
}

func chooseExplainerStartChoice(_ officegen.Slide, donor officegen.Slide, state explainerVariantState) explainerLayoutChoice {
	sections := deriveExplainerSections(donor, nil, 2, 3)
	longestDetail := longestSectionDetailRunes(sections)
	if (looksLikeSequentialExplainerSlide(donor) || looksLikeStarterSlide(donor) || isEmptyNormalizedSlide(donor)) && longestDetail <= 78 {
		return explainerLayoutChoice{Layout: "timeline", Variant: pickUnusedExplainerVariant(state, "timeline-steps", "timeline-zigzag")}
	}
	return explainerLayoutChoice{Layout: "closing", Variant: pickUnusedExplainerVariant(state, "closing-starter-guidance", "closing-takeaway", "closing-cards-light")}
}

func pickUnusedExplainerVariant(state explainerVariantState, candidates ...string) string {
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if !containsString(state.usedVariants, candidate) {
			return candidate
		}
	}
	for _, candidate := range candidates {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func hasReliableExplainerHeroImage(slide officegen.Slide) bool {
	return slide.HasImage && strings.TrimSpace(slide.ImagePrompt) != ""
}

func hasReliableExplainerImage(slide officegen.Slide) bool {
	if hasReliableExplainerHeroImage(slide) {
		return true
	}
	for _, visual := range slide.Visuals {
		if strings.TrimSpace(visual.Prompt) != "" {
			return true
		}
	}
	return false
}

func defaultExplainerSlides(deckTitle string, enableImages bool) []officegen.Slide {
	slides := []officegen.Slide{
		{
			Title:         firstNonEmpty(deckTitle, "Topic Explainer"),
			Layout:        "title",
			IsTitle:       true,
			NarrativeRole: "cover",
			Subtitle:      "A direct, beginner-friendly walkthrough of what it is, why it stands out, and how to get started",
		},
		{
			Title:         "What It Is",
			Layout:        "content",
			Variant:       "bullets",
			NarrativeRole: "summary",
			Subtitle:      "Start with the core idea in plain language",
			Points: []string{
				"Minecraft is a sandbox game where players shape their own experience.",
				"The world is made of simple blocks you can explore, gather, and build with.",
				"There is no single correct way to play because you set your own goals.",
			},
		},
		{
			Title:         "Core Ways to Play",
			Layout:        "timeline",
			Variant:       "timeline",
			NarrativeRole: "analysis",
			Subtitle:      "Focus on the few actions that create most of the experience",
			Sections: []officegen.SlideSection{
				{Heading: "Explore", Detail: "Walk through forests, caves, villages, and biomes to find materials and ideas."},
				{Heading: "Build", Detail: "Turn blocks into shelters, farms, tools, and bigger personal projects."},
				{Heading: "Survive or Create", Detail: "Choose pressure and challenge, or remove pressure and build freely."},
			},
		},
		{
			Title:         "Why It Stands Out",
			Layout:        "comparison",
			Variant:       "comparison",
			NarrativeRole: "analysis",
			Subtitle:      "Highlight the traits that make it memorable and replayable",
			Sections: []officegen.SlideSection{
				{Heading: "Freedom", Detail: "There is no single fixed path, so players set their own goals."},
				{Heading: "Creativity", Detail: "Simple blocks can become homes, machines, towns, or art."},
			},
		},
	}
	if enableImages {
		slides = append(slides, officegen.Slide{
			Title:         "Example / Gameplay Visual",
			Layout:        "gallery",
			Variant:       "gallery",
			NarrativeRole: "analysis",
			Subtitle:      "Use one concentrated visual example to make the experience concrete",
			Points: []string{
				"Notice how the world is built from readable blocks and clear landmarks.",
				"Notice how exploring, gathering, and building connect into one simple loop.",
			},
		})
	}
	slides = append(slides,
		officegen.Slide{
			Title:         "Who It Suits",
			Layout:        "content",
			Variant:       "sections-grid",
			NarrativeRole: "analysis",
			Subtitle:      "Different play styles make it approachable for different people",
			Sections: []officegen.SlideSection{
				{Heading: "Beginners", Detail: "Simple first steps make the game easy to try."},
				{Heading: "Creative Players", Detail: "It rewards building, designing, and shaping spaces."},
				{Heading: "Curious Explorers", Detail: "It fits people who like discovering new places by trying things."},
			},
		},
		officegen.Slide{
			Title:         "How to Start",
			Layout:        "timeline",
			Variant:       "timeline",
			NarrativeRole: "closing",
			Subtitle:      "Close with easy first steps",
			Sections: []officegen.SlideSection{
				{Heading: "Pick a Mode", Detail: "Start in Creative for freedom or Survival for a gentle challenge."},
				{Heading: "Try One Small Goal", Detail: "Build a first shelter, gather food, and learn the loop in one short session."},
				{Heading: "Keep It Small", Detail: "A simple first objective is enough to see whether the game feels fun."},
			},
		},
	)
	return slides
}

func explainerBodyDonors(donors []officegen.Slide) []officegen.Slide {
	if len(donors) <= 1 {
		return nil
	}
	out := make([]officegen.Slide, 0, len(donors)-1)
	for _, slide := range donors[1:] {
		switch slideLayoutName(slide) {
		case "title", "toc", "chapter":
			continue
		}
		if isEmptyNormalizedSlide(slide) {
			continue
		}
		if isExplainerScaffoldNoise(slide.Title) {
			continue
		}
		out = append(out, slide)
	}
	return out
}

func takeExplainerDonor(remaining *[]officegen.Slide, matchers ...func(officegen.Slide) bool) officegen.Slide {
	if remaining == nil || len(*remaining) == 0 {
		return officegen.Slide{}
	}
	for _, matcher := range matchers {
		if matcher == nil {
			continue
		}
		for idx, slide := range *remaining {
			if !matcher(slide) {
				continue
			}
			chosen := slide
			*remaining = append((*remaining)[:idx], (*remaining)[idx+1:]...)
			return chosen
		}
	}
	return officegen.Slide{}
}

func mergeExplainerCover(slot, donor officegen.Slide, deckTitle string, enableImages bool, choice explainerLayoutChoice) officegen.Slide {
	out := slot
	out.Layout = choice.Layout
	out.Variant = choice.Variant
	if title := cleanVisibleText(donor.Title); title != "" && !isPlaceholderSlideTitle(title) {
		out.Title = title
	}
	if subtitle := cleanVisibleText(firstNonEmpty(donor.Subtitle, donor.Content)); subtitle != "" && !looksLikeBusinessNarrative(subtitle) {
		out.Subtitle = subtitle
	}
	if !enableImages {
		return out
	}
	out.HasImage = true
	out.ImagePos = normalizeImagePosition(firstNonEmpty(donor.ImagePos, "background"))
	out.ImagePrompt = fitTextForLayout(strings.TrimSpace(firstNonEmpty(donor.ImagePrompt, buildFallbackImagePrompt(out, deckTitle))), 240)
	if choice.Variant == "title-center-hero" || choice.Variant == "title-split-hero" {
		out.ImagePos = "background"
	}
	return out
}

func buildExplainerWhatItIsSlide(slot, donor officegen.Slide, deckTitle string, enableImages bool, choice explainerLayoutChoice) officegen.Slide {
	out := slot
	out.Layout = choice.Layout
	out.Variant = choice.Variant
	out.NarrativeRole = "summary"
	out.Points = deriveExplainerPoints(donor, slot.Points, 2, 3)
	if longestPointRunes(out.Points) > 88 || totalRunes(out.Points...) > 190 {
		out.Points = normalizePoints(slot.Points, 3, 0)
	}
	out.Sections = nil
	out.Visuals = nil
	out.Content = ""
	out.Metrics = nil
	out.Chart = nil
	out.Source = ""
	if subtitle := cleanVisibleText(donor.Subtitle); subtitle != "" && !looksLikeBusinessNarrative(subtitle) {
		out.Subtitle = subtitle
	}
	if utf8.RuneCountInString(strings.TrimSpace(out.Subtitle)) > 78 {
		out.Subtitle = slot.Subtitle
	}
	if enableImages {
		if prompt, ok := extractReliableExplainerImagePrompt(donor); ok {
			out.HasImage = true
			out.ImagePrompt = fitTextForLayout(prompt, 240)
			if choice.Variant == "image-left-editorial" {
				out.ImagePos = "left"
			} else {
				out.ImagePos = "right"
			}
			if len(out.Points) > 2 {
				out.Points = out.Points[:2]
			}
			return out
		}
	}
	out.HasImage = false
	out.ImagePrompt = ""
	out.ImagePos = ""
	return out
}

func buildExplainerCoreWaysSlide(slot, donor officegen.Slide, choice explainerLayoutChoice) officegen.Slide {
	out := slot
	out.Layout = choice.Layout
	out.Variant = choice.Variant
	out.NarrativeRole = "analysis"
	if subtitle := cleanVisibleText(donor.Subtitle); subtitle != "" && !looksLikeBusinessNarrative(subtitle) {
		out.Subtitle = subtitle
	}
	if utf8.RuneCountInString(strings.TrimSpace(out.Subtitle)) > 78 {
		out.Subtitle = slot.Subtitle
	}
	sections := deriveExplainerSections(donor, slot.Sections, 3, 3)
	if longestSectionDetailRunes(sections) > 84 || totalSectionRunes(sections) > 240 {
		sections = normalizeSections(slot.Sections, 3)
	}
	out.HasImage = false
	out.ImagePrompt = ""
	out.ImagePos = ""
	out.Visuals = nil
	out.Points = nil
	out.Content = ""
	out.Metrics = nil
	out.Chart = nil
	out.Source = ""
	if choice.Layout == "timeline" {
		out.Sections = sections
		return out
	}
	out.Sections = sections
	return out
}

func buildExplainerStandoutSlide(slot, donor officegen.Slide, choice explainerLayoutChoice) officegen.Slide {
	out := slot
	out.Layout = choice.Layout
	out.Variant = choice.Variant
	out.NarrativeRole = "analysis"
	if subtitle := cleanVisibleText(donor.Subtitle); subtitle != "" && !looksLikeBusinessNarrative(subtitle) {
		out.Subtitle = subtitle
	}
	if utf8.RuneCountInString(strings.TrimSpace(out.Subtitle)) > 72 {
		out.Subtitle = slot.Subtitle
	}
	out.HasImage = false
	out.ImagePrompt = ""
	out.ImagePos = ""
	out.Visuals = nil
	out.Points = nil
	out.Content = ""
	out.Metrics = nil
	out.Chart = nil
	out.Source = ""
	comparisonFallback := slot.Sections
	gridFallback := append([]officegen.SlideSection(nil), slot.Sections...)
	gridFallback = append(gridFallback, officegen.SlideSection{Heading: "Replay Value", Detail: "The same world can keep feeling fresh because goals are self-directed."})
	comparisonSections := deriveExplainerSections(donor, comparisonFallback, 2, 2)
	if longestSectionDetailRunes(comparisonSections) > 80 {
		comparisonSections = normalizeSections(comparisonFallback, 2)
	}
	if choice.Layout == "comparison" {
		out.Sections = comparisonSections
		return out
	}
	out.Sections = deriveExplainerSections(donor, gridFallback, 3, 3)
	if longestSectionDetailRunes(out.Sections) > 72 || totalSectionRunes(out.Sections) > 220 {
		out.Sections = normalizeSections(gridFallback, 3)
	}
	return out
}

func buildExplainerExampleSlide(slot, donor officegen.Slide, deckTitle string, choice explainerLayoutChoice) officegen.Slide {
	out := slot
	out.Layout = choice.Layout
	out.Variant = choice.Variant
	out.NarrativeRole = "analysis"
	if subtitle := cleanVisibleText(donor.Subtitle); subtitle != "" && !looksLikeBusinessNarrative(subtitle) {
		out.Subtitle = subtitle
	}
	if utf8.RuneCountInString(strings.TrimSpace(out.Subtitle)) > 72 {
		out.Subtitle = slot.Subtitle
	}
	out.Points = deriveExplainerPoints(donor, slot.Points, 2, 2)
	if longestPointRunes(out.Points) > 72 {
		out.Points = normalizePoints(slot.Points, 2, 0)
	}
	out.Sections = nil
	out.HasImage = false
	out.ImagePrompt = ""
	out.ImagePos = ""
	out.Content = ""
	out.Metrics = nil
	out.Chart = nil
	out.Source = ""
	if choice.Layout == "gallery" || choice.Variant == "image-right-focus" {
		out.Visuals = deriveExplainerVisuals(donor, deckTitle)
	}
	if choice.Variant == "image-right-focus" && len(out.Visuals) > 0 {
		visual := out.Visuals[0]
		out.HasImage = true
		out.ImageData = visual.ImageData
		out.ImageMIME = visual.ImageMIME
		out.ImagePrompt = visual.Prompt
		out.ImagePos = "right"
		out.Visuals = nil
	}
	if strings.HasPrefix(choice.Variant, "bullets") {
		out.Layout = "content"
		out.HasImage = false
		out.ImagePrompt = ""
		out.ImagePos = ""
		out.Visuals = nil
	}
	return out
}

func buildExplainerAudienceSlide(slot, donor officegen.Slide, choice explainerLayoutChoice) officegen.Slide {
	out := slot
	out.Layout = choice.Layout
	out.Variant = choice.Variant
	out.NarrativeRole = "analysis"
	if subtitle := cleanVisibleText(donor.Subtitle); subtitle != "" && !looksLikeBusinessNarrative(subtitle) {
		out.Subtitle = subtitle
	}
	if utf8.RuneCountInString(strings.TrimSpace(out.Subtitle)) > 72 {
		out.Subtitle = slot.Subtitle
	}
	fallback := slot.Sections
	sections := deriveExplainerSections(donor, fallback, 2, 3)
	if longestSectionDetailRunes(sections) > 72 || totalSectionRunes(sections) > 220 {
		sections = normalizeSections(fallback, 3)
	}
	out.HasImage = false
	out.ImagePrompt = ""
	out.ImagePos = ""
	out.Visuals = nil
	out.Points = nil
	out.Content = ""
	out.Metrics = nil
	out.Chart = nil
	out.Source = ""
	if choice.Layout == "comparison" {
		out.Sections = sections
		return out
	}
	out.Sections = deriveExplainerSections(donor, fallback, 3, 3)
	return out
}

func buildExplainerStartSlide(slot, donor officegen.Slide, choice explainerLayoutChoice) officegen.Slide {
	out := slot
	out.Layout = choice.Layout
	out.Variant = choice.Variant
	out.NarrativeRole = "closing"
	if subtitle := cleanVisibleText(donor.Subtitle); subtitle != "" && !looksLikeBusinessNarrative(subtitle) {
		out.Subtitle = subtitle
	}
	if utf8.RuneCountInString(strings.TrimSpace(out.Subtitle)) > 72 {
		out.Subtitle = slot.Subtitle
	}
	fallback := slot.Sections
	sections := deriveExplainerSections(donor, fallback, 2, 3)
	if longestSectionDetailRunes(sections) > 72 || totalSectionRunes(sections) > 220 {
		sections = normalizeSections(fallback, 3)
	}
	out.HasImage = false
	out.ImagePrompt = ""
	out.ImagePos = ""
	out.Visuals = nil
	out.Points = nil
	out.Content = ""
	out.Metrics = nil
	out.Chart = nil
	out.Source = ""
	if choice.Layout == "timeline" {
		out.Sections = sections
		return out
	}
	out.Sections = sections
	return out
}

func deriveExplainerPoints(donor officegen.Slide, fallback []string, minCount, maxCount int) []string {
	points := make([]string, 0, maxCount)
	for _, point := range normalizePoints(donor.Points, maxCount, 0) {
		if point != "" {
			points = append(points, point)
		}
	}
	if len(points) == 0 {
		for _, section := range normalizeSections(donor.Sections, maxCount) {
			point := cleanVisibleText(firstNonEmpty(section.Detail, section.Heading))
			if point != "" {
				points = append(points, point)
			}
		}
	}
	if len(points) == 0 {
		for _, point := range splitContentToPoints(cleanVisibleText(donor.Content), maxCount) {
			if point != "" {
				points = append(points, point)
			}
		}
	}
	if len(points) == 0 {
		points = append(points, fallback...)
	}
	for _, point := range fallback {
		if len(points) >= minCount {
			break
		}
		point = cleanVisibleText(point)
		if point == "" || containsString(points, point) {
			continue
		}
		points = append(points, point)
	}
	if maxCount > 0 && len(points) > maxCount {
		points = points[:maxCount]
	}
	return normalizePoints(points, maxCount, 0)
}

func deriveExplainerSections(donor officegen.Slide, fallback []officegen.SlideSection, minCount, maxCount int) []officegen.SlideSection {
	sections := make([]officegen.SlideSection, 0, maxCount)
	if len(donor.Sections) > 0 {
		sections = append(sections, normalizeSections(donor.Sections, maxCount)...)
	} else if len(donor.Points) > 0 {
		sections = append(sections, pointsToSummarySections(donor.Points, maxCount)...)
	} else if content := cleanVisibleText(donor.Content); content != "" {
		sections = append(sections, pointsToSummarySections(splitContentToPoints(content, maxCount), maxCount)...)
	}
	for idx := range sections {
		if idx >= len(fallback) {
			break
		}
		if isGenericExplainerHeading(sections[idx].Heading) {
			sections[idx].Heading = fallback[idx].Heading
		}
		if strings.TrimSpace(sections[idx].Detail) == "" {
			sections[idx].Detail = fallback[idx].Detail
		}
	}
	if len(sections) == 0 {
		sections = append(sections, fallback...)
	}
	for _, section := range fallback {
		if len(sections) >= minCount {
			break
		}
		sections = append(sections, section)
	}
	if maxCount > 0 && len(sections) > maxCount {
		sections = sections[:maxCount]
	}
	return normalizeSections(sections, maxCount)
}

func deriveExplainerVisuals(donor officegen.Slide, deckTitle string) []officegen.SlideVisual {
	if visuals := normalizeSlideVisuals(donor.Visuals, 2); len(visuals) > 0 {
		return visuals
	}
	if prompt, ok := extractReliableExplainerImagePrompt(donor); ok {
		return []officegen.SlideVisual{{
			Label:   firstNonEmpty(cleanVisibleText(donor.Title), "Gameplay"),
			Prompt:  fitTextForLayout(prompt, 240),
			Caption: cleanVisibleText(firstNonEmpty(donor.Subtitle, "Focused visual example")),
		}}
	}
	return defaultExplainerVisuals(deckTitle)
}

func extractReliableExplainerImagePrompt(slide officegen.Slide) (string, bool) {
	if prompt := strings.TrimSpace(slide.ImagePrompt); prompt != "" {
		return prompt, true
	}
	for _, visual := range slide.Visuals {
		if prompt := strings.TrimSpace(visual.Prompt); prompt != "" {
			return prompt, true
		}
	}
	return "", false
}

func defaultExplainerVisuals(deckTitle string) []officegen.SlideVisual {
	titleText := strings.ToLower(strings.TrimSpace(deckTitle))
	if strings.Contains(titleText, "minecraft") {
		return []officegen.SlideVisual{
			{
				Label:   "World View",
				Prompt:  fitTextForLayout("Minecraft gameplay scene, blocky voxel sandbox world, cubic terrain, forests and plains biomes, simple survival shelter, no hand-painted fantasy art, no corporate diagram, no text overlay", 240),
				Caption: "A blocky voxel world makes the sandbox instantly recognizable",
			},
			{
				Label:   "Craft and Build",
				Prompt:  fitTextForLayout("Minecraft-like crafting and building scene, crafting table, wooden tools, block building in progress, cozy shelter, voxel lighting, no workshop illustration, no corporate infographic, no text overlay", 240),
				Caption: "Crafting and building show the loop in one glance",
			},
		}
	}
	return []officegen.SlideVisual{
		{
			Label:   "Example",
			Prompt:  fitTextForLayout(strings.TrimSpace(buildFallbackImagePrompt(officegen.Slide{Title: deckTitle, Subtitle: "Concrete example scene"}, deckTitle)), 240),
			Caption: "One strong example helps the audience picture the experience",
		},
		{
			Label:   "How It Feels",
			Prompt:  fitTextForLayout(strings.TrimSpace(buildFallbackImagePrompt(officegen.Slide{Title: deckTitle, Subtitle: "Immersive real-world style usage scene"}, deckTitle)), 240),
			Caption: "A second visual keeps the explanation grounded and memorable",
		},
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func isGenericExplainerHeading(value string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	switch {
	case text == "":
		return true
	case strings.HasPrefix(text, "takeaway"),
		strings.HasPrefix(text, "point"),
		strings.HasPrefix(text, "item"),
		strings.HasPrefix(text, "step"),
		strings.HasPrefix(text, "section"):
		return true
	default:
		return false
	}
}

func isExplainerScaffoldNoise(value string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	for _, keyword := range []string{"contents", "agenda", "chapter", "next steps", "rollout", "decision"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func looksLikeBusinessNarrative(value string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	for _, keyword := range []string{"rollout", "owner", "milestone", "next step", "decision", "executive summary", "business review"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func looksLikeAudienceFitSlide(slide officegen.Slide) bool {
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Content))
	for _, section := range slide.Sections {
		text += " " + strings.ToLower(strings.TrimSpace(section.Heading+" "+section.Detail))
	}
	for _, point := range slide.Points {
		text += " " + strings.ToLower(strings.TrimSpace(point))
	}
	for _, keyword := range []string{"who it suits", "good fit", "beginners", "players", "audience", "适合", "新手", "玩家", "受众"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func looksLikeStarterSlide(slide officegen.Slide) bool {
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Content))
	for _, section := range slide.Sections {
		text += " " + strings.ToLower(strings.TrimSpace(section.Heading+" "+section.Detail))
	}
	for _, point := range slide.Points {
		text += " " + strings.ToLower(strings.TrimSpace(point))
	}
	for _, keyword := range []string{"how to start", "first step", "start", "begin", "try", "mode", "starter", "how to", "开始", "入门", "第一步", "先试"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func looksLikePlayLoopSlide(slide officegen.Slide) bool {
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Content))
	for _, section := range slide.Sections {
		text += " " + strings.ToLower(strings.TrimSpace(section.Heading+" "+section.Detail))
	}
	for _, point := range slide.Points {
		text += " " + strings.ToLower(strings.TrimSpace(point))
	}
	for _, keyword := range []string{"play", "gameplay", "how it works", "loop", "build", "craft", "gather", "explore", "mode", "怎么玩", "玩法", "机制", "探索", "建造", "合成"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func looksLikeWhatItIsSlide(slide officegen.Slide) bool {
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Content))
	for _, keyword := range []string{"what it is", "what is", "overview", "introduction", "basic idea", "basics", "概述", "介绍", "是什么", "简介"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func looksLikeGenericExplainerOverviewSlide(slide officegen.Slide) bool {
	if isEmptyNormalizedSlide(slide) {
		return false
	}
	if looksLikePlayLoopSlide(slide) || looksLikeStandoutSlide(slide) || looksLikeAudienceFitSlide(slide) || looksLikeStarterSlide(slide) || looksLikeExampleVisualSlide(slide) {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle))
	for _, keyword := range []string{"overview", "summary", "intro", "guide", "understand", "quick look", "概览", "总览", "了解"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return len(slide.Points) > 0 || len(slide.Sections) > 0 || strings.TrimSpace(slide.Content) != ""
}

func looksLikeExampleVisualSlide(slide officegen.Slide) bool {
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Content))
	if len(slide.Visuals) > 0 || strings.TrimSpace(slide.ImagePrompt) != "" || slide.HasImage {
		return true
	}
	for _, keyword := range []string{"example", "scene", "visual", "gameplay", "screenshot", "demo", "示例", "画面", "场景", "演示"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func looksLikeSequentialExplainerSlide(slide officegen.Slide) bool {
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Content))
	for _, section := range slide.Sections {
		text += " " + strings.ToLower(strings.TrimSpace(section.Heading+" "+section.Detail))
	}
	for _, point := range slide.Points {
		text += " " + strings.ToLower(strings.TrimSpace(point))
	}
	for _, keyword := range []string{"step", "first", "then", "next", "start", "begin", "loop", "mode", "pick", "try", "timeline", "步骤", "开始", "然后", "接着", "下一步", "先"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func looksLikeContrastExplainerSlide(slide officegen.Slide) bool {
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Content))
	for _, section := range slide.Sections {
		text += " " + strings.ToLower(strings.TrimSpace(section.Heading+" "+section.Detail))
	}
	for _, point := range slide.Points {
		text += " " + strings.ToLower(strings.TrimSpace(point))
	}
	for _, keyword := range []string{"difference", "versus", "vs", "compare", "contrast", "modes", "types", "options", "对比", "区别", "模式", "类型"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func looksLikeStandoutSlide(slide officegen.Slide) bool {
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Content))
	for _, section := range slide.Sections {
		text += " " + strings.ToLower(strings.TrimSpace(section.Heading+" "+section.Detail))
	}
	for _, point := range slide.Points {
		text += " " + strings.ToLower(strings.TrimSpace(point))
	}
	for _, keyword := range []string{"stand out", "why it stands out", "special", "unique", "freedom", "creative", "replay", "memorable", "亮点", "特别", "自由", "创意", "重复可玩"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func buildFallbackImagePrompt(slide officegen.Slide, deckTitle string) string {
	subject := firstNonEmpty(cleanVisibleText(slide.Title), cleanVisibleText(deckTitle), "Presentation topic")
	fragments := make([]string, 0, 4)
	if subtitle := cleanVisibleText(slide.Subtitle); subtitle != "" {
		fragments = append(fragments, subtitle)
	}
	for _, section := range slide.Sections {
		if len(fragments) >= 3 {
			break
		}
		fragment := firstNonEmpty(cleanVisibleText(section.Heading), cleanVisibleText(section.Detail))
		if fragment != "" {
			fragments = append(fragments, fragment)
		}
	}
	for _, point := range slide.Points {
		if len(fragments) >= 3 {
			break
		}
		if cleaned := cleanVisibleText(point); cleaned != "" {
			fragments = append(fragments, cleaned)
		}
	}
	scene := strings.Join(fragments, "; ")
	styleHint := "editorial-light presentation visual, strong composition, no text overlay"
	lower := strings.ToLower(strings.TrimSpace(subject + " " + scene))
	switch {
	case strings.Contains(lower, "minecraft"):
		styleHint = "blocky voxel sandbox, Minecraft-like cubic terrain, crafting, biomes, survival shelter, block building, avoid hand-painted fantasy, workshop illustration, corporate diagram, no text overlay"
	case looksLikeGameSlide(slide):
		styleHint = "focused gameplay-style scene, immersive environment, polished editorial composition, no text overlay"
	case slide.NarrativeRole == "cover":
		styleHint = "editorial-light cover hero image, atmospheric composition, polished lighting, no text overlay"
	}
	return trimRunes(strings.TrimSpace(strings.Join([]string{subject, scene, styleHint}, ". ")), 320)
}

func looksLikeGameSlide(slide officegen.Slide) bool {
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Content))
	for _, keyword := range []string{"game", "gameplay", "world", "character", "sandbox", "minecraft", "游戏", "玩法", "世界", "角色", "沙盒"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func suggestStylePreset(style string, archetype pptxArchetype, topic string) string {
	text := strings.ToLower(strings.TrimSpace(style))
	topicText := strings.ToLower(strings.TrimSpace(topic))
	switch {
	case text == officegen.StylePresetExecutiveDark,
		strings.Contains(text, "board"),
		strings.Contains(text, "executive"),
		strings.Contains(text, "executive"):
		return officegen.StylePresetExecutiveDark
	case text == officegen.StylePresetEditorialLight,
		text == officegen.StylePresetExplainerVoxel,
		strings.Contains(text, "editorial"),
		strings.Contains(text, "magazine"),
		strings.Contains(text, "light background"),
		strings.Contains(text, "light"):
		if text == officegen.StylePresetExplainerVoxel || strings.Contains(text, "voxel") || strings.Contains(text, "sandbox") {
			return officegen.StylePresetExplainerVoxel
		}
		return officegen.StylePresetEditorialLight
	case text == officegen.StylePresetTrainingManual,
		strings.Contains(text, "training"),
		strings.Contains(text, "tutorial"),
		strings.Contains(text, "manual"):
		return officegen.StylePresetTrainingManual
	case text == officegen.StylePresetInvestorWarm,
		strings.Contains(text, "investor"),
		strings.Contains(text, "fundraising"),
		strings.Contains(text, "pitch"),
		strings.Contains(text, "融资"),
		strings.Contains(text, "路演"):
		return officegen.StylePresetInvestorWarm
	case text == officegen.StylePresetProjectForest,
		strings.Contains(text, "project"),
		strings.Contains(text, "implementation"),
		strings.Contains(text, "delivery plan"),
		strings.Contains(text, "项目"),
		strings.Contains(text, "实施"),
		strings.Contains(text, "里程碑"):
		return officegen.StylePresetProjectForest
	case text == officegen.StylePresetReviewCopper,
		strings.Contains(text, "review"),
		strings.Contains(text, "quarterly"),
		strings.Contains(text, "board"),
		strings.Contains(text, "复盘"),
		strings.Contains(text, "经营"),
		strings.Contains(text, "季度"):
		return officegen.StylePresetReviewCopper
	case text == officegen.StylePresetSlateSerif,
		strings.Contains(text, "collaboration"),
		strings.Contains(text, "procurement"),
		strings.Contains(text, "sales"),
		strings.Contains(text, "采购"),
		strings.Contains(text, "协作"),
		strings.Contains(text, "平台"):
		return officegen.StylePresetSlateSerif
	case text == officegen.StylePresetTechContrast,
		strings.Contains(text, "tech"),
		strings.Contains(text, "contrast"),
		strings.Contains(text, "technical"):
		return officegen.StylePresetTechContrast
	}
	switch {
	case strings.Contains(topicText, "融资"), strings.Contains(topicText, "路演"), strings.Contains(topicText, "investor"), strings.Contains(topicText, "fundraising"), strings.Contains(topicText, "pitch deck"):
		return officegen.StylePresetInvestorWarm
	case strings.Contains(topicText, "采购"), strings.Contains(topicText, "协作平台"), strings.Contains(topicText, "enterprise collaboration"), strings.Contains(topicText, "sales presentation"), strings.Contains(topicText, "value interpretation"):
		return officegen.StylePresetSlateSerif
	case strings.Contains(topicText, "项目方案"), strings.Contains(topicText, "实施方案"), strings.Contains(topicText, "里程碑"), strings.Contains(topicText, "resource plan"), strings.Contains(topicText, "project implementation"):
		return officegen.StylePresetProjectForest
	case strings.Contains(topicText, "launch plan"), strings.Contains(topicText, "release plan"), strings.Contains(topicText, "rollout plan"), strings.Contains(topicText, "上线计划"), strings.Contains(topicText, "发布计划"):
		return officegen.StylePresetProjectForest
	case strings.Contains(topicText, "培训"), strings.Contains(topicText, "入职"), strings.Contains(topicText, "新员工"), strings.Contains(topicText, "onboarding"), strings.Contains(topicText, "tutorial"):
		return officegen.StylePresetTrainingManual
	case strings.Contains(topicText, "复盘"), strings.Contains(topicText, "经营"), strings.Contains(topicText, "quarterly review"), strings.Contains(topicText, "business review"), strings.Contains(topicText, "board update"):
		return officegen.StylePresetReviewCopper
	}
	switch archetype {
	case pptxArchetypeCompany:
		return officegen.StylePresetSlateSerif
	case pptxArchetypeOps:
		return officegen.StylePresetReviewCopper
	case pptxArchetypeProject:
		return officegen.StylePresetProjectForest
	case pptxArchetypeMarket:
		return officegen.StylePresetInvestorWarm
	case pptxArchetypeTraining:
		return officegen.StylePresetTrainingManual
	case pptxArchetypeExplainer:
		return officegen.StylePresetExplainerVoxel
	default:
		return officegen.StylePresetTechContrast
	}
}

func rebalanceNarrativeSlides(slides []officegen.Slide, deckTitle string, archetype pptxArchetype, maxSlides int) []officegen.Slide {
	if len(slides) == 0 {
		return slides
	}
	slides = ensureMinimumNarrativeSlides(slides, deckTitle, archetype)
	if len(slides) > 1 && shouldInsertOverviewSlide(slides[1]) && len(slides) < maxSlides {
		slides = insertSlide(slides, 1, defaultSummarySlide(archetype, deckTitle))
	}
	if len(slides) > 1 {
		slides[1] = enforceOverviewSlide(slides[1], archetype)
	}
	slides = ensureEvidenceCoverage(slides, deckTitle, archetype, maxSlides)
	slides = ensureClosingActionSlide(slides, deckTitle, archetype, maxSlides)
	return slides
}

func insertSlide(slides []officegen.Slide, idx int, slide officegen.Slide) []officegen.Slide {
	if idx < 0 {
		idx = 0
	}
	if idx > len(slides) {
		idx = len(slides)
	}
	slides = append(slides, officegen.Slide{})
	copy(slides[idx+1:], slides[idx:])
	slides[idx] = slide
	return slides
}

func ensureMinimumNarrativeSlides(slides []officegen.Slide, deckTitle string, archetype pptxArchetype) []officegen.Slide {
	for len(slides) < 4 {
		switch len(slides) {
		case 1:
			slides = append(slides, defaultSummarySlide(archetype, deckTitle))
		case 2:
			slides = append(slides, defaultSupportingSlide(archetype, deckTitle))
		case 3:
			slides = append(slides, defaultActionSlide(archetype, deckTitle))
		default:
			return slides
		}
	}
	return slides
}

func defaultSummarySlide(archetype pptxArchetype, deckTitle string) officegen.Slide {
	switch archetype {
	case pptxArchetypeCompany:
		slide := officegen.Slide{
			Title:    "Key Takeaways",
			Layout:   "content",
			Subtitle: "Lead with the outcome, then explain the capabilities behind it",
			Sections: []officegen.SlideSection{
				{Heading: "Unified Work", Detail: "Bring messages, docs, approvals, and follow-up into one flow"},
				{Heading: "Visible ROI", Detail: "Anchor the story in cycle time, on-time work, and reuse metrics"},
				{Heading: "Safe Rollout", Detail: "Move from pilot to scale with governance and adoption checkpoints"},
			},
		}
		slide.Variant = normalizeSlideVariant(slide)
		return slide
	case pptxArchetypeGeneral:
		slide := officegen.Slide{
			Title:    "Executive Summary",
			Layout:   "content",
			Subtitle: "Lead with the decision, then support it slide by slide",
			Sections: []officegen.SlideSection{
				{Heading: "Core Insight", Detail: "State the main conclusion in direct business language"},
				{Heading: "Why It Matters", Detail: "Summarize the impact, risk, or upside behind the conclusion"},
				{Heading: "Decision", Detail: "Clarify what should happen next and who needs to move"},
			},
		}
		slide.Variant = normalizeSlideVariant(slide)
		return slide
	default:
		slide := defaultArchetypeSlide(archetype, 1, deckTitle)
		slide.Variant = normalizeSlideVariant(slide)
		return slide
	}
}

func defaultSupportingSlide(archetype pptxArchetype, deckTitle string) officegen.Slide {
	if archetype != pptxArchetypeGeneral {
		slide := defaultArchetypeSlide(archetype, 2, deckTitle)
		slide.Variant = normalizeSlideVariant(slide)
		return slide
	}
	slide := officegen.Slide{
		Title:    "What Matters Most",
		Layout:   "content",
		Subtitle: "Support the headline with the few facts that change the decision",
		Sections: []officegen.SlideSection{
			{Heading: "Signal", Detail: "Show the strongest evidence behind the conclusion"},
			{Heading: "Tradeoff", Detail: "Explain what becomes easier, faster, or safer"},
			{Heading: "Constraint", Detail: "Call out the key limit, dependency, or condition"},
		},
	}
	slide.Variant = normalizeSlideVariant(slide)
	return slide
}

func defaultActionSlide(archetype pptxArchetype, deckTitle string) officegen.Slide {
	if archetype != pptxArchetypeGeneral {
		slide := defaultArchetypeSlide(archetype, 5, deckTitle)
		slide.Variant = normalizeSlideVariant(slide)
		return slide
	}
	slide := officegen.Slide{
		Title:    "Recommendation",
		Layout:   "closing",
		Variant:  "closing-decision-banner",
		Subtitle: "Approve one focused validation cycle before scaling the approach.",
		Sections: []officegen.SlideSection{
			{Heading: "Decision", Detail: "Run a scoped pilot against the most important deck workflow."},
			{Heading: "Quality Gate", Detail: "Require editable output, layout diversity, and contrast checks."},
			{Heading: "Next Step", Detail: "Compare generated decks with a manual baseline before rollout."},
		},
	}
	slide.Variant = normalizeSlideVariant(slide)
	return slide
}

func enforceOverviewSlide(slide officegen.Slide, archetype pptxArchetype) officegen.Slide {
	if slideLayoutName(slide) == "chart" || slideLayoutName(slide) == "dashboard" {
		return defaultSummarySlide(archetype, slide.Title)
	}
	if len(slide.Sections) == 0 && len(slide.Points) >= 3 {
		if sections := pointsToSummarySections(slide.Points, 3); len(sections) > 0 {
			slide.Sections = sections
			slide.Points = nil
		}
	}
	if !looksLikeOverviewSlide(slide) || isPlaceholderSlideTitle(slide.Title) {
		slide.Title = summaryTitleForArchetype(archetype)
	}
	if strings.TrimSpace(slide.Subtitle) == "" || isPlaceholderSlideTitle(slide.Subtitle) {
		slide.Subtitle = summarySubtitleForArchetype(archetype)
	}
	slide.Layout = "content"
	slide.HasImage = false
	slide.ImagePrompt = ""
	slide.ImagePos = ""
	slide.Variant = normalizeSlideVariant(slide)
	return slide
}

func ensureEvidenceCoverage(slides []officegen.Slide, deckTitle string, archetype pptxArchetype, maxSlides int) []officegen.Slide {
	if archetype != pptxArchetypeMarket && archetype != pptxArchetypeOps {
		return slides
	}
	for idx := 1; idx < len(slides); idx++ {
		switch slideLayoutName(slides[idx]) {
		case "chart", "dashboard":
			return slides
		}
	}
	evidenceSlide := defaultSupportingSlide(archetype, deckTitle)
	if len(slides) > 2 && isReplaceableNarrativeSlide(slides[2]) {
		slides[2] = evidenceSlide
		return slides
	}
	if len(slides) < maxSlides {
		insertIdx := 2
		if insertIdx > len(slides) {
			insertIdx = len(slides)
		}
		slides = append(slides, officegen.Slide{})
		copy(slides[insertIdx+1:], slides[insertIdx:])
		slides[insertIdx] = evidenceSlide
	}
	return slides
}

func ensureClosingActionSlide(slides []officegen.Slide, deckTitle string, archetype pptxArchetype, maxSlides int) []officegen.Slide {
	if len(slides) == 0 {
		return slides
	}
	lastIdx := len(slides) - 1
	last := slides[lastIdx]
	if isActionSlide(last) || looksLikeClosingSlide(last) {
		if isPlaceholderSlideTitle(last.Title) {
			last.Title = actionTitleForArchetype(archetype)
		}
		if strings.TrimSpace(last.Subtitle) == "" || isPlaceholderSlideTitle(last.Subtitle) {
			last.Subtitle = actionSubtitleForArchetype(archetype)
		}
		last = normalizeActionSlide(last)
		last.Layout = "content"
		last.HasImage = false
		last.ImagePrompt = ""
		last.ImagePos = ""
		last.Variant = normalizeSlideVariant(last)
		slides[lastIdx] = last
		return slides
	}
	if len(slides) < maxSlides {
		return append(slides, defaultActionSlide(archetype, deckTitle))
	}
	if isReplaceableNarrativeSlide(last) || slideLayoutName(last) == "content" {
		slides[lastIdx] = defaultActionSlide(archetype, deckTitle)
	}
	return slides
}

func applyNarrativeScaffold(slides []officegen.Slide, deckTitle string, archetype pptxArchetype, maxSlides int) []officegen.Slide {
	if len(slides) < 3 {
		return slides
	}
	sectionTitles := sectionTitlesForArchetype(archetype)
	if slideLayoutName(slides[1]) != "toc" && len(slides) < maxSlides {
		slides = insertSlide(slides, 1, buildTOCSlide(slides[1:], sectionTitles[0]))
	}

	bodyStart := 1
	if len(slides) > 1 && slideLayoutName(slides[1]) == "toc" {
		bodyStart = 2
	}
	if bodyStart < len(slides)-1 && slideLayoutName(slides[bodyStart]) != "chapter" && len(slides) < maxSlides {
		slides = insertSlide(slides, bodyStart, buildChapterSlide(1, sectionTitles[0]))
	}
	if len(slides) > bodyStart+2 && len(slides) < maxSlides {
		lastIdx := len(slides) - 1
		if slideLayoutName(slides[lastIdx]) != "closing" && slideLayoutName(slides[lastIdx-1]) != "chapter" {
			slides = insertSlide(slides, lastIdx, buildChapterSlide(2, sectionTitles[1]))
		}
	}

	lastIdx := len(slides) - 1
	slides[lastIdx].Layout = "closing"
	slides[lastIdx].NarrativeRole = "closing"
	slides[lastIdx].SectionIndex = len(sectionTitles)
	slides[lastIdx].SectionTitle = sectionTitles[len(sectionTitles)-1]
	slides[lastIdx].Variant = normalizeSlideVariant(slides[lastIdx])
	if len(slides[lastIdx].Sections) == 0 && len(slides[lastIdx].Points) > 0 {
		slides[lastIdx] = normalizeActionSlide(slides[lastIdx])
	}

	currentSection := 1
	currentTitle := sectionTitles[0]
	for idx := 1; idx < len(slides); idx++ {
		switch slideLayoutName(slides[idx]) {
		case "toc":
			slides[idx].NarrativeRole = "toc"
			slides[idx].SectionIndex = 0
			slides[idx].SectionTitle = "Agenda"
		case "chapter":
			currentSection = maxInt(slides[idx].SectionIndex, currentSection)
			currentTitle = firstNonEmpty(slides[idx].SectionTitle, sectionTitleAt(sectionTitles, currentSection-1))
			slides[idx].NarrativeRole = "chapter"
			slides[idx].SectionIndex = currentSection
			slides[idx].SectionTitle = currentTitle
		case "closing":
			slides[idx].NarrativeRole = "closing"
			slides[idx].SectionIndex = len(sectionTitles)
			slides[idx].SectionTitle = sectionTitles[len(sectionTitles)-1]
		default:
			if slides[idx].NarrativeRole == "" {
				if idx <= bodyStart+1 {
					slides[idx].NarrativeRole = "summary"
				} else {
					slides[idx].NarrativeRole = "analysis"
				}
			}
			slides[idx].SectionIndex = currentSection
			slides[idx].SectionTitle = currentTitle
		}
		if strings.TrimSpace(slides[idx].Variant) == "" {
			slides[idx].Variant = normalizeSlideVariant(slides[idx])
		}
	}

	if len(slides) > 1 && slideLayoutName(slides[1]) == "toc" {
		slides[1] = buildTOCSlide(slides[2:], sectionTitles[0])
	}
	return slides
}

func buildTOCSlide(body []officegen.Slide, title string) officegen.Slide {
	sections := make([]officegen.SlideSection, 0, len(body))
	for _, slide := range body {
		switch slideLayoutName(slide) {
		case "title", "toc":
			continue
		}
		label := strings.TrimSpace(slide.Title)
		if label == "" {
			continue
		}
		sections = append(sections, officegen.SlideSection{
			Heading: fmt.Sprintf("%02d", len(sections)+1),
			Detail:  label,
		})
		if len(sections) >= 6 {
			break
		}
	}
	return officegen.Slide{
		Title:         "Contents",
		Layout:        "toc",
		Variant:       "toc",
		NarrativeRole: "toc",
		SectionTitle:  title,
		Subtitle:      "Review the structure before diving into the content.",
		Sections:      sections,
	}
}

func buildChapterSlide(idx int, title string) officegen.Slide {
	return officegen.Slide{
		Title:         title,
		Layout:        "chapter",
		Variant:       "chapter",
		NarrativeRole: "chapter",
		SectionIndex:  idx,
		SectionTitle:  title,
		Subtitle:      "Use this section to shift the story before the next cluster of slides.",
	}
}

func buildProjectPlanDeck(slides []officegen.Slide, deckTitle string) []officegen.Slide {
	if len(slides) == 0 {
		return slides
	}
	title := firstNonEmpty(cleanVisibleText(deckTitle), cleanVisibleText(slides[0].Title), "Project Launch Plan")
	cover := slides[0]
	cover.Title = title
	cover.Layout = "title"
	cover.IsTitle = true
	cover.NarrativeRole = "cover"
	cover.Role = "cover"
	cover.Variant = normalizeSlideVariant(cover)
	if strings.TrimSpace(cover.Subtitle) == "" {
		cover.Subtitle = "Align scope, readiness, owners, gates, and risk controls in one operating plan"
	}
	cover.Points = nil
	cover.Sections = nil
	cover.Metrics = nil
	cover.Chart = nil
	cover.Content = ""
	cover.Visuals = nil

	out := []officegen.Slide{
		cover,
		{
			Title:         "Contents",
			Layout:        "toc",
			Variant:       "toc",
			NarrativeRole: "toc",
			SectionTitle:  "Agenda",
			Subtitle:      "Review the operating path before the workstream detail.",
			Sections: normalizeSections([]officegen.SlideSection{
				{Heading: "01", Detail: "Launch outcomes and success criteria"},
				{Heading: "02", Detail: "Readiness scorecard"},
				{Heading: "03", Detail: "Ownership and milestone gates"},
				{Heading: "04", Detail: "Risk controls and decision request"},
			}, 4),
		},
		{
			Title:         "Readiness",
			Layout:        "chapter",
			Variant:       "chapter",
			NarrativeRole: "chapter",
			SectionIndex:  1,
			SectionTitle:  "Readiness",
			Subtitle:      "Move from intent to measurable launch conditions.",
		},
		{
			Title:         "Launch Outcomes",
			Layout:        "content",
			Variant:       "sections-grid-3up",
			NarrativeRole: "summary",
			SectionIndex:  1,
			SectionTitle:  "Readiness",
			Subtitle:      "Success is date confidence, complete teams, and controlled quality.",
			Sections: normalizeSections([]officegen.SlideSection{
				{Heading: "Scope Freeze", Detail: "Lock launch scope two weeks before go-live"},
				{Heading: "Team Ready", Detail: "GTM and support finish assets and training"},
				{Heading: "Quality Bar", Detail: "No critical blocker remains open at launch"},
			}, 3),
		},
		{
			Title:         "Readiness Scorecard",
			Layout:        "dashboard",
			Variant:       "kpi-band",
			NarrativeRole: "evidence",
			SectionIndex:  1,
			SectionTitle:  "Readiness",
			Subtitle:      "The launch gate should track readiness by workstream, not opinion.",
			Metrics: normalizeMetrics([]officegen.MetricCard{
				{Label: "Scope", Value: "100%", Note: "Freeze signed"},
				{Label: "GTM", Value: "90%+", Note: "Assets ready"},
				{Label: "Support", Value: "95%+", Note: "Training done"},
				{Label: "Quality", Value: "0", Note: "Critical blockers"},
			}, 4),
		},
		{
			Title:         "Execution System",
			Layout:        "chapter",
			Variant:       "chapter",
			NarrativeRole: "chapter",
			SectionIndex:  2,
			SectionTitle:  "Execution and Decisions",
			Subtitle:      "Translate readiness into owners, gates, and controls.",
		},
		{
			Title:         "Workstream Ownership",
			Layout:        "comparison",
			Variant:       "comparison-columns",
			NarrativeRole: "analysis",
			SectionIndex:  2,
			SectionTitle:  "Execution and Decisions",
			Subtitle:      "Single-accountable owners reduce handoff ambiguity.",
			Sections: normalizeSections([]officegen.SlideSection{
				{Heading: "Product and Eng", Detail: "Own scope, defects, notes, launch quality"},
				{Heading: "GTM", Detail: "Own messaging, assets, enablement handoff"},
				{Heading: "Support and Ops", Detail: "Own training, comms, runbooks, and issue routing"},
			}, 3),
		},
		{
			Title:         "Milestones and Gates",
			Layout:        "timeline",
			Variant:       "timeline-axis",
			NarrativeRole: "action",
			SectionIndex:  2,
			SectionTitle:  "Execution and Decisions",
			Subtitle:      "Each phase ends with a decision gate.",
			Sections: normalizeSections([]officegen.SlideSection{
				{Heading: "T-8 to T-6", Detail: "Lock scope, owners, and success criteria"},
				{Heading: "T-5 to T-3", Detail: "Finish QA, messaging, enablement"},
				{Heading: "T-2 to Launch", Detail: "Approve readiness and rollback path"},
			}, 3),
		},
		{
			Title:         "Risk Controls",
			Layout:        "content",
			Variant:       "sections-grid-band",
			NarrativeRole: "analysis",
			SectionIndex:  2,
			SectionTitle:  "Execution and Decisions",
			Subtitle:      "Manage launch risk through triggers and rehearsed responses.",
			Sections: normalizeSections([]officegen.SlideSection{
				{Heading: "Trigger", Detail: "Escalate red scope, quality, or readiness"},
				{Heading: "Response", Detail: "Assign one DRI and a 24-hour mitigation plan"},
				{Heading: "Fallback", Detail: "Use rollback or phased release if not green"},
			}, 3),
		},
		{
			Title:         "Decision Request",
			Layout:        "closing",
			Variant:       "closing-decision-banner",
			NarrativeRole: "closing",
			SectionIndex:  2,
			SectionTitle:  "Execution and Decisions",
			Subtitle:      "Approve the operating model so every function works from one launch plan.",
			Sections: normalizeSections([]officegen.SlideSection{
				{Heading: "Decision", Detail: "Approve goals, cadence, owners, milestone gates"},
				{Heading: "Owner", Detail: "Launch lead publishes tracker and criteria"},
				{Heading: "Timing", Detail: "Confirm the model within 48 hours"},
			}, 3),
		},
	}
	return reduceAdjacentVariantRepetition(out)
}

func sectionTitlesForArchetype(archetype pptxArchetype) []string {
	switch archetype {
	case pptxArchetypeCompany:
		return []string{"Context and Value", "Scenarios and Rollout"}
	case pptxArchetypeMarket:
		return []string{"Market Read", "Decision and Entry"}
	case pptxArchetypeOps:
		return []string{"Business Readout", "Priorities and Actions"}
	case pptxArchetypeProject:
		return []string{"Readiness", "Execution and Decisions"}
	case pptxArchetypeTraining:
		return []string{"Setup and Commands", "Practice and Guardrails"}
	default:
		return []string{"Core Storyline", "Decision and Next Steps"}
	}
}

func sectionTitleAt(items []string, idx int) string {
	if idx >= 0 && idx < len(items) {
		return items[idx]
	}
	if len(items) > 0 {
		return items[len(items)-1]
	}
	return ""
}

func pointsToSummarySections(points []string, limit int) []officegen.SlideSection {
	sections := make([]officegen.SlideSection, 0, len(points))
	for idx, point := range points {
		heading, detail := pointToSection(point, idx)
		if heading == "" && detail == "" {
			continue
		}
		if strings.HasPrefix(heading, "Step ") || heading == "" {
			heading = fmt.Sprintf("Takeaway %d", idx+1)
		}
		if detail == "" {
			detail = fitTextForLayout(cleanSentence(point), 24)
		}
		sections = append(sections, officegen.SlideSection{
			Heading: heading,
			Detail:  detail,
		})
		if limit > 0 && len(sections) >= limit {
			break
		}
	}
	return normalizeSections(sections, limit)
}

func summaryTitleForArchetype(archetype pptxArchetype) string {
	switch archetype {
	case pptxArchetypeCompany:
		return "Key Takeaways"
	case pptxArchetypeMarket:
		return "Key Takeaways"
	case pptxArchetypeOps:
		return "Business Takeaways"
	case pptxArchetypeProject:
		return "Launch Outcomes"
	case pptxArchetypeTraining:
		return "Learning Goals"
	default:
		return "Executive Summary"
	}
}

func summarySubtitleForArchetype(archetype pptxArchetype) string {
	switch archetype {
	case pptxArchetypeCompany:
		return "Lead with the outcome, then explain the capabilities behind it"
	case pptxArchetypeMarket:
		return "Start with the market call, then support it with evidence"
	case pptxArchetypeOps:
		return "Start with the operating conclusion, then show what is driving it"
	case pptxArchetypeProject:
		return "Align scope, owners, gates, and readiness before execution"
	case pptxArchetypeTraining:
		return "Clarify what the audience should learn before stepping into detail"
	default:
		return "Lead with the decision, then support it slide by slide"
	}
}

func actionTitleForArchetype(archetype pptxArchetype) string {
	switch archetype {
	case pptxArchetypeCompany:
		return "Rollout Path"
	case pptxArchetypeMarket:
		return "Entry Recommendations"
	case pptxArchetypeOps:
		return "Board Recommendation"
	case pptxArchetypeProject:
		return "Decision Request"
	case pptxArchetypeTraining:
		return "Next Practice Steps"
	default:
		return "Recommendation"
	}
}

func actionSubtitleForArchetype(archetype pptxArchetype) string {
	switch archetype {
	case pptxArchetypeCompany:
		return "Close with staged rollout actions, owners, and proof points"
	case pptxArchetypeMarket:
		return "Close with the market sequence, owner, and validation window"
	case pptxArchetypeOps:
		return "Close with one decision ask and the metric or proof point needed to validate it"
	case pptxArchetypeProject:
		return "Close with the decision, owner, timing, and validation gate"
	case pptxArchetypeTraining:
		return "Close with the next commands, practice loop, and caution points"
	default:
		return "Close with one recommendation and 1-2 supporting validation points"
	}
}

func looksLikeOverviewSlide(slide officegen.Slide) bool {
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle))
	for _, keyword := range []string{"summary", "takeaway", "overview", "learning goal", "headline", "key point"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func looksLikeClosingSlide(slide officegen.Slide) bool {
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle))
	for _, keyword := range []string{"next step", "next action", "decision", "recommendation", "rollout", "plan", "action"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func isReplaceableNarrativeSlide(slide officegen.Slide) bool {
	if isWeakArchetypeSlide(slide) {
		return true
	}
	if isPlaceholderSlideTitle(slide.Title) {
		return true
	}
	return strings.TrimSpace(slide.Subtitle) == "" &&
		len(slide.Points) <= 1 &&
		len(slide.Sections) == 0 &&
		len(slide.Metrics) == 0 &&
		slide.Chart == nil
}

func shouldInsertOverviewSlide(slide officegen.Slide) bool {
	switch slideLayoutName(slide) {
	case "chart", "dashboard", "gallery":
		return true
	}
	if slide.HasImage || strings.TrimSpace(slide.ImagePrompt) != "" || len(slide.Visuals) > 0 {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle))
	for _, keyword := range []string{"product", "scenario", "workflow", "interface", "demo", "experience"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func isPlaceholderSlideTitle(value string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	switch {
	case text == "":
		return true
	case strings.HasPrefix(text, "part "),
		strings.HasPrefix(text, "slide "),
		strings.HasPrefix(text, "section "),
		strings.HasPrefix(text, "first slide"),
		strings.HasPrefix(text, "second slide"),
		strings.HasPrefix(text, "third slide"),
		strings.HasPrefix(text, "fourth slide"),
		strings.HasPrefix(text, "fifth slide"),
		strings.HasPrefix(text, "sixth slide"),
		strings.HasPrefix(text, "seventh slide"),
		strings.HasPrefix(text, "eighth slide"),
		text == "overview deck",
		text == "content slide":
		return true
	default:
		return false
	}
}

func normalizeSlideVariant(slide officegen.Slide) string {
	switch strings.TrimSpace(slide.Layout) {
	case "title":
		switch strings.TrimSpace(slide.Variant) {
		case "title-split", "title-split-hero":
			return strings.TrimSpace(slide.Variant)
		case "title-center-hero", "title-center-minimal":
			return strings.TrimSpace(slide.Variant)
		}
		return "title-center"
	case "toc":
		return "toc"
	case "chapter":
		return "chapter"
	case "gallery":
		switch strings.TrimSpace(slide.Variant) {
		case "gallery-duo", "gallery-filmstrip", "gallery-focus":
			return strings.TrimSpace(slide.Variant)
		}
		return "gallery"
	case "comparison":
		switch strings.TrimSpace(slide.Variant) {
		case "comparison-columns", "comparison-vs-band", "comparison-spotlight":
			return strings.TrimSpace(slide.Variant)
		}
		return "comparison"
	case "timeline":
		switch strings.TrimSpace(slide.Variant) {
		case "timeline-axis", "timeline-steps", "timeline-zigzag":
			return strings.TrimSpace(slide.Variant)
		}
		return "timeline"
	case "closing":
		switch strings.TrimSpace(slide.Variant) {
		case "closing-checklist", "closing-takeaway", "closing-cards-light", "closing-decision-banner", "closing-rollout-strip", "closing-starter-guidance":
			return strings.TrimSpace(slide.Variant)
		}
		if slide.NarrativeRole == "closing" {
			return chooseClosingVariant(slide, pptxArchetypeGeneral)
		}
		return "closing"
	case "chart":
		return "chart-focus"
	case "dashboard":
		return "kpi-band"
	default:
		switch strings.TrimSpace(slide.Variant) {
		case "sections-grid", "sections-grid-3up", "sections-grid-staggered", "sections-grid-band", "sections-grid-persona",
			"comparison", "comparison-columns", "comparison-vs-band", "comparison-spotlight",
			"timeline", "timeline-axis", "timeline-steps", "timeline-zigzag",
			"image-right", "image-right-editorial", "image-left-editorial", "image-right-focus",
			"gallery", "gallery-duo", "gallery-filmstrip", "gallery-focus",
			"closing", "closing-checklist", "closing-takeaway", "closing-cards-light", "closing-decision-banner", "closing-rollout-strip", "closing-starter-guidance",
			"bullets", "bullets-plain", "bullets-band", "bullets-callout":
			return strings.TrimSpace(slide.Variant)
		}
		if slide.HasImage {
			return "image-right"
		}
		if len(slide.Visuals) > 0 {
			return "gallery"
		}
		if len(slide.Sections) > 0 {
			return "sections-grid"
		}
		return "bullets"
	}
}

func expandSlideForDensity(slide officegen.Slide) []officegen.Slide {
	switch slideLayoutName(slide) {
	case "toc", "chapter", "gallery", "comparison", "timeline", "closing":
		return []officegen.Slide{slide}
	}
	switch {
	case shouldSplitPointsSlide(slide):
		chunk := 4
		if slide.HasImage || longestPointRunes(slide.Points) > 80 || totalRunes(slide.Points...) > 200 {
			chunk = 3
		}
		if totalRunes(slide.Points...) > 260 && len(slide.Points) > 2 {
			chunk = 2
		}
		return splitSlidePoints(slide, chunk)
	case shouldSplitSectionsSlide(slide):
		chunk := 3
		if len(slide.Sections) > 3 || longestSectionHeadingRunes(slide.Sections) > 28 || longestSectionDetailRunes(slide.Sections) > 110 || totalSectionRunes(slide.Sections) > 280 {
			chunk = 2
		}
		return splitSlideSections(slide, chunk)
	case shouldSplitMetricsSlide(slide):
		chunk := 4
		if totalMetricRunes(slide.Metrics) > 160 && len(slide.Metrics) > 2 {
			chunk = 2
		} else if len(slide.Metrics) > 3 {
			chunk = 3
		}
		return splitSlideMetrics(slide, chunk)
	default:
		return []officegen.Slide{slide}
	}
}

func shouldSplitPointsSlide(slide officegen.Slide) bool {
	if len(slide.Points) > 4 {
		return true
	}
	if slide.HasImage && len(slide.Points) > 2 {
		return true
	}
	return longestPointRunes(slide.Points) > 80 || totalRunes(slide.Points...) > 220
}

func shouldSplitSectionsSlide(slide officegen.Slide) bool {
	if len(slide.Sections) > 3 {
		return true
	}
	if len(slide.Sections) == 0 {
		return false
	}
	return longestSectionHeadingRunes(slide.Sections) > 28 || longestSectionDetailRunes(slide.Sections) > 110 || totalSectionRunes(slide.Sections) > 280
}

func shouldSplitMetricsSlide(slide officegen.Slide) bool {
	if len(slide.Metrics) > 4 {
		return true
	}
	return len(slide.Metrics) > 2 && totalMetricRunes(slide.Metrics) > 180
}

func totalRunes(values ...string) int {
	total := 0
	for _, value := range values {
		total += utf8.RuneCountInString(strings.TrimSpace(value))
	}
	return total
}

func longestPointRunes(points []string) int {
	longest := 0
	for _, point := range points {
		if size := utf8.RuneCountInString(strings.TrimSpace(point)); size > longest {
			longest = size
		}
	}
	return longest
}

func longestSectionHeadingRunes(sections []officegen.SlideSection) int {
	longest := 0
	for _, section := range sections {
		if size := utf8.RuneCountInString(strings.TrimSpace(section.Heading)); size > longest {
			longest = size
		}
	}
	return longest
}

func longestSectionDetailRunes(sections []officegen.SlideSection) int {
	longest := 0
	for _, section := range sections {
		if size := utf8.RuneCountInString(strings.TrimSpace(section.Detail)); size > longest {
			longest = size
		}
	}
	return longest
}

func totalSectionRunes(sections []officegen.SlideSection) int {
	total := 0
	for _, section := range sections {
		total += utf8.RuneCountInString(strings.TrimSpace(section.Heading))
		total += utf8.RuneCountInString(strings.TrimSpace(section.Detail))
	}
	return total
}

func totalMetricRunes(metrics []officegen.MetricCard) int {
	total := 0
	for _, metric := range metrics {
		total += utf8.RuneCountInString(strings.TrimSpace(metric.Label))
		total += utf8.RuneCountInString(strings.TrimSpace(metric.Value))
		total += utf8.RuneCountInString(strings.TrimSpace(metric.Note))
	}
	return total
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
		category := trimChartLabel(cleanVisibleText(chart.Categories[idx]), 10)
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
		Title:      cleanVisibleText(firstNonEmpty(chart.Title, "Key Data Comparison")),
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
	case "toc":
		return "先看结构，再进入每个章节。"
	case "chapter":
		return "建立上下文后，再进入核心内容。"
	case "gallery":
		return "用更高的视觉密度承载同一主题。"
	case "comparison":
		return "把关键差异并排展示。"
	case "timeline":
		return "把阶段、时间和动作放到一条线上。"
	case "closing":
		return "用少量动作收束整套叙事。"
	case "chart":
		if len(slide.Points) > 0 {
			return cleanVisibleText(slide.Points[0])
		}
		if slide.Chart != nil {
			return "Start with the takeaway, then support it with data"
		}
	case "dashboard":
		if len(slide.Points) > 0 {
			return cleanVisibleText(slide.Points[0])
		}
		if len(slide.Metrics) > 0 {
			return "Key metrics and action focus"
		}
	}
	if len(slide.Points) > 0 {
		return cleanVisibleText(slide.Points[0])
	}
	if len(slide.Sections) > 0 {
		return cleanVisibleText(firstNonEmpty(slide.Sections[0].Detail, slide.Sections[0].Heading))
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
	case "left", "right", "top", "bottom", "background", "center", "diagonal":
		return strings.ToLower(strings.TrimSpace(pos))
	default:
		return "right"
	}
}

func allowImageForSlide(slide officegen.Slide) bool {
	switch slideLayoutName(slide) {
	case "toc", "closing", "chapter", "gallery", "comparison", "timeline", "chart", "dashboard":
		return false
	}
	if len(slide.Visuals) > 0 {
		return false
	}
	if len(slide.Sections) > 0 || len(slide.Metrics) > 0 || slide.Chart != nil {
		return false
	}
	if len(slide.Points) > 3 {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle))
	for _, keyword := range []string{"market", "industry", "competition", "review", "value", "recommendation", "next step", "rollout", "region", "opportunity", "risk", "operations", "data", "cadence"} {
		if strings.Contains(text, keyword) {
			return false
		}
	}
	for _, keyword := range []string{"product", "interface", "scenario", "training", "workflow", "experience", "demo"} {
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

func normalizeClosingSlide(slide officegen.Slide, archetype pptxArchetype) officegen.Slide {
	if slideLayoutName(slide) != "closing" && slide.NarrativeRole != "closing" {
		return slide
	}
	slide.Layout = "closing"
	slide.NarrativeRole = "closing"
	if len(slide.Sections) == 0 && len(slide.Points) > 0 {
		slide = normalizeActionSlide(slide)
	}
	slide.Sections = normalizeSections(slide.Sections, 3)
	if isBusinessLikeArchetype(archetype) && len(slide.Sections) > 0 {
		for idx := range slide.Sections {
			slide.Sections[idx].Detail = fitTextForLayout(slide.Sections[idx].Detail, 56)
		}
	}
	slide.Variant = chooseClosingVariant(slide, archetype)
	return slide
}

func chooseClosingVariant(slide officegen.Slide, archetype pptxArchetype) string {
	if strings.TrimSpace(slide.Variant) != "" {
		switch strings.TrimSpace(slide.Variant) {
		case "closing-checklist", "closing-takeaway", "closing-cards-light", "closing-decision-banner", "closing-rollout-strip", "closing-starter-guidance":
			return strings.TrimSpace(slide.Variant)
		}
	}
	if archetype == pptxArchetypeExplainer {
		if looksLikeStarterSlide(slide) || looksLikeAudienceFitSlide(slide) {
			return "closing-starter-guidance"
		}
		return "closing-takeaway"
	}
	if looksLikeRolloutClosingSlide(slide) {
		return "closing-rollout-strip"
	}
	return "closing-decision-banner"
}

func isBusinessLikeArchetype(archetype pptxArchetype) bool {
	switch archetype {
	case pptxArchetypeCompany, pptxArchetypeMarket, pptxArchetypeOps, pptxArchetypeProject, pptxArchetypeTraining:
		return true
	default:
		return false
	}
}

func allowClosingPrimaryImage(slide officegen.Slide, archetype pptxArchetype) bool {
	return false
}

func looksLikeRolloutClosingSlide(slide officegen.Slide) bool {
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.SectionTitle))
	for _, section := range slide.Sections {
		text += " " + strings.ToLower(strings.TrimSpace(section.Heading+" "+section.Detail))
	}
	for _, keyword := range []string{"rollout", "milestone", "timeline", "phase", "launch plan", "implementation", "cadence", "roadmap", "workstream", "上线", "里程碑", "阶段", "实施", "节奏", "路线图"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func normalizeEvidenceSlide(slide officegen.Slide) officegen.Slide {
	if slide.Chart != nil || len(slide.Metrics) > 0 {
		return slide
	}
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle))
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
	text := strings.ToLower(strings.TrimSpace(description + " " + title))
	switch {
	case strings.Contains(text, "enterprise collaboration platform"),
		strings.Contains(text, "协作平台"),
		strings.Contains(text, "采购建议"),
		strings.Contains(text, "value interpretation"),
		strings.Contains(text, "价值解读"):
		return pptxArchetypeCompany
	case strings.Contains(text, "market opportunity") || strings.Contains(text, "market analysis") || strings.Contains(text, "global expansion") ||
		strings.Contains(text, "市场机会") || strings.Contains(text, "市场分析") || strings.Contains(text, "出海"):
		return pptxArchetypeMarket
	case strings.Contains(text, "business review") || strings.Contains(text, "quarterly operations") || strings.Contains(text, "data report") || strings.Contains(text, "operations review") ||
		strings.Contains(text, "board review") || strings.Contains(text, "qbr") ||
		strings.Contains(text, "经营复盘") || strings.Contains(text, "季度经营") || strings.Contains(text, "月度经营") || strings.Contains(text, "管理层经营") || strings.Contains(text, "董事会复盘"):
		return pptxArchetypeOps
	case strings.Contains(text, "launch plan") || strings.Contains(text, "release plan") || strings.Contains(text, "implementation plan") || strings.Contains(text, "rollout plan") ||
		strings.Contains(text, "project implementation") || strings.Contains(text, "cross-functional launch") || strings.Contains(text, "go-to-market launch") ||
		strings.Contains(text, "项目实施") || strings.Contains(text, "实施方案") || strings.Contains(text, "上线计划") || strings.Contains(text, "发布计划") || strings.Contains(text, "里程碑计划"):
		return pptxArchetypeProject
	case strings.Contains(text, "onboarding training") || strings.Contains(text, "new hire") || strings.Contains(text, "tutorial") || strings.Contains(text, "getting started guide") ||
		strings.Contains(text, "培训课件") || strings.Contains(text, "入职培训") || strings.Contains(text, "新员工培训") || strings.Contains(text, "培训"):
		return pptxArchetypeTraining
	case strings.Contains(text, "minecraft") || strings.Contains(text, "game") || strings.Contains(text, "video game") || strings.Contains(text, "what is") || strings.Contains(text, "how it works") ||
		strings.Contains(text, "introduction to") || strings.Contains(text, "gameplay") || strings.Contains(text, "history of") ||
		strings.Contains(text, "游戏") || strings.Contains(text, "玩法") || strings.Contains(text, "介绍") || strings.Contains(text, "科普") || strings.Contains(text, "入门"):
		return pptxArchetypeExplainer
	default:
		return pptxArchetypeGeneral
	}
}

func buildArchetypePromptRules(archetype pptxArchetype) string {
	switch archetype {
	case pptxArchetypeCompany:
		return `- For this topic, a strong storyline is usually cover -> toc -> chapter -> key takeaways -> core capabilities -> customer value -> use cases -> chapter -> rollout path, but adapt the exact slide count to the prompt.
- Use the first chapter to frame context/value and the second chapter to transition into rollout.
- Customer value should prefer quantified evidence instead of abstract slogans.
- Use cases and rollout path should use sections with action, owner, timing, or validation criteria.`
	case pptxArchetypeMarket:
		return `- For this topic, a strong storyline is usually cover -> toc -> chapter -> key takeaways -> market size -> regional opportunities -> competitive landscape -> chapter -> entry recommendations, but adapt the exact slide count to the prompt.
- Slide 3 must use a chart and include a source. Do not present market size as plain text judgment.
- Regional opportunities and competition should be handled on separate slides.
- Entry recommendations should use sections with time, owner, and validation criteria. Do not add images by default for this topic.`
	case pptxArchetypeOps:
		return `- For this topic, a strong storyline is usually cover -> toc -> chapter -> business takeaways -> core metrics -> issue diagnosis -> next-quarter priorities -> chapter -> execution actions, but adapt the exact slide count to the prompt.
- Slide 3 must use a chart and clearly state the data framing or comparison period.
- Slide 4 should use sections to break issues down by dimensions such as acquisition, delivery, and collections instead of long bullets.
- The final action cluster must close the loop with at least two of phase, owner, deadline, or validation criteria. Do not add images by default for this topic.`
	case pptxArchetypeProject:
		return `- For this topic, a strong storyline is usually cover -> toc -> chapter -> launch outcomes -> readiness scorecard -> chapter -> ownership matrix -> milestones and decision gates -> risk controls -> decision request, but adapt the exact slide count to the prompt.
- Do not repeat "Executive Summary" across multiple slides. Each slide title must name the specific operating question it answers.
- Readiness, ownership, milestones, and risks should use sections, dashboard metrics, timeline, or comparison layouts instead of long bullets.
- Every execution slide should include at least two of owner, timing, decision gate, acceptance criterion, or risk trigger. Do not add images beyond the cover for this topic.`
	case pptxArchetypeTraining:
		return `- For this topic, a strong storyline is usually cover -> toc -> chapter -> learning goals -> installation and setup -> common commands -> example workflow -> chapter -> cautions, but adapt the exact slide count to the prompt.
- Setup/command slides should prefer sections organized by step, command, and result.
- Command-heavy slides should use short command names plus concise explanations. Avoid long prose and truncated commands.
- Training decks should not use images by default, and example workflows should prefer structured steps over screenshots.`
	case pptxArchetypeExplainer:
		return `- For this topic, go straight into the topic with a 6-8 slide explainer arc such as cover -> what it is -> core ways to play or main parts -> why it stands out -> optional example or gameplay visual -> who it suits -> how to start.
- Do not insert contents or chapter-divider scaffolding for this topic.
- Keep card headings especially short and readable, and preserve complete visible wording. If content feels crowded, split or reflow it instead of clipping text.
- If images are enabled, allow one cover hero image and concentrate the remaining 2-3 strong related visuals on the example or gameplay slide.
- The ending should sound like starter tips or who-it-suits guidance, not owners, milestones, rollout, or executive next steps.`
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
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle))
	for _, keyword := range []string{"recommendation", "next step", "rollout", "plan", "release", "training", "path", "action", "caution", "how to start", "beginner tip", "starter tip", "who it suits", "如何开始", "入门建议"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func pointToSection(point string, idx int) (string, string) {
	cleaned := cleanVisibleText(point)
	if cleaned == "" {
		return "", ""
	}
	for _, marker := range []string{"within 30 days", "within 60 days", "within 90 days", "weeks 1-2", "weeks 3-6", "weeks 7-10", "this week", "this month"} {
		if strings.HasPrefix(cleaned, marker) {
			return cleanVisibleText(marker), cleanVisibleText(strings.TrimSpace(strings.TrimPrefix(cleaned, marker)))
		}
	}
	for _, sep := range []string{"：", ":"} {
		parts := strings.SplitN(cleaned, sep, 2)
		if len(parts) != 2 {
			continue
		}
		label := cleanVisibleText(strings.TrimSpace(parts[0]))
		body := cleanVisibleText(strings.TrimSpace(parts[1]))
		if label != "" && body != "" {
			return label, body
		}
	}
	return fmt.Sprintf("Step %d", idx+1), cleanVisibleText(cleaned)
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
			return finishLayoutPhrase(prefix)
		}
	}
	if strings.ContainsAny(value, " \t") {
		return value
	}
	runes := []rune(value)
	for idx := maxRunes; idx > 0 && idx <= len(runes); idx-- {
		if unicode.IsSpace(runes[idx-1]) {
			candidate := strings.TrimSpace(string(runes[:idx-1]))
			if candidate != "" {
				return finishLayoutPhrase(candidate)
			}
		}
	}
	return finishLayoutPhrase(strings.TrimSpace(string(runes[:maxRunes])))
}

func shortenLayoutText(value string, maxRunes int) string {
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
			return finishLayoutPhrase(prefix)
		}
	}
	runes := []rune(value)
	start := maxRunes
	if start > len(runes) {
		start = len(runes)
	}
	for idx := start; idx > 0; idx-- {
		if unicode.IsSpace(runes[idx-1]) {
			candidate := strings.TrimSpace(string(runes[:idx-1]))
			if candidate != "" {
				return finishLayoutPhrase(candidate)
			}
		}
	}
	return finishLayoutPhrase(strings.TrimSpace(string(runes[:maxRunes])))
}

func finishLayoutPhrase(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimRight(value, " ,;:，；：")
	words := strings.Fields(value)
	for len(words) > 1 {
		tail := strings.ToLower(strings.Trim(words[len(words)-1], ".,;:，。；：()[]{}"))
		switch tail {
		case "and", "or", "but", "with", "without", "for", "from", "to", "by", "of", "in", "on", "at", "as", "the", "a", "an":
			words = words[:len(words)-1]
			value = strings.Join(words, " ")
			value = strings.TrimRight(value, " ,;:，；：")
			continue
		}
		break
	}
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, ".!?。！？") {
		return value
	}
	return value + "."
}

func diversifyBusinessLayouts(slides []officegen.Slide, archetype pptxArchetype) []officegen.Slide {
	if len(slides) == 0 || archetype == pptxArchetypeExplainer {
		return slides
	}
	sectionsGridCount := 0
	for idx := range slides {
		slide := slides[idx]
		if slideLayoutName(slide) != "content" || normalizeSlideVariant(slide) != "sections-grid" {
			continue
		}
		sectionsGridCount++
		switch {
		case sectionsGridCount == 1:
			slides[idx] = convertSectionsGridToBullets(slide)
		case sectionsGridCount > 2:
			slides[idx] = diversifySectionsGridSlide(slide)
		}
	}
	lastIdx := len(slides) - 1
	if lastIdx > 0 && slideLayoutName(slides[lastIdx]) == "closing" && slideLayoutName(slides[lastIdx-1]) == "closing" {
		slides[lastIdx-1] = diversifyClosingNeighbor(slides[lastIdx-1])
	}
	return slides
}

func reduceAdjacentVariantRepetition(slides []officegen.Slide) []officegen.Slide {
	if len(slides) < 2 {
		return slides
	}
	for idx := 1; idx < len(slides); idx++ {
		prevVariant := renderedVariantRhythmKey(slides[idx-1])
		curVariant := renderedVariantRhythmKey(slides[idx])
		if prevVariant == "" || curVariant == "" || prevVariant != curVariant {
			continue
		}
		if slideLayoutName(slides[idx-1]) != slideLayoutName(slides[idx]) {
			continue
		}
		slides[idx] = chooseAlternateVariant(slides[idx], prevVariant, idx)
	}
	return slides
}

func renderedVariantRhythmKey(slide officegen.Slide) string {
	variant := normalizeSlideVariant(slide)
	switch variant {
	case "bullets", "bullets-plain":
		return "bullets-plain"
	case "sections-grid":
		return "sections-grid-3up"
	case "timeline":
		return "timeline-axis"
	case "gallery":
		return "gallery-duo"
	case "closing":
		return "closing-cards-light"
	default:
		return variant
	}
}

func compactDeckTextDensity(slides []officegen.Slide, maxRunes int) []officegen.Slide {
	if maxRunes <= 0 {
		return slides
	}
	for idx := range slides {
		slides[idx] = compactSlideTextDensity(slides[idx], maxRunes)
	}
	return slides
}

func compactSlideTextDensity(slide officegen.Slide, maxRunes int) officegen.Slide {
	for textDensityRunes(slide) > maxRunes {
		changed := false
		for idx := range slide.Sections {
			if textDensityRunes(slide) <= maxRunes {
				break
			}
			if utf8.RuneCountInString(slide.Sections[idx].Detail) > 44 {
				next := shortenLayoutText(slide.Sections[idx].Detail, 44)
				if next != slide.Sections[idx].Detail {
					slide.Sections[idx].Detail = next
					changed = true
				}
			}
			if textDensityRunes(slide) <= maxRunes {
				break
			}
			if utf8.RuneCountInString(slide.Sections[idx].Heading) > 20 {
				next := shortenLayoutText(slide.Sections[idx].Heading, 20)
				if next != slide.Sections[idx].Heading {
					slide.Sections[idx].Heading = next
					changed = true
				}
			}
		}
		for idx := range slide.Points {
			if textDensityRunes(slide) <= maxRunes {
				break
			}
			if utf8.RuneCountInString(slide.Points[idx]) > 28 {
				next := shortenLayoutText(slide.Points[idx], 28)
				if next != slide.Points[idx] {
					slide.Points[idx] = next
					changed = true
				}
			}
		}
		for idx := range slide.Metrics {
			if textDensityRunes(slide) <= maxRunes {
				break
			}
			if utf8.RuneCountInString(slide.Metrics[idx].Note) > 32 {
				next := shortenLayoutText(slide.Metrics[idx].Note, 32)
				if next != slide.Metrics[idx].Note {
					slide.Metrics[idx].Note = next
					changed = true
				}
			}
		}
		if textDensityRunes(slide) > maxRunes && len(slide.Metrics) > 0 && len(slide.Points) > 2 {
			slide.Points = slide.Points[:2]
			changed = true
		}
		if textDensityRunes(slide) > maxRunes && len(slide.Metrics) > 0 && strings.TrimSpace(slide.Source) != "" {
			slide.Source = ""
			changed = true
		}
		if textDensityRunes(slide) > maxRunes && utf8.RuneCountInString(slide.Subtitle) > 58 {
			next := shortenLayoutText(slide.Subtitle, 58)
			if next != slide.Subtitle {
				slide.Subtitle = next
				changed = true
			}
		}
		if textDensityRunes(slide) > maxRunes && len(slide.Sections) > 2 {
			slide.Sections = slide.Sections[:2]
			changed = true
		}
		if textDensityRunes(slide) > maxRunes && len(slide.Points) > 2 {
			slide.Points = slide.Points[:2]
			changed = true
		}
		if !changed {
			break
		}
	}
	return slide
}

func textDensityRunes(slide officegen.Slide) int {
	total := utf8.RuneCountInString(slide.Title) + utf8.RuneCountInString(slide.Subtitle) + utf8.RuneCountInString(slide.Content) + utf8.RuneCountInString(slide.Source)
	for _, point := range slide.Points {
		total += utf8.RuneCountInString(point)
	}
	for _, section := range slide.Sections {
		total += utf8.RuneCountInString(section.Heading) + utf8.RuneCountInString(section.Detail)
	}
	for _, metric := range slide.Metrics {
		total += utf8.RuneCountInString(metric.Label) + utf8.RuneCountInString(metric.Value) + utf8.RuneCountInString(metric.Note)
	}
	return total
}

func chooseAlternateVariant(slide officegen.Slide, previous string, idx int) officegen.Slide {
	switch slideLayoutName(slide) {
	case "content":
		if len(slide.Sections) > 0 {
			options := []string{"sections-grid-3up", "sections-grid-staggered", "sections-grid-band", "sections-grid-persona"}
			slide.Variant = firstDifferentVariant(options, previous, idx)
			return slide
		}
		options := []string{"bullets-band", "bullets-callout"}
		slide.Variant = firstDifferentVariant(options, previous, idx)
	case "comparison":
		slide.Variant = firstDifferentVariant([]string{"comparison-vs-band", "comparison-spotlight", "comparison-columns"}, previous, idx)
	case "timeline":
		slide.Variant = firstDifferentVariant([]string{"timeline-axis", "timeline-zigzag", "timeline-steps"}, previous, idx)
	case "gallery":
		slide.Variant = firstDifferentVariant([]string{"gallery-duo", "gallery-filmstrip", "gallery-focus"}, previous, idx)
	case "closing":
		slide.Variant = firstDifferentVariant([]string{"closing-decision-banner", "closing-checklist", "closing-cards-light", "closing-takeaway"}, previous, idx)
	}
	return slide
}

func firstDifferentVariant(options []string, previous string, idx int) string {
	if len(options) == 0 {
		return previous
	}
	start := idx % len(options)
	for offset := 0; offset < len(options); offset++ {
		candidate := options[(start+offset)%len(options)]
		if candidate != previous {
			return candidate
		}
	}
	return options[0]
}

func convertSectionsGridToBullets(slide officegen.Slide) officegen.Slide {
	if len(slide.Points) == 0 {
		slide.Points = sectionBullets(slide.Sections, 3)
	}
	slide.Layout = "content"
	slide.Variant = "bullets"
	slide.Sections = nil
	slide.Metrics = nil
	slide.Chart = nil
	return slide
}

func diversifySectionsGridSlide(slide officegen.Slide) officegen.Slide {
	if len(slide.Sections) == 2 {
		slide.Layout = "comparison"
		slide.Variant = "comparison"
		slide.Points = nil
		return slide
	}
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle))
	switch {
	case strings.Contains(text, "路径"), strings.Contains(text, "步骤"), strings.Contains(text, "行动"), strings.Contains(text, "计划"), strings.Contains(text, "重点"),
		strings.Contains(text, "path"), strings.Contains(text, "step"), strings.Contains(text, "action"), strings.Contains(text, "plan"), strings.Contains(text, "priority"):
		slide.Layout = "timeline"
		slide.Variant = "timeline"
		slide.Points = nil
		return slide
	default:
		return convertSectionsGridToBullets(slide)
	}
}

func diversifyClosingNeighbor(slide officegen.Slide) officegen.Slide {
	if len(slide.Sections) == 0 && len(slide.Points) > 0 {
		slide = normalizeActionSlide(slide)
	}
	if len(slide.Sections) > 0 {
		slide.Layout = "timeline"
		slide.Variant = "timeline"
		slide.NarrativeRole = "action"
		return slide
	}
	slide.Layout = "content"
	slide.Variant = "bullets"
	slide.NarrativeRole = "action"
	return slide
}

func sectionBullets(sections []officegen.SlideSection, limit int) []string {
	points := make([]string, 0, len(sections))
	for _, section := range sections {
		heading := cleanVisibleText(section.Heading)
		detail := cleanVisibleText(section.Detail)
		switch {
		case heading != "" && detail != "":
			points = append(points, fmt.Sprintf("%s: %s", heading, detail))
		case heading != "":
			points = append(points, heading)
		case detail != "":
			points = append(points, detail)
		}
		if limit > 0 && len(points) >= limit {
			break
		}
	}
	return normalizePoints(points, maxInt(limit, 1), 32)
}

func slideLayoutName(slide officegen.Slide) string {
	layout := strings.ToLower(strings.TrimSpace(slide.Layout))
	switch layout {
	case "title", "content", "chart", "dashboard", "toc", "chapter", "gallery", "comparison", "timeline", "closing":
		return layout
	}
	if slide.IsTitle {
		return "title"
	}
	if len(slide.Visuals) > 0 {
		return "gallery"
	}
	if len(slide.Metrics) > 0 {
		return "dashboard"
	}
	if slide.Chart != nil {
		return "chart"
	}
	return "content"
}

func cleanVisibleText(value string) string {
	value = cleanSentence(value)
	if value == "" {
		return ""
	}
	return strings.Join(strings.Fields(value), " ")
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

func trimChartLabel(value string, maxRunes int) string {
	value = cleanVisibleText(value)
	if value == "" || maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	lastSpace := -1
	for idx, r := range runes {
		if unicode.IsSpace(r) {
			lastSpace = idx
		}
		if idx+1 >= maxRunes {
			break
		}
	}
	if lastSpace > 0 {
		return strings.TrimSpace(string(runes[:lastSpace]))
	}
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
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Chart.Title))
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
